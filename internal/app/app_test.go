package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
)

// binary is a gitpersona build used as the real credential helper.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gitpersona-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "gitpersona")
	if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/gitpersona").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build gitpersona: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const demoConfig = `version = 1
default = "acme"

[[profile]]
id      = "personal"
account = "octo-alex"
name    = "Alex Octo"
email   = "1000001+octo-alex@users.noreply.github.com"
dirs    = ["~/Projects/personal", "~/Projects/acme/oss"]

[[profile]]
id      = "acme"
account = "acme-alex"
name    = "Alex (Acme)"
email   = "1000002+acme-alex@users.noreply.github.com"
dirs    = ["~/Projects/acme"]

[[profile]]
id       = "globex"
provider = "gitlab"
host     = "gitlab.globex.example"
account  = "globex-alex"
name     = "Alex"
email    = "alex@globex.example"
dirs     = ["~/Projects/clients/globex"]

[[pin]]
repo    = "~/Downloads/globex-hotfix"
profile = "globex"
`

// fakeGH imitates the gh commands GitPersona uses. Signed-in accounts come
// from FAKE_GH_ACCOUNTS; every invocation is appended to GH_LOG when set.
const fakeGH = `#!/bin/sh
[ -n "$GH_LOG" ] && echo "$*" >> "$GH_LOG"
accounts="${FAKE_GH_ACCOUNTS-octo-alex acme-alex}"
case "$1 $2" in
"auth token")
  user=""
  while [ $# -gt 0 ]; do [ "$1" = --user ] && user="$2"; shift; done
  case " $accounts " in *" $user "*) echo "token-for-$user" ;; *) exit 1 ;; esac ;;
"auth status")
  printf '{"hosts":{"github.com":['
  sep=""; active=true
  for a in $accounts; do printf '%s{"login":"%s","state":"success","active":%s}' "$sep" "$a" "$active"; sep=","; active=false; done
  printf ']}}\n' ;;
"api --hostname") case "$4" in users/octo-alex) echo '{"login":"octo-alex","id":1000001,"name":"Alex Octo","email":"alex@octo.example"}' ;; users/acme-alex) echo '{"login":"acme-alex","id":1000002,"name":null,"email":null}' ;; *) exit 1 ;; esac ;;
*)
  # Any other gh command: report which account's token it received, without printing the token.
  if [ -n "$GH_TOKEN" ]; then echo "gh ran as ${GH_TOKEN#token-for-}"; else echo "gh ran with its active account"; fi ;;
