package e2e

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file drives a real Chrome over the DevTools protocol with a minimal
// WebSocket client, so the end-to-end tests need no extra dependencies.

const (
	waitTimeout = 10 * time.Second
	pollEvery   = 50 * time.Millisecond
)

// findChrome returns a Chrome or Chromium executable, or "".
func findChrome() string {
	if path := os.Getenv("GITPERSONA_CHROME"); path != "" {
		return path
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// browser is one headless Chrome with one page.
type browser struct {
	t    *testing.T
	conn *wsConn

	mu       sync.Mutex
	nextID   int
	pending  map[int]chan cdpMessage
	problems []string // script errors, console errors and CSP violations
}

type cdpMessage struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

var devtoolsURL = regexp.MustCompile(`DevTools listening on (ws://\S+)`)

// startBrowser launches Chrome and attaches to its page.
func startBrowser(t *testing.T) *browser {
	t.Helper()
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no Chrome or Chromium found; set GITPERSONA_CHROME")
	}
	args := []string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--remote-debugging-port=0", "--user-data-dir=" + t.TempDir(), "about:blank"}
	if os.Getuid() == 0 {
		args = append(args, "--no-sandbox") // Chrome refuses to run its sandbox as root (CI containers)
	}
	cmd := exec.Command(chrome, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Chrome: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	wsURL := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if m := devtoolsURL.FindStringSubmatch(scanner.Text()); m != nil {
				wsURL <- m[1]
			}
		}
	}()
	var browserURL string
	select {
	case browserURL = <-wsURL:
	case <-time.After(waitTimeout):
		t.Fatal("Chrome did not start its DevTools endpoint")
	}
	u, _ := url.Parse(browserURL)
	conn, err := dialWebSocket(pageTarget(t, u.Host))
	if err != nil {
		t.Fatalf("connect to Chrome: %v", err)
	}
	b := &browser{t: t, conn: conn, pending: map[int]chan cdpMessage{}}
	t.Cleanup(func() { _ = conn.Close() })
	go b.readLoop()

	b.call("Runtime.enable", nil)
	b.call("Log.enable", nil)
	b.call("Page.enable", nil)
	// Collect CSP violations from every page this browser opens.
	b.call("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": `
		window.__csp = [];
		document.addEventListener("securitypolicyviolation", (e) => window.__csp.push(e.violatedDirective + " " + e.blockedURI));`})
	return b
}

