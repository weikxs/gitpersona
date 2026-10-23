package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const addr = "127.0.0.1:43210"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(addr, Handlers{
		API: map[string]http.HandlerFunc{
			"GET /api/ping":  func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("pong")) },
			"POST /api/echo": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("changed")) },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func request(method, target string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Host = addr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// login exchanges the one-time key for a session cookie.
func login(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, request("GET", "/?key="+s.Key(), nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login with the key: status %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == s.cookieName() {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("session cookie must be HttpOnly and SameSite=Strict: %+v", c)
			}
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func TestOneTimeKeyWorksOnce(t *testing.T) {
	s := newTestServer(t)
	login(t, s)

	w := httptest.NewRecorder()
	s.ServeHTTP(w, request("GET", "/?key="+s.Key(), nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("second use of the key: status %d, want 403", w.Code)
	}
}

func TestRequestsWithoutSessionAreRejected(t *testing.T) {
	s := newTestServer(t)
	for _, target := range []string{"/", "/api/ping", "/?key=wrong"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, request("GET", target, nil))
		if w.Code != http.StatusForbidden {
			t.Errorf("GET %s without a session: status %d, want 403", target, w.Code)
		}
	}
}

func TestWrongHostIsRejected(t *testing.T) {
	s := newTestServer(t)
	cookie := login(t, s)
	r := request("GET", "/api/ping", nil)
	r.Host = "evil.example:43210" // DNS rebinding
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign Host: status %d, want 403", w.Code)
	}
}

func TestChangesNeedOriginAndCSRFToken(t *testing.T) {
	s := newTestServer(t)
	cookie := login(t, s)
	good := map[string]string{"Origin": "http://" + addr, csrfHeader: s.CSRFToken()}
	cases := map[string]map[string]string{
		"no origin":      {csrfHeader: s.CSRFToken()},
		"foreign origin": {"Origin": "https://evil.example", csrfHeader: s.CSRFToken()},
		"no token":       {"Origin": "http://" + addr},
		"wrong token":    {"Origin": "http://" + addr, csrfHeader: "guess"},
	}
	for name, headers := range cases {
		r := request("POST", "/api/echo", headers)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, w.Code)
		}
	}

	r := request("POST", "/api/echo", good)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "changed" {
		t.Fatalf("valid change: status %d, body %q", w.Code, w.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(t)
	cookie := login(t, s)
	r := request("GET", "/api/ping", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	h := w.Header()
	if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'self'") ||
		!strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
		h.Get("X-Frame-Options") != "DENY" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Referrer-Policy") != "no-referrer" || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("missing security headers: %v", h)
	}
}

func TestServesTheEmbeddedPageWithTheCSRFToken(t *testing.T) {
	s := newTestServer(t)
	cookie := login(t, s)
	r := request("GET", "/", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), s.CSRFToken()) || strings.Contains(w.Body.String(), "<script>") {
		t.Fatalf("index: status %d, must carry the CSRF token and no inline script", w.Code)
	}
}

func TestIdleTimeoutStopsTheServer(t *testing.T) {
	s := newTestServer(t)
	s.idle = 20 * time.Millisecond

	select {
	case <-s.idleTimer():
	case <-time.After(time.Second):
		t.Fatal("the server did not stop after being idle")
	}
}

func TestNewRejectsNonLoopbackAddresses(t *testing.T) {
	for _, a := range []string{"0.0.0.0:0", "192.168.1.5:8080", "[::]:0", "example.com:80"} {
		if _, err := New(a, Handlers{}); err == nil {
			t.Errorf("New(%q) accepted a non-loopback address", a)
		}
	}
}

func TestServeOnARealLoopbackPort(t *testing.T) {
	ln, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ln.Addr().String(), Handlers{API: map[string]http.HandlerFunc{
		"GET /api/ping": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("pong")) },
	}})
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- s.Serve(context.Background(), ln) }()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	resp, err := client.Get(s.URL())
	if err != nil {
		t.Fatalf("open launch URL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open launch URL: %v", resp.Status)
	}
	resp, err = client.Get("http://" + ln.Addr().String() + "/api/ping")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "pong" {
		t.Fatalf("API through the real server = %q", body)
	}

	s.Stop()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return after Stop")
	}
}

func TestSessionCookiesOfTwoPagesDoNotCollide(t *testing.T) {
	a, err := New("127.0.0.1:43210", Handlers{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("127.0.0.1:43211", Handlers{})
	if err != nil {
		t.Fatal(err)
	}
	if a.cookieName() == b.cookieName() {
		t.Fatalf("both pages use the cookie %q; the second would end the first one's session", a.cookieName())
	}
}