esac
`

type world struct {
	home  string
	repos map[string]string
	// pathWithoutGH is PATH without the fake gh, to simulate gh being absent.
	pathWithoutGH string
}

// newWorld builds a demo home under /tmp with repositories and a fake gh,
// and isolates Git from the real user configuration.
func newWorld(t *testing.T) world {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	mustWrite(t, filepath.Join(bin, "gh"), fakeGH, 0o700)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+originalPath)
	// Commands act on the current directory; never let them see the real one.
	t.Chdir(home)

	w := world{home: home, repos: map[string]string{}, pathWithoutGH: originalPath}
	for name, rel := range map[string]string{
		"blog":    "Projects/personal/blog",
		"billing": "Projects/acme/billing",
		"sdk":     "Projects/acme/oss/sdk",
		"hotfix":  "Downloads/globex-hotfix",
		"scratch": "Desktop/scratch",
	} {
		dir := filepath.Join(home, rel)
		runGit(t, home, "init", "-q", dir)
		w.repos[name] = dir
	}
	mustWrite(t, filepath.Join(home, ".config", "gitpersona", "config.toml"), demoConfig, 0o600)
	return w
}

func (w world) env(t *testing.T, workDir string) (Env, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return Env{
		Home:            w.home,
		ConfigDir:       filepath.Join(w.home, ".config", "gitpersona"),
		WorkDir:         workDir,
		Executable:      binary,
		CaseInsensitive: paths.CaseInsensitiveFS(),
		Stdin:           strings.NewReader(""),
		Stdout:          &out,
		Stderr:          &out,
		Now:             func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
		LookPath:        exec.LookPath,
		Run:             provider.ExecRunner,
	}, &out
}

func TestApplyMakesGitUseTheRightIdentityPerDirectory(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	want := map[string]string{
		"blog":    "1000001+octo-alex@users.noreply.github.com",
		"billing": "1000002+acme-alex@users.noreply.github.com",
		"sdk":     "1000001+octo-alex@users.noreply.github.com", // nested rule wins
		"hotfix":  "alex@globex.example",                        // pin
		"scratch": "1000002+acme-alex@users.noreply.github.com", // default
	}
	for repo, email := range want {
		if got := runGit(t, w.repos[repo], "config", "user.email"); got != email {
			t.Errorf("%s: git config user.email = %q, want %q", repo, got, email)
		}
	}
}

func TestCredentialFillReturnsTheProfileAccount(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	for repo, account := range map[string]string{"billing": "acme-alex", "blog": "octo-alex", "sdk": "octo-alex"} {
		out := runGitInput(t, w.repos[repo], "protocol=https\nhost=github.com\n\n", "credential", "fill")

		if !strings.Contains(out, "username="+account) || !strings.Contains(out, "password=token-for-"+account) {
			t.Errorf("%s: git credential fill =\n%s\nwant account %s", repo, out, account)
		}
	}
}

func TestApplyPreservesUserConfigBacksUpAndIsIdempotent(t *testing.T) {
	w := newWorld(t)
	userConfig := "[alias]\n\tst = status\n"
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), userConfig, 0o644)
	env, out := w.env(t, w.home)

	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	gitconfig, _ := os.ReadFile(filepath.Join(w.home, ".gitconfig"))
	if !strings.HasPrefix(string(gitconfig), userConfig) {
		t.Errorf("~/.gitconfig lost the user's settings:\n%s", gitconfig)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("second apply output = %q, want it to report nothing to do", out.String())
	}
	snapshots, _ := os.ReadDir(filepath.Join(w.home, ".config", "gitpersona", "backups"))
	if len(snapshots) != 1 {
		t.Errorf("found %d backups, want exactly 1 (the second apply changed nothing)", len(snapshots))
	}
}

func TestUsersStoringHelperNeverReceivesGHTokens(t *testing.T) {
	w := newWorld(t)
	store := filepath.Join(t.TempDir(), "stored-credentials")
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[credential]\n\thelper = store --file "+store+"\n", 0o644)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	// What Git does around a successful push: fill, then approve to every helper.
	filled := runGitInput(t, w.repos["billing"], "protocol=https\nhost=github.com\n\n", "credential", "fill")
	runGitInput(t, w.repos["billing"], filled+"\n\n", "credential", "approve")

	if !strings.Contains(filled, "password=token-for-acme-alex") {
		t.Fatalf("GitPersona should answer for github.com:\n%s", filled)
	}
	saved, _ := os.ReadFile(store)
	if strings.Contains(string(saved), "token-for-") {
		t.Fatalf("the user's store helper saved a gh token:\n%s", saved)
	}
}

func TestUsersHelperStillServesHostsGitPersonaDoesNotHandle(t *testing.T) {
	w := newWorld(t)
	userHelper := `!f() { test "$1" = get && echo username=fallback-user && echo password=fallback-pass; }; f`
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[credential]\n\thelper = "+strconv.Quote(userHelper)+"\n", 0o644)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	gitlab := runGitInput(t, w.repos["hotfix"], "protocol=https\nhost=gitlab.globex.example\n\n", "credential", "fill")

	if !strings.Contains(gitlab, "username=fallback-user") {
		t.Fatalf("for a GitLab profile (not handled yet) the user's own helper must still answer:\n%s", gitlab)
	}
}

func TestApplyFailsOnInvalidConfig(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), "version = 1\ndefault = \"ghost\"\n", 0o600)
	env, _ := w.env(t, w.home)

	if err := Apply(context.Background(), env, ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("Apply() error = %v, want it to name the missing default profile", err)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatal("Apply() with an invalid config touched ~/.gitconfig")
	}
}

func TestWhoamiExplainsTheChoice(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.repos["sdk"])
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := Whoami(context.Background(), env, false); err != nil {
		t.Fatalf("Whoami() error: %v", err)
	}

	for _, want := range []string{"octo-alex", "personal", "directory rule", "~/Projects/acme/oss", "1000001+octo-alex@users.noreply.github.com", "path    ~/Projects/acme/oss/sdk"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("whoami output is missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "warning") {
		t.Errorf("whoami warned although Git agrees:\n%s", out.String())
	}
}

func TestWhoamiJSONAndLocalOverrideWarning(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.repos["billing"])
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	runGit(t, w.repos["billing"], "config", "--local", "user.email", "local@example.com")
	out.Reset()

	if err := Whoami(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	var got WhoamiReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("whoami --json is not valid JSON: %v\n%s", err, out.String())
	}
	if got.Profile != "acme" || got.Account != "acme-alex" || got.Reason != "rule" || got.EffectiveEmail != "local@example.com" || got.Consistent {
		t.Fatalf("whoami --json = %+v", got)
	}
}

func TestWhoamiOutsideAnyRule(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), strings.Replace(demoConfig, "default = \"acme\"\n", "", 1), 0o600)
	env, out := w.env(t, w.repos["scratch"])

	if err := Whoami(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "No GitPersona profile applies") {
		t.Fatalf("whoami output = %q", out.String())
	}
}

func TestDoctorBeforeAndAfterApply(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if ok := Doctor(context.Background(), env); ok {
		t.Fatalf("Doctor() passed before apply:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "gitpersona apply") {
		t.Errorf("doctor did not tell the user to run apply:\n%s", out.String())
	}

	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if ok := Doctor(context.Background(), env); !ok {
		t.Fatalf("Doctor() failed after apply:\n%s", out.String())
	}
	if strings.Contains(out.String(), "token-for-") {
		t.Fatalf("doctor printed a token:\n%s", out.String())
	}
}

func TestDoctorReportsAccountWithoutToken(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"),
		strings.Replace(demoConfig, `account = "acme-alex"`, `account = ""`, 1), 0o600)
	env, out := w.env(t, w.home)

	if ok := Doctor(context.Background(), env); ok {
		t.Fatalf("Doctor() passed with an invalid profile:\n%s", out.String())
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGitInput(t, dir, "", args...)
}

func runGitInput(t *testing.T, dir, input string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil && len(args) > 0 && args[0] != "config" {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mustWrite(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}
