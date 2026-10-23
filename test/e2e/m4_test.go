package e2e

import (
	"encoding/pem"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// gitServer is a Git server over HTTPS on 127.0.0.1 (git http-backend) that
// accepts each demo account with its own token, and records who pushed.
type gitServer struct {
	*httptest.Server
	host   string // 127.0.0.1:<port>
	caFile string // its certificate, for GIT_SSL_CAINFO
	mu     sync.Mutex
	pushes []string // "<account> <repo path>"
}

func startGitServer(t *testing.T, s *sandbox, tokens map[string]string, repos ...string) *gitServer {
	t.Helper()
	root := filepath.Join(filepath.Dir(s.home), "server")
	for _, r := range repos {
		s.git(filepath.Dir(s.home), "init", "-q", "--bare", filepath.Join(root, r))
		s.git(filepath.Join(root, r), "config", "http.receivepack", "true")
	}
	execPath := s.git(s.home, "--exec-path")
	g := &gitServer{}
	g.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || tokens[user] == "" || tokens[user] != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="demo"`)
			http.Error(w, "sign in", http.StatusUnauthorized)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/git-receive-pack") {
			g.mu.Lock()
			g.pushes = append(g.pushes, user+" "+strings.TrimSuffix(r.URL.Path, "/git-receive-pack"))
			g.mu.Unlock()
		}
		(&cgi.Handler{
			Path: filepath.Join(execPath, "git-http-backend"),
			Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "REMOTE_USER=" + user, "GIT_CONFIG_NOSYSTEM=1", "HOME=" + root},
		}).ServeHTTP(w, r)
	}))
	t.Cleanup(g.Close)
	u, _ := url.Parse(g.URL)
	g.host = u.Host
	g.caFile = filepath.Join(filepath.Dir(s.home), "server-ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: g.Certificate().Raw})
	if err := os.WriteFile(g.caFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	s.env = append(s.env, "GIT_SSL_CAINFO="+g.caFile)
	return g
}

func (g *gitServer) pushed() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.pushes)
}

// demoKeychain is the user's own credential helper: one token per host and
// account in a folder of the sandbox, as a system keychain keeps them.
const demoKeychain = `#!/bin/sh
store="$(dirname "$0")/../keychain"
mkdir -p "$store"
host=; user=; pass=
while IFS='=' read -r k v; do
	[ -z "$k" ] && break
	case "$k" in host) host=$v;; username) user=$v;; password) pass=$v;; esac
done
f="$store/$host@$user"
case "$1" in
	get) [ -f "$f" ] && printf 'password=%s\n' "$(cat "$f")";;
	store) printf '%s' "$pass" > "$f";;
	erase) rm -f "$f";;
esac
`

// runInput runs gitpersona in dir with input on stdin.
func (s *sandbox) runInput(dir, input string, args ...string) (string, error) {
	s.t.Helper()
	cmd := exec.Command(gitpersonaBin, args...)
	cmd.Env, cmd.Dir, cmd.Stdin = s.env, dir, strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// hostSandbox sets up two accounts on one company Git server, each with a
// directory, and the user's own credential helper.
func hostSandbox(t *testing.T) (*sandbox, *gitServer) {
	t.Helper()
	s := newSandbox(t, false)
	g := startGitServer(t, s, map[string]string{"lab-a": "token-for-a", "lab-b": "token-for-b"}, "team/a.git", "team/b.git")
	helper := filepath.Join(filepath.Dir(s.home), "bin", "git-credential-demo")
	s.write("../bin/git-credential-demo", demoKeychain, 0o700)
	s.write(".gitconfig", "[user]\n\tname = Someone\n[credential]\n\thelper = "+helper+"\n", 0o600)
	s.write(".config/gitpersona/config.toml", `version = 1

[[profile]]
id       = "a"
provider = "generic"
host     = "`+g.host+`"
account  = "lab-a"
name     = "Demo A"
email    = "a@demo.example"
dirs     = ["~/a"]