func pageTarget(t *testing.T, host string) string {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + host + "/json")
		if err == nil {
			var targets []struct {
				Type string `json:"type"`
				URL  string `json:"webSocketDebuggerUrl"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&targets)
			resp.Body.Close()
			for _, target := range targets {
				if target.Type == "page" {
					return target.URL
				}
			}
		}
		time.Sleep(pollEvery)
	}
	t.Fatal("Chrome has no page")
	return ""
}

func (b *browser) readLoop() {
	for {
		data, err := b.conn.ReadMessage()
		if err != nil {
			return
		}
		var m cdpMessage
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.ID != 0 {
			b.mu.Lock()
			ch := b.pending[m.ID]
			delete(b.pending, m.ID)
			b.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		b.event(m)
	}
}

func (b *browser) event(m cdpMessage) {
	var problem string
	switch m.Method {
	case "Runtime.exceptionThrown":
		var p struct {
			Details struct {
				Text      string `json:"text"`
				Exception struct {
					Description string `json:"description"`
				} `json:"exception"`
			} `json:"exceptionDetails"`
		}
		_ = json.Unmarshal(m.Params, &p)
		problem = "script error: " + p.Details.Text + " " + p.Details.Exception.Description
	case "Log.entryAdded":
		var p struct {
			Entry struct {
				Level string `json:"level"`
				Text  string `json:"text"`
				URL   string `json:"url"`
			} `json:"entry"`
		}
		_ = json.Unmarshal(m.Params, &p)
		// Rejected requests are part of some tests; everything else counts.
		if p.Entry.Level == "error" && !strings.Contains(p.Entry.Text, "status of 4") {
			problem = "console error: " + p.Entry.Text + " " + p.Entry.URL
		}
	}
	if problem != "" {
		b.mu.Lock()
		b.problems = append(b.problems, problem)
		b.mu.Unlock()
	}
}

// call sends a DevTools command and returns its result.
func (b *browser) call(method string, params any) json.RawMessage {
	b.t.Helper()
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	ch := make(chan cdpMessage, 1)
	b.pending[id] = ch
	b.mu.Unlock()
	msg, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := b.conn.WriteMessage(msg); err != nil {
		b.t.Fatalf("%s: %v", method, err)
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			b.t.Fatalf("%s: %s", method, m.Error.Message)
		}
		return m.Result
	case <-time.After(waitTimeout):
		b.t.Fatalf("%s: no answer from Chrome", method)
		return nil
	}
}

// eval runs JavaScript in the page (awaiting promises) and decodes its value.
func (b *browser) eval(expression string, into any) {
	b.t.Helper()
	raw := b.call("Runtime.evaluate", map[string]any{
		"expression": "(async () => { " + expression + " })()", "awaitPromise": true, "returnByValue": true,
	})
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		b.t.Fatal(err)
	}
	if r.Exception != nil {
		b.t.Fatalf("evaluate %q: %s %s", expression, r.Exception.Text, r.Exception.Exception.Description)
	}
	if into != nil && len(r.Result.Value) > 0 {
		if err := json.Unmarshal(r.Result.Value, into); err != nil {
			b.t.Fatalf("evaluate %q: %v (value %s)", expression, err, r.Result.Value)
		}
	}
}

// text returns the visible text of the first element matching selector.
func (b *browser) text(selector string) string {
	b.t.Helper()
	var s string
	b.eval(fmt.Sprintf(`return document.querySelector(%q)?.innerText ?? ""`, selector), &s)
	return s
}

// waitFor polls a JavaScript condition until it is true.
func (b *browser) waitFor(what, condition string) {
	b.t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		var ok bool
		b.eval("return Boolean("+condition+")", &ok)
		if ok {
			return
		}
		time.Sleep(pollEvery)
	}
	b.t.Fatalf("timed out waiting for %s\npage: %s\nstatus: %s", what, b.text("#view"), b.text("#status"))
}

// open loads url and waits until the page has rendered a section.
func (b *browser) open(link string) {
	b.t.Helper()
	b.call("Page.navigate", map[string]any{"url": link})
	b.waitFor("the page to render", `document.querySelector("#view section h2")`)
}

// viewport emulates a screen size and color scheme.
func (b *browser) viewport(width, height int, dark bool) {
	b.t.Helper()
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{
		"width": width, "height": height, "deviceScaleFactor": 1, "mobile": width < 600,
	})
	scheme := "light"
	if dark {
		scheme = "dark"
	}
	b.call("Emulation.setEmulatedMedia", map[string]any{
		"features": []map[string]string{{"name": "prefers-color-scheme", "value": scheme}},
	})
}

// checkClean fails the test for script errors, console errors or CSP
// violations seen so far.
func (b *browser) checkClean() {
	b.t.Helper()
	var csp []string
	b.eval(`return window.__csp ?? []`, &csp)
	b.mu.Lock()
	problems := append(append([]string{}, b.problems...), csp...)
	b.mu.Unlock()
	for _, p := range problems {
		b.t.Errorf("browser reported: %s", p)
	}
}

// wsConn is a minimal client-side WebSocket connection (RFC 6455): text
// messages only, which is all the DevTools protocol uses.
type wsConn struct {
	conn net.Conn
	r    *bufio.Reader
	wmu  sync.Mutex
}

func dialWebSocket(rawURL string) (*wsConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{Timeout: waitTimeout}).DialContext(context.Background(), "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", u.RequestURI(), u.Host, key)
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	_ = resp.Body.Close() // a 101 response has no body; the connection stays open
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.StatusCode != http.StatusSwitchingProtocols ||
		resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake failed: %s", resp.Status)
	}
	return &wsConn{conn: conn, r: r}, nil
}

func (c *wsConn) Close() error { return c.conn.Close() }

// WriteMessage sends one masked text frame.
func (c *wsConn) WriteMessage(data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	var header bytes.Buffer
	header.WriteByte(0x81) // FIN + text
	switch n := len(data); {
	case n < 126:
		header.WriteByte(0x80 | byte(n))
	case n <= 0xFFFF:
		header.WriteByte(0x80 | 126)
		_ = binary.Write(&header, binary.BigEndian, uint16(n))
	default:
		header.WriteByte(0x80 | 127)
		_ = binary.Write(&header, binary.BigEndian, uint64(n))
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	header.Write(mask)
	masked := make([]byte, len(data))
	for i, b := range data {
		masked[i] = b ^ mask[i%4]
	}
	_, err := c.conn.Write(append(header.Bytes(), masked...))
	return err
}

// ReadMessage returns the next text message, joining fragments and
// skipping control frames.
func (c *wsConn) ReadMessage() ([]byte, error) {
	var message []byte
	for {
		head := make([]byte, 2)
		if _, err := io.ReadFull(c.r, head); err != nil {
			return nil, err
		}
		fin, opcode := head[0]&0x80 != 0, head[0]&0x0F
		length := uint64(head[1] & 0x7F)
		switch length {
		case 126:
			var n uint16
			if err := binary.Read(c.r, binary.BigEndian, &n); err != nil {
				return nil, err
			}
			length = uint64(n)
		case 127:
			if err := binary.Read(c.r, binary.BigEndian, &length); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(c.r, payload); err != nil {
			return nil, err
		}
		switch opcode {
		case 0x8:
			return nil, errors.New("websocket closed")
		case 0x9, 0xA:
			continue // ping and pong: Chrome does not require answers here
		}
		message = append(message, payload...)
		if fin {
			return message, nil
		}
	}
}

// sandboxPath resolves symlinks such as macOS's /tmp, so paths match what
// programs report.
func sandboxPath(t *testing.T, path string) string {
	t.Helper()
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return realPath
}
