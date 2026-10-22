package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/provider"
)

const demoConfig = `version = 1
default = "acme"

[[profile]]
id      = "personal"
account = "octo-alex"
name    = "Alex Octo"
email   = "1000001+octo-alex@users.noreply.github.com"
dirs    = ["~/Projects/personal"]

[[profile]]
id      = "acme"
account = "acme-alex"
name    = "Alex (Acme)"
email   = "1000002+acme-alex@users.noreply.github.com"
dirs    = ["~/Projects/acme"]
`

const fakeGH = `#!/bin/sh
[ -n "$GH_LOG" ] && echo "$*" >> "$GH_LOG"
case "$1 $2" in
"auth status") echo '{"hosts":{"github.com":[{"login":"octo-alex","state":"success","active":true},{"login":"acme-alex","state":"success","active":false}]}}' ;;
"auth token") echo "token-for-leak-check" ;;
*) exit 1 ;;
esac
`

type apiWorld struct {
	home string
	srv  *Server
	ts   *httptest.Server
	log  string
}

func newAPIWorld(t *testing.T) *apiWorld {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	for path, content := range map[string]string{
		filepath.Join(bin, "gh"):                                    fakeGH,
		filepath.Join(home, ".config", "gitpersona", "config.toml"): demoConfig,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	w := &apiWorld{home: home, log: filepath.Join(root, "gh.log")}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_LOG", w.log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(home)
	for _, r := range []string{"Projects/personal/blog", "Projects/acme/billing", "Desktop/scratch"} {
		if out, err := exec.Command("git", "init", "-q", filepath.Join(home, r)).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v %s", err, out)
		}
	}

	env := app.Env{
		Home: home, ConfigDir: filepath.Join(home, ".config", "gitpersona"), WorkDir: home,
		Executable: filepath.Join(bin, "gitpersona"), Stdin: strings.NewReader(""),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Now: time.Now, LookPath: exec.LookPath, Run: provider.ExecRunner,
	}
	ts := httptest.NewUnstartedServer(nil)
	srv, err := New(ts.Listener.Addr().String(), Handlers{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Register(API(env, srv.Stop))
	ts.Config.Handler = srv
	ts.Start()
	t.Cleanup(ts.Close)
	w.srv, w.ts = srv, ts
	return w
}

// call performs an authenticated API request and decodes the envelope.
func (w *apiWorld) call(t *testing.T, method, path string, body any) envelope {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, w.ts.URL+path, reader)
	req.Header.Set("Origin", w.ts.URL)
	req.Header.Set(csrfHeader, w.srv.CSRFToken())
	req.AddCookie(w.session(t))
	resp, err := w.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("%s %s: response is not JSON (status %d): %v", method, path, resp.StatusCode, err)
	}
	return env
}

func (w *apiWorld) session(t *testing.T) *http.Cookie {
	t.Helper()
	w.srv.mu.Lock()
	defer w.srv.mu.Unlock()
	if len(w.srv.sessions) == 0 {
		w.srv.keyUsed = true
		w.srv.sessions = append(w.srv.sessions, randomToken())
	}
	return &http.Cookie{Name: sessionCookie, Value: w.srv.sessions[0]}
}

func TestStateAndPreviewThenApply(t *testing.T) {
	w := newAPIWorld(t)

	state := w.call(t, "GET", "/api/state", nil)
	var s app.StateReport
	decode(t, state, &s)
	if !state.OK || len(s.Config.Profiles) != 2 || len(s.Accounts) != 2 {
		t.Fatalf("state = %+v", state)
	}

	preview := w.call(t, "GET", "/api/preview", nil)
	var changes []app.FileChange
	decode(t, preview, &changes)
	if len(changes) == 0 || changes[0].Diff == "" {
		t.Fatalf("preview = %+v", preview)
	}

	applied := w.call(t, "POST", "/api/apply", map[string]bool{"yes": false})
	if !applied.OK || !strings.Contains(applied.Log, "Backup saved") {
		t.Fatalf("apply = %+v", applied)
	}
	if got := gitEmail(t, filepath.Join(w.home, "Projects", "personal", "blog")); got != "1000001+octo-alex@users.noreply.github.com" {
		t.Fatalf("after apply through the page git uses %q", got)
	}
}

