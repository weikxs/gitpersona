package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/credential"
)

func TestConfigDirFollowsXDG(t *testing.T) {
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{name: "unset uses ~/.config", xdg: "", want: "/tmp/home/.config/gitpersona"},
		{name: "absolute XDG_CONFIG_HOME", xdg: "/tmp/xdg", want: "/tmp/xdg/gitpersona"},
		{name: "relative XDG_CONFIG_HOME is ignored per the spec", xdg: "relative/xdg", want: "/tmp/home/.config/gitpersona"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)

			if got := configDir("/tmp/home"); got != tc.want {
				t.Fatalf("configDir() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestFromOSDescribesTheRunningProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	env, err := FromOS(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	if err != nil {
		t.Fatalf("FromOS() error: %v", err)
	}
	if env.Home != home || env.ConfigDir != filepath.Join(home, ".config", "gitpersona") {
		t.Errorf("FromOS() home/config = %s, %s", env.Home, env.ConfigDir)
	}
	if env.WorkDir == "" || !filepath.IsAbs(env.Executable) || env.Now == nil || env.Run == nil || env.LookPath == nil {
		t.Errorf("FromOS() left fields empty: %+v", env)
	}
}

func TestFromOSFailsWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := FromOS(nil, nil, nil); err == nil {
		t.Fatal("FromOS() without HOME succeeded, want an error")
	}
}

func TestCredentialInProcess(t *testing.T) {
	w := newWorld(t)
	target := credential.Target{Provider: "github", Host: "github.com", Account: "acme-alex"}

	answered, _ := w.env(t, w.home)
	answered.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	var out bytes.Buffer
	answered.Stdout = &out
	if err := Credential(context.Background(), answered, "get", target); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "password=token-for-acme-alex") {
		t.Errorf("Credential() output = %q, want the token from gh", out.String())
	}

	noGH, _ := w.env(t, w.home)
	noGH.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	var silent bytes.Buffer
	noGH.Stdout = &silent
	noGH.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	if err := Credential(context.Background(), noGH, "get", target); err != nil || silent.Len() != 0 {
		t.Errorf("Credential() without gh = %v, %q; want silence", err, silent.String())
	}

	if err := Credential(context.Background(), noGH, "approve", target); err == nil {
		t.Error("Credential() with an unknown operation succeeded, want an error")
	}
}

func TestDoctorWithoutGH(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.LookPath = func(string) (string, error) { return "", errors.New("not found") }

	if Doctor(context.Background(), env) {
		t.Fatal("Doctor() passed without gh")
	}
	if !strings.Contains(out.String(), "https://cli.github.com") {
		t.Errorf("doctor did not explain how to get gh:\n%s", out.String())
	}
}

func TestDoctorAccountNotSignedIn(t *testing.T) {
	w := newWorld(t)
	t.Setenv("FAKE_GH_ACCOUNTS", "octo-alex")
	env, out := w.env(t, w.home)

	if Doctor(context.Background(), env) {
		t.Fatal("Doctor() passed with an account that is not signed in")
	}
	if !strings.Contains(out.String(), "acme-alex is not signed in") || !strings.Contains(out.String(), "gh auth login --hostname github.com") {
		t.Errorf("doctor output:\n%s", out.String())
	}
}

func TestDoctorReportsGHStatusFailure(t *testing.T) {
	w := newWorld(t)
	brokenGH := filepath.Join(t.TempDir(), "gh")
	mustWrite(t, brokenGH, "#!/bin/sh\nexit 1\n", 0o700)
	env, out := w.env(t, w.home)
	env.LookPath = func(string) (string, error) { return brokenGH, nil }

	if Doctor(context.Background(), env) || !strings.Contains(out.String(), "could not ask gh") {
		t.Fatalf("doctor should report that gh could not be queried:\n%s", out.String())
	}
}

