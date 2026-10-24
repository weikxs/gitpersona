package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/app"
)

// callRaw is call with a request body sent exactly as given.
func (w *apiWorld) callRaw(t *testing.T, method, path, body string) envelope {
	t.Helper()
	req, err := http.NewRequest(method, w.ts.URL+path, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
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

func TestEveryEndpointThatReadsABodyRefusesMalformedJSON(t *testing.T) {
	w := newAPIWorld(t)
	endpoints := []struct{ method, path string }{
		{"POST", "/api/apply"},
		{"POST", "/api/setup/preview"},
		{"POST", "/api/setup"},
		{"POST", "/api/paths"},
		{"POST", "/api/profiles"},
		{"PATCH", "/api/profiles/acme"},
		{"POST", "/api/default"},
		{"POST", "/api/dirs"},
		{"DELETE", "/api/dirs"},
		{"POST", "/api/pins"},
		{"DELETE", "/api/pins"},
		{"POST", "/api/rollback"},
		{"POST", "/api/shells"},
		{"POST", "/api/shells/preview"},
	}
	for _, e := range endpoints {
		t.Run(e.method+" "+e.path, func(t *testing.T) {
			got := w.callRaw(t, e.method, e.path, "{not json")

			if got.OK || !strings.Contains(got.Error, "invalid request") {
				t.Fatalf("response = %+v, want an invalid request error", got)
			}
		})
	}
}

func TestEndpointsThatTakeAFolderRefuseARelativeOne(t *testing.T) {
	w := newAPIWorld(t)
	tests := []struct {
		name, method, path string
		body               any
	}{
		{"remove a directory rule", "DELETE", "/api/dirs", map[string]string{"dir": "relative/dir"}},
		{"pin a repository", "POST", "/api/pins", map[string]string{"repo": "relative/repo", "profile": "acme"}},
		{"unpin a repository", "DELETE", "/api/pins", map[string]string{"repo": "relative/repo"}},
		{"resolve a folder", "GET", "/api/resolve?dir=relative/dir", nil},
		{"scan a folder", "GET", "/api/discover?roots=relative/dir", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := w.call(t, tc.method, tc.path, tc.body)

			if got.OK || !strings.Contains(got.Error, "absolute or start with ~/") {
				t.Fatalf("response = %+v, want an error about the path form", got)
			}
		})
	}
}

func TestAProfileCanBeAddedAndUpdatedWithEveryOptionalField(t *testing.T) {
	w := newAPIWorld(t)
	for _, key := range []string{"id_demo", "id_demo.pub"} { // stand-ins: only their existence is checked
		if err := os.MkdirAll(filepath.Join(w.home, ".ssh"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(w.home, ".ssh", key), []byte("demo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	profile := map[string]any{
		"id": "demo", "account": "demo-alex", "name": "Alex Demo", "email": "alex@demo.example",
		"ssh_key": "~/.ssh/id_demo", "signing": map[string]string{"method": "ssh", "key": "~/.ssh/id_demo.pub"},
	}

	added := w.call(t, "POST", "/api/profiles", profile)
	updated := w.call(t, "PATCH", "/api/profiles/demo", map[string]any{"name": "Alex D.", "signing": map[string]string{"method": "none"}})

	if !added.OK || !updated.OK {
		t.Fatalf("add = %+v, update = %+v; want both to succeed", added, updated)
	}
	data, err := os.ReadFile(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id      = "demo"`, `name    = "Alex D."`, `ssh_key = "~/.ssh/id_demo"`, `method = "none"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config.toml lacks %q:\n%s", want, data)
		}
	}
}

func TestRollbackNeedsABackupName(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/rollback", map[string]string{"name": ""})

	if got.OK || !strings.Contains(got.Error, "choose a backup") {
		t.Fatalf("response = %+v, want a request to choose a backup", got)
	}
}

func TestBackupsFailsWhenTheBackupFolderCannotBeListed(t *testing.T) {
	w := newAPIWorld(t)
	dir := app.Env{Home: w.home, ConfigDir: filepath.Join(w.home, ".config", "gitpersona")}.BackupDir()
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got := w.call(t, "GET", "/api/backups", nil)

	if got.OK || !strings.Contains(got.Error, "list backups") {
		t.Fatalf("response = %+v, want a list error", got)
	}
}

func TestShellIntegrationIsInstalledFromThePage(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/shells", map[string][]string{"shells": {"zsh"}})

	if !got.OK {
		t.Fatalf("install = %+v", got)
	}
	data, err := os.ReadFile(filepath.Join(w.home, ".zshrc"))
	if err != nil || !strings.Contains(string(data), "gitpersona") {
		t.Fatalf("~/.zshrc = %q, %v; want the gitpersona block", data, err)
	}
	if again := w.call(t, "POST", "/api/shells", map[string][]string{"shells": {"tcsh"}}); again.OK {
		t.Error("an unknown shell should be refused")
	}
}

func TestDiscoverListsIdentitiesFoundInTheUsersGitConfiguration(t *testing.T) {
	w := newAPIWorld(t)
	for path, content := range map[string]string{
		filepath.Join(w.home, ".gitconfig-demo"): "[user]\n\tname = Alex Demo\n\temail = alex@demo.example\n",
		filepath.Join(w.home, ".gitconfig"):      "[includeIf \"gitdir:~/Projects/demo/\"]\n\tpath = ~/.gitconfig-demo\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var report DiscoverReport
	decode(t, w.call(t, "GET", "/api/discover", nil), &report)

	if len(report.Existing) != 1 || report.Existing[0].Email != "alex@demo.example" ||
		report.Existing[0].Source != "~/.gitconfig-demo" {
		t.Fatalf("existing identities = %+v, want the one the rule includes, with a ~ source", report.Existing)
	}
}