[[profile]]
id       = "b"
provider = "generic"
host     = "`+g.host+`"
account  = "lab-b"
name     = "Demo B"
email    = "b@demo.example"
dirs     = ["~/b"]
`, 0o600)
	if out := s.run("apply", "--yes"); !strings.Contains(out, "Backup saved") {
		t.Fatalf("apply:\n%s", out)
	}
	return s, g
}

// Two accounts on one host push as themselves, each with the token the
// user's own store keeps for it; GitPersona keeps no copy.
func TestEachAccountPushesToItsHostAsItself(t *testing.T) {
	s, g := hostSandbox(t)
	for _, id := range []string{"a", "b"} {
		if out, err := s.runInput(s.home, "token-for-"+id+"\n", "auth", "login", "--profile", id); err != nil {
			t.Fatalf("auth login %s: %v\n%s", id, err, out)
		}
	}

	for _, id := range []string{"a", "b"} {
		dir := filepath.Join(s.home, id, "repo")
		s.repo(id+"/repo", "https://"+g.host+"/team/"+id+".git")
		s.write(id+"/repo/README", "demo\n", 0o600)
		s.git(dir, "add", "README")
		s.git(dir, "commit", "-q", "-m", "first")
		if got := s.git(dir, "log", "-1", "--format=%ae"); got != id+"@demo.example" {
			t.Errorf("~/%s commits as %q", id, got)
		}
		s.git(dir, "push", "-q", "origin", "HEAD:main")
	}

	want := []string{"lab-a /team/a.git", "lab-b /team/b.git"}
	if got := g.pushed(); !slices.Equal(got, want) {
		t.Errorf("pushes = %v, want %v", got, want)
	}
	assertNoTokenInGitPersonaFiles(t, s, "token-for-a", "token-for-b")
	if out := s.run("auth", "status"); !strings.Contains(out, "lab-a") || !strings.Contains(out, "lab-b") {
		t.Errorf("auth status should list both accounts:\n%s", out)
	}
}

// A push without a saved token is refused by the server; it never goes
// through as the other account.
func TestAnAccountWithoutATokenNeverPushesAsAnother(t *testing.T) {
	s, g := hostSandbox(t)
	if out, err := s.runInput(s.home, "token-for-a\n", "auth", "login", "--profile", "a"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	s.repo("b/repo", "https://"+g.host+"/team/b.git")
	dir := filepath.Join(s.home, "b", "repo")
	s.write("b/repo/README", "demo\n", 0o600)
	s.git(dir, "add", "README")
	s.git(dir, "commit", "-q", "-m", "first")

	cmd := exec.Command("git", "-C", dir, "push", "-q", "origin", "HEAD:main")
	cmd.Env = append(slices.Clone(s.env), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("a push from ~/b without lab-b's token should fail:\n%s", out)
	}
	if got := g.pushed(); len(got) != 0 {
		t.Errorf("pushes = %v, want none", got)
	}
}

func assertNoTokenInGitPersonaFiles(t *testing.T, s *sandbox, tokens ...string) {
	t.Helper()
	roots := []string{filepath.Join(s.home, ".config", "gitpersona"), filepath.Join(s.home, ".gitconfig")}
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			data, _ := os.ReadFile(path)
			for _, tok := range tokens {
				if strings.Contains(string(data), tok) {
					t.Errorf("%s holds a token; GitPersona must keep no copy", path)
				}
			}
			return nil
		})
	}
}

// fakeGlab reports the account it was given, never the token itself.
const fakeGlab = `#!/bin/sh
case "$GITLAB_TOKEN" in
	"") echo "glab ran with its own account";;
	token-for-*) echo "glab ran as ${GITLAB_TOKEN#token-for-} on $GITLAB_HOST";;
	*) echo "glab ran with an unknown token";;
esac
`

// glab, through GitPersona's shim, runs as the account of the directory.
func TestGlabRunsAsTheAccountOfTheDirectory(t *testing.T) {
	s := newSandbox(t, false)
	helper := filepath.Join(filepath.Dir(s.home), "bin", "git-credential-demo")
	s.write("../bin/git-credential-demo", demoKeychain, 0o700)
	s.write("../bin/glab", fakeGlab, 0o700)
	shim := filepath.Join(filepath.Dir(s.home), "shim")
	if err := os.MkdirAll(shim, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitpersonaBin, filepath.Join(shim, "glab")); err != nil {
		t.Fatal(err)
	}
	s.write(".gitconfig", "[user]\n\tname = Someone\n[credential]\n\thelper = "+helper+"\n", 0o600)
	s.write(".config/gitpersona/config.toml", `version = 1

[[profile]]
id       = "lab"
provider = "gitlab"
host     = "gitlab.demo.example"
account  = "lab-a"
name     = "Demo A"
email    = "a@demo.example"
dirs     = ["~/lab"]
`, 0o600)
	s.run("apply", "--yes")
	if out, err := s.runInput(s.home, "token-for-lab-a\n", "auth", "login", "--profile", "lab"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	s.repo("lab/tool", "https://gitlab.demo.example/lab-a/tool.git")
	s.repo("elsewhere/x", "")

	glab := func(dir string) string {
		cmd := exec.Command(filepath.Join(shim, "glab"), "repo", "view")
		cmd.Env, cmd.Dir = slices.Clone(s.env), dir
		for i, kv := range cmd.Env {
			if strings.HasPrefix(kv, "PATH=") {
				cmd.Env[i] = "PATH=" + shim + string(os.PathListSeparator) + strings.TrimPrefix(kv, "PATH=")
			}
		}
		out, _ := cmd.CombinedOutput()
		return strings.TrimSpace(string(out))
	}

	if got := glab(filepath.Join(s.home, "lab", "tool")); got != "glab ran as lab-a on gitlab.demo.example" {
		t.Errorf("glab in ~/lab: %q", got)
	}
	if got := glab(filepath.Join(s.home, "elsewhere", "x")); got != "glab ran with its own account" {
		t.Errorf("glab outside every GitLab profile: %q", got)
	}
}

// fakeSSH greets like GitLab with the account its key belongs to, and logs
// its arguments; it never reads the key either.
const fakeSSH = `#!/bin/sh
echo "$*" >> "$(dirname "$0")/../ssh.log"
for a in "$@"; do case "$a" in *id_lab_a) echo "Welcome to GitLab, @lab-a!" >&2; exit 0;; esac; done
echo "Welcome to GitLab, @someone-else!" >&2
exit 0
`

// check-ssh reports which account each profile's key signs in as, and
// only when asked.
func TestCheckSSHSaysWhichAccountEachKeySignsInAs(t *testing.T) {
	s := newSandbox(t, false)
	s.write("../bin/ssh", fakeSSH, 0o700)
	s.write(".ssh/id_lab_a", "KEY PLACEHOLDER (never read)\n", 0o600)
	s.write(".ssh/id_lab_b", "KEY PLACEHOLDER (never read)\n", 0o600)
	s.write(".config/gitpersona/config.toml", `version = 1