// TestNeverTouchesKeysOrTokensOutsideCredentialRequests proves the hard
// privacy rule: GitPersona does not read, list or use private keys, and
// health checks never fetch tokens.
func TestNeverTouchesKeysOrTokensOutsideCredentialRequests(t *testing.T) {
	w := newWorld(t)
	sshDir := filepath.Join(w.home, ".ssh")
	mustWrite(t, filepath.Join(sshDir, "id_ed25519_globex"), "PRIVATE KEY PLACEHOLDER\n", 0o000)
	mustWrite(t, filepath.Join(sshDir, "id_ed25519_personal.pub"), "PUBLIC KEY PLACEHOLDER\n", 0o000)
	if err := os.Chmod(sshDir, 0o000); err != nil { // not even listable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sshDir, 0o700) })
	withKeys := strings.Replace(demoConfig, `email    = "alex@globex.example"`,
		`email    = "alex@globex.example"`+"\n"+`ssh_key  = "~/.ssh/id_ed25519_globex"`, 1)
	withKeys = strings.Replace(withKeys, `dirs    = ["~/Projects/personal", "~/Projects/acme/oss"]`,
		`dirs    = ["~/Projects/personal", "~/Projects/acme/oss"]`+"\n"+`signing = { method = "ssh", key = "~/.ssh/id_ed25519_personal.pub" }`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), withKeys, 0o600)
	ghLog := filepath.Join(t.TempDir(), "gh.log")
	t.Setenv("GH_LOG", ghLog)
	env, out := w.env(t, w.repos["sdk"])

	if err := Apply(context.Background(), env); err != nil {
		t.Fatalf("Apply() with unreadable keys: %v", err)
	}
	if err := Whoami(context.Background(), env, false); err != nil {
		t.Fatalf("Whoami() with unreadable keys: %v", err)
	}
	if !Doctor(context.Background(), env) {
		t.Fatalf("Doctor() with unreadable keys failed:\n%s", out.String())
	}

	calls, _ := os.ReadFile(ghLog)
	if strings.Contains(string(calls), "auth token") || strings.Contains(string(calls), "show-token") {
		t.Fatalf("apply, whoami and doctor must never fetch a token; gh was called with:\n%s", calls)
	}
	if !strings.Contains(string(calls), "auth status") {
		t.Fatalf("doctor should check sign-in with a token-free status query; gh calls:\n%s", calls)
	}
}

func TestDoctorSkipsGHWhenNoGitHubProfiles(t *testing.T) {
	w := newWorld(t)
	gitlabOnly := `version = 1
[[profile]]
id = "globex"
provider = "gitlab"
host = "gitlab.globex.example"
account = "globex-alex"
name = "Alex"
email = "alex@globex.example"
dirs = ["~/Projects/clients/globex"]
`
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), gitlabOnly, 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if !Doctor(context.Background(), env) || strings.Contains(out.String(), "GitHub CLI") {
		t.Fatalf("doctor with only GitLab profiles should pass without checking gh:\n%s", out.String())
	}
}

func TestApplyAndDoctorReportBrokenMarkers(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), block.Begin+"\n[user]\n", 0o644)
	env, out := w.env(t, w.home)

	err := Apply(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), ".gitconfig") {
		t.Fatalf("Apply() error = %v, want it to name ~/.gitconfig", err)
	}
	if Doctor(context.Background(), env) || !strings.Contains(out.String(), "generated Git configuration:") {
		t.Fatalf("doctor should report the broken markers:\n%s", out.String())
	}
}

func TestApplyWithoutConfigExplainsWhatIsMissing(t *testing.T) {
	w := newWorld(t)
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	err := Apply(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("Apply() error = %v, want it to name the missing config file", err)
	}
}

func TestWhoamiOutsideARepositoryReportsWhatGitUsesAndWhatNewReposGet(t *testing.T) {
	w := newWorld(t)
	plain := filepath.Join(w.home, "Projects", "personal", "notes")
	if err := os.MkdirAll(plain, 0o700); err != nil {
		t.Fatal(err)
	}
	env, out := w.env(t, plain)
	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := Whoami(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}
	var got WhoamiReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.InRepository || got.Profile != "acme" || got.NewRepositories == nil || got.NewRepositories.Profile != "personal" {
		t.Fatalf("whoami --json outside a repository = %+v, want git using acme and new repositories using personal", got)
	}
	if email := runGit(t, plain, "config", "user.email"); email != got.Email {
		t.Fatalf("Git uses %q outside a repository, whoami says %q", email, got.Email)
	}

	out.Reset()
	if err := Whoami(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Not inside a Git repository") || !strings.Contains(out.String(), "will use octo-alex") {
		t.Fatalf("whoami text outside a repository:\n%s", out.String())
	}
}