func TestEditingThroughTheAPI(t *testing.T) {
	w := newAPIWorld(t)
	steps := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/profiles", map[string]any{"id": "client", "account": "client-alex", "name": "Alex", "email": "alex@client.example"}},
		{"PATCH", "/api/profiles/client", map[string]any{"name": "Alex C."}},
		{"POST", "/api/dirs", map[string]string{"dir": "~/Projects/client", "profile": "client"}},
		{"POST", "/api/default", map[string]string{"id": "personal"}},
		{"POST", "/api/pins", map[string]string{"repo": "~/Desktop/scratch", "profile": "client"}},
	}
	for _, s := range steps {
		if got := w.call(t, s.method, s.path, s.body); !got.OK {
			t.Fatalf("%s %s = %+v", s.method, s.path, got)
		}
	}
	if got := gitEmail(t, filepath.Join(w.home, "Desktop", "scratch")); got != "alex@client.example" {
		t.Fatalf("pinned through the page, git uses %q", got)
	}

	for _, s := range []struct {
		method, path string
		body         any
	}{
		{"DELETE", "/api/pins", map[string]string{"repo": "~/Desktop/scratch"}},
		{"DELETE", "/api/dirs", map[string]string{"dir": "~/Projects/client"}},
		{"DELETE", "/api/profiles/client", nil},
	} {
		if got := w.call(t, s.method, s.path, s.body); !got.OK {
			t.Fatalf("%s %s = %+v", s.method, s.path, got)
		}
	}
	text, _ := os.ReadFile(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if strings.Contains(string(text), "client") {
		t.Fatalf("profile still present:\n%s", text)
	}
}

func TestAPIRejectsBadInputWithClearErrors(t *testing.T) {
	w := newAPIWorld(t)
	for _, s := range []struct {
		method, path string
		body         any
		want         string
	}{
		{"POST", "/api/dirs", map[string]string{"dir": "relative/dir", "profile": "acme"}, "absolute or start with ~/"},
		{"POST", "/api/dirs", map[string]string{"dir": "~/x", "profile": "ghost"}, "unknown profile"},
		{"POST", "/api/profiles", map[string]any{"id": "Bad ID"}, "id must"},
		{"POST", "/api/pins", map[string]string{"repo": "~/Desktop", "profile": "acme"}, "not inside a Git repository"},
		{"POST", "/api/apply", "not an object", "invalid request"},
	} {
		got := w.call(t, s.method, s.path, s.body)
		if got.OK || !strings.Contains(got.Error, s.want) {
			t.Errorf("%s %s = %+v, want an error containing %q", s.method, s.path, got, s.want)
		}
	}
}

func TestResolveDiscoverBackupsAndClose(t *testing.T) {
	w := newAPIWorld(t)
	w.call(t, "POST", "/api/apply", map[string]bool{})

	var who app.WhoamiReport
	decode(t, w.call(t, "GET", "/api/resolve?dir="+filepath.Join(w.home, "Projects", "acme", "billing"), nil), &who)
	if who.Profile != "acme" {
		t.Errorf("resolve = %+v", who)
	}

	var found DiscoverReport
	decode(t, w.call(t, "GET", "/api/discover?roots=~/Projects", nil), &found)
	if found.Roots[0] != "~/Projects" {
		t.Errorf("discover = %+v", found)
	}

	var backups []BackupView
	decode(t, w.call(t, "GET", "/api/backups", nil), &backups)
	if len(backups) == 0 || !strings.HasPrefix(backups[0].Command, "apply") {
		t.Fatalf("backups = %+v", backups)
	}
	if got := w.call(t, "POST", "/api/rollback", map[string]string{"name": backups[0].Name}); !got.OK {
		t.Fatalf("rollback = %+v", got)
	}

	if got := w.call(t, "POST", "/api/close", nil); !got.OK {
		t.Fatalf("close = %+v", got)
	}
	select {
	case <-w.srv.done:
	case <-time.After(time.Second):
		t.Fatal("close did not stop the server")
	}
}

func TestNoEndpointFetchesOrReturnsATokenExceptNone(t *testing.T) {
	w := newAPIWorld(t)
	var all bytes.Buffer
	for _, path := range []string{"/api/state", "/api/preview", "/api/discover", "/api/backups", "/api/resolve?dir=~/Projects"} {
		got := w.call(t, "GET", path, nil)
		data, _ := json.Marshal(got)
		all.Write(data)
	}
	w.call(t, "POST", "/api/apply", map[string]bool{})

	if strings.Contains(all.String(), "token-for-") {
		t.Fatal("an API response contains a token")
	}
	if calls, _ := os.ReadFile(w.log); strings.Contains(string(calls), "auth token") {
		t.Fatalf("the page fetched a token; gh calls:\n%s", calls)
	}
}

func decode(t *testing.T, e envelope, into any) {
	t.Helper()
	if !e.OK {
		t.Fatalf("request failed: %+v", e)
	}
	data, _ := json.Marshal(e.Data)
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

func gitEmail(t *testing.T, dir string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", dir, "config", "user.email").Output()
	return strings.TrimSpace(string(out))
}