[[profile]]
id       = "a"
provider = "gitlab"
host     = "gitlab.com"
account  = "lab-a"
name     = "Demo A"
email    = "a@demo.example"
ssh_key  = "~/.ssh/id_lab_a"

[[profile]]
id       = "b"
provider = "gitlab"
host     = "gitlab.com"
account  = "lab-b"
name     = "Demo B"
email    = "b@demo.example"
ssh_key  = "~/.ssh/id_lab_b"
`, 0o600)
	s.run("apply", "--yes")
	logFile := filepath.Join(filepath.Dir(s.home), "ssh.log")
	if _, err := os.Stat(logFile); err == nil {
		t.Fatal("ssh ran before check-ssh was asked for")
	}

	out := s.run("check-ssh")

	if !strings.Contains(out, "lab-a") || !strings.Contains(out, "someone-else") {
		t.Errorf("check-ssh should report both accounts:\n%s", out)
	}
	logged, _ := os.ReadFile(logFile)
	for _, want := range []string{"BatchMode=yes", "IdentitiesOnly=yes", "git@gitlab.com"} {
		if !strings.Contains(string(logged), want) {
			t.Errorf("ssh arguments %q lack %q", logged, want)
		}
	}
}

// Without gh, setup turns a hand-made identity for a company server into a
// profile, and pushes from its folder still go as that account.
func TestSetupWithoutGhTakesOverACompanyServerIdentity(t *testing.T) {
	s := newSandbox(t, false)
	if err := os.Remove(filepath.Join(filepath.Dir(s.home), "bin", "gh")); err != nil {
		t.Fatal(err)
	}
	for _, kv := range s.env {
		for _, dir := range filepath.SplitList(strings.TrimPrefix(kv, "PATH=")) {
			if _, err := os.Stat(filepath.Join(dir, "gh")); strings.HasPrefix(kv, "PATH=") && err == nil {
				t.Skipf("a real gh is on the sandbox PATH (%s); it must never run", dir)
			}
		}
	}
	g := startGitServer(t, s, map[string]string{"lab-a": "token-for-a"}, "team/a.git")
	helper := filepath.Join(filepath.Dir(s.home), "bin", "git-credential-demo")
	s.write("../bin/git-credential-demo", demoKeychain, 0o700)
	s.write(".gitconfig", "[user]\n\tname = Someone\n[credential]\n\thelper = "+helper+"\n"+
		"[includeIf \"gitdir:~/corp/\"]\n\tpath = ~/.gitconfig-corp\n", 0o600)
	s.write(".gitconfig-corp", "[user]\n\tname = Demo A\n\temail = a@demo.example\n"+
		"[credential \"https://"+g.host+"\"]\n\tusername = lab-a\n", 0o600)
	s.repo("corp/repo", "https://"+g.host+"/team/a.git")

	out := s.run("setup", "--yes", "--no-shell")

	if !strings.Contains(out, "GitPersona is set up") {
		t.Fatalf("setup without gh failed:\n%s", out)
	}
	cfg, _ := os.ReadFile(filepath.Join(s.home, ".config", "gitpersona", "config.toml"))
	if !strings.Contains(string(cfg), `host    = "`+g.host+`"`) || !strings.Contains(string(cfg), `account = "lab-a"`) {
		t.Errorf("config.toml should hold the company profile:\n%s", cfg)
	}
	if strings.Contains(s.git(s.home, "config", "--file", filepath.Join(s.home, ".gitconfig"), "--get-regexp", "includeif"), "corp") {
		t.Error("the hand-made rule the profile took over should be gone")
	}
	if o, err := s.runInput(s.home, "token-for-a\n", "auth", "login", "--profile", "lab-a"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, o)
	}
	dir := filepath.Join(s.home, "corp", "repo")
	s.write("corp/repo/README", "demo\n", 0o600)
	s.git(dir, "add", "README")
	s.git(dir, "commit", "-q", "-m", "first")
	s.git(dir, "push", "-q", "origin", "HEAD:main")
	if got := g.pushed(); !slices.Equal(got, []string{"lab-a /team/a.git"}) {
		t.Errorf("pushes = %v", got)
	}
}