func TestWhoamiAgreesWithGitInWorktrees(t *testing.T) {
	w := newWorld(t)
	runGit(t, w.repos["blog"], "-c", "user.email=x@example.com", "-c", "user.name=x", "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, w.repos["hotfix"], "-c", "user.email=x@example.com", "-c", "user.name=x", "commit", "-q", "--allow-empty", "-m", "init")
	acmeWorktree := filepath.Join(w.home, "Projects", "acme", "blog-wt")
	pinWorktree := filepath.Join(w.home, "Projects", "acme", "hotfix-wt")
	runGit(t, w.repos["blog"], "worktree", "add", "-q", acmeWorktree)
	runGit(t, w.repos["hotfix"], "worktree", "add", "-q", pinWorktree)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	for wt, want := range map[string]string{acmeWorktree: "personal", pinWorktree: "globex"} {
		env.WorkDir = wt
		out.Reset()
		if err := Whoami(context.Background(), env, true); err != nil {
			t.Fatal(err)
		}
		var got WhoamiReport
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Profile != want || !got.Consistent || runGit(t, wt, "config", "user.email") != got.Email {
			t.Errorf("%s: whoami = %+v, want %s and agreement with git", filepath.Base(wt), got, want)
		}
	}
}

func TestDisplayKeepsPathsOutsideHome(t *testing.T) {
	env := Env{Home: "/tmp/home"}

	if got := env.display("/srv/repos/x"); got != "/srv/repos/x" {
		t.Fatalf("display() = %s, want the absolute path unchanged", got)
	}
	if got := env.display("/tmp/home/Projects/x"); got != "~/Projects/x" {
		t.Fatalf("display() = %s, want ~/Projects/x", got)
	}
}

func TestReadOptionalReportsUnreadableFiles(t *testing.T) {
	if _, err := readOptional(t.TempDir()); err == nil {
		t.Fatal("readOptional() of a directory succeeded, want an error")
	}
	if got, err := readOptional(filepath.Join(t.TempDir(), "absent")); err != nil || got != "" {
		t.Fatalf("readOptional() of a missing file = %q, %v; want empty and no error", got, err)
	}
}

func TestApplyWritesThroughASymlinkedGitConfig(t *testing.T) {
	w := newWorld(t)
	target := filepath.Join(w.home, "dotfiles", "gitconfig")
	mustWrite(t, target, "[alias]\n\tst = status\n", 0o644)
	link := filepath.Join(w.home, ".gitconfig")
	if err := os.Symlink(filepath.Join("dotfiles", "gitconfig"), link); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("~/.gitconfig is no longer a symlink (err=%v)", err)
	}
	content, _ := os.ReadFile(target)
	if !strings.Contains(string(content), block.Begin) || !strings.HasPrefix(string(content), "[alias]") {
		t.Fatalf("symlink target was not updated in place:\n%s", content)
	}
	if got := runGit(t, w.repos["billing"], "config", "user.email"); got != "1000002+acme-alex@users.noreply.github.com" {
		t.Fatalf("identity through a symlinked ~/.gitconfig = %q", got)
	}
}

func TestApplyHonorsGitConfigGlobal(t *testing.T) {
	w := newWorld(t)
	alt := filepath.Join(w.home, "alt", "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", alt)
	env, _ := w.env(t, w.home)
	env.GlobalGitConfig = alt

	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(w.home, ".gitconfig")); !os.IsNotExist(err) {
		t.Errorf("apply wrote ~/.gitconfig although GIT_CONFIG_GLOBAL points elsewhere")
	}
	if got := runGit(t, w.repos["blog"], "config", "user.email"); got != "1000001+octo-alex@users.noreply.github.com" {
		t.Fatalf("identity with GIT_CONFIG_GLOBAL = %q", got)
	}
}

func TestApplyRefusesReadOnlyGitConfig(t *testing.T) {
	w := newWorld(t)
	path := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, path, "[user]\n", 0o444)
	env, _ := w.env(t, w.home)

	err := Apply(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("Apply() error = %v, want a read-only refusal", err)
	}
	if content, _ := os.ReadFile(path); string(content) != "[user]\n" {
		t.Fatalf("read-only ~/.gitconfig was modified:\n%s", content)
	}
}

func TestBrokenMarkersErrorTellsHowToFix(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), block.Begin+"\n[user]\n", 0o644)
	env, _ := w.env(t, w.home)

	err := Apply(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), "keep exactly one pair") {
		t.Fatalf("Apply() error = %v, want instructions to repair the markers", err)
	}
}

func TestStableExecutablePrefersTheCommandOnPath(t *testing.T) {
	cellar := filepath.Join(t.TempDir(), "Cellar", "gitpersona", "0.1.0", "bin")
	real := filepath.Join(cellar, "gitpersona")
	mustWrite(t, real, "#!/bin/sh\n", 0o700)
	brewBin := t.TempDir()
	link := filepath.Join(brewBin, "gitpersona")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	onPath := func(string) (string, error) { return link, nil }
	notOnPath := func(string) (string, error) { return "", errors.New("not found") }
	otherOnPath := func(string) (string, error) { return filepath.Join(brewBin, "missing"), nil }

	if got := stableExecutable(real, onPath); got != link {
		t.Errorf("stableExecutable() = %s, want the PATH entry %s that survives upgrades", got, link)
	}
	if got := stableExecutable(real, notOnPath); got != real {
		t.Errorf("stableExecutable() without a PATH entry = %s, want %s", got, real)
	}
	if got := stableExecutable(real, otherOnPath); got != real {
		t.Errorf("stableExecutable() with a different gitpersona on PATH = %s, want %s", got, real)
	}
}

func TestDoctorListsOutOfDateFiles(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	env.Executable = "/tmp/home/upgraded/gitpersona"
	out.Reset()

	if Doctor(context.Background(), env) {
		t.Fatal("Doctor() passed although the helper path changed")
	}
	if !strings.Contains(out.String(), "~/.config/gitpersona/profiles/acme.gitconfig") {
		t.Fatalf("doctor should name the files that differ:\n%s", out.String())
	}
}
