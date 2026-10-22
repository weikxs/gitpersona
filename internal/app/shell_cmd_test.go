package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/shell"
)

func fishFile(w world) string {
	return filepath.Join(w.home, ".config", "fish", "conf.d", "gitpersona.fish")
}

func TestInstallShellDefaultsToTheLoginShellAndIsIdempotent(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.LoginShell = func(context.Context) shell.Detected {
		return shell.Detected{Path: "/opt/homebrew/bin/fish", Kind: shell.Fish, Source: "dscl"}
	}

	if err := InstallShell(context.Background(), env, nil, false); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(fishFile(w))
	if err != nil || !strings.Contains(string(first), "gitpersona init fish | source") || !block.Contains(string(first)) {
		t.Fatalf("fish integration = %q, %v", first, err)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".zshenv")); !os.IsNotExist(err) {
		t.Error("only the login shell should be set up by default")
	}
	if !strings.Contains(out.String(), "fish (your login shell, from dscl)") {
		t.Errorf("install-shell should say which shell it detected and how:\n%s", out.String())
	}

	out.Reset()
	if err := InstallShell(context.Background(), env, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("second install should change nothing:\n%s", out.String())
	}
}

func TestInstallShellExplicitShellsKeepsUserContentAndUninstallRemovesIt(t *testing.T) {
	w := newWorld(t)
	zshenv := filepath.Join(w.home, ".zshenv")
	bashrc := filepath.Join(w.home, ".bashrc")
	mustWrite(t, zshenv, "export EDITOR=vim\n", 0o644)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Zsh, shell.Bash}, false); err != nil {
		t.Fatal(err)
	}
	z, _ := os.ReadFile(zshenv)
	if !strings.HasPrefix(string(z), "export EDITOR=vim\n") || !strings.Contains(string(z), "gitpersona init zsh") {
		t.Fatalf(".zshenv = %q", z)
	}
	if b, _ := os.ReadFile(bashrc); !strings.Contains(string(b), "gitpersona init bash") {
		t.Fatalf(".bashrc = %q", b)
	}

	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}
	if z, _ := os.ReadFile(zshenv); string(z) != "export EDITOR=vim\n" {
		t.Fatalf(".zshenv after uninstall = %q, want the original", z)
	}
	if _, err := os.Stat(bashrc); !os.IsNotExist(err) {
		t.Fatal(".bashrc created by GitPersona should be removed by uninstall")
	}
}

func TestInstallShellDryRunWritesNothing(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(fishFile(w)); !os.IsNotExist(err) || !strings.Contains(out.String(), "would create") {
		t.Fatalf("dry run wrote the file or did not report it:\n%s", out.String())
	}
}

func TestDoctorChecksShellIntegrationOfTheLoginShell(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.LoginShell = func(context.Context) shell.Detected {
		return shell.Detected{Path: "/bin/zsh", Kind: shell.Zsh, Source: "dscl"}
	}
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)
	if !strings.Contains(out.String(), "! zsh integration is not installed") {
		t.Fatalf("doctor should warn that the login shell lacks the integration:\n%s", out.String())
	}

	if err := InstallShell(context.Background(), env, nil, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	Doctor(context.Background(), env)
	if !strings.Contains(out.String(), "✓ zsh integration is installed") {
		t.Fatalf("doctor after install-shell:\n%s", out.String())
	}
}

// TestRealFishShowsTheProfileAfterInstall runs a fresh fish with the
// installed integration and the built binary, in a demo repository.
func TestRealFishShowsTheProfileAfterInstall(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish is not installed")
	}
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("fish", "-c", "gitpersona_prompt")
	cmd.Dir = w.repos["sdk"]
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(binary)+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()

	if err != nil || strings.TrimSpace(string(out)) != "octo-alex" {
		t.Fatalf("fish prompt in a demo repository = %q, %v; want octo-alex", out, err)
	}
}

func shellEnv(t *testing.T, w world) (Env, *strings.Builder) {
	t.Helper()
	env, _ := w.env(t, w.home)
	var out strings.Builder
	env.Stdout = &out
	env.LoginShell = func(context.Context) shell.Detected {
		return shell.Detected{Path: "/opt/homebrew/bin/fish", Kind: shell.Fish, Source: "dscl"}
	}
	env.CurrentShell = func(context.Context) shell.Detected {
		return shell.Detected{Path: "-zsh", Kind: shell.Zsh, Source: "process 300"}
	}
	env.InstalledShells = func() []shell.Installed {
		return []shell.Installed{{Kind: shell.Fish, Path: "/opt/homebrew/bin/fish"}, {Kind: shell.Zsh, Path: "/bin/zsh"}, {Kind: shell.Bash, Path: "/bin/bash"}}
	}
	return env, &out
}

func TestShellsReportsDefaultCurrentAndInstalled(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := Shells(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"default  fish", "from dscl",
		"current  zsh", "process 300",
		"fish  /opt/homebrew/bin/fish  integration installed",
		"zsh   /bin/zsh  integration not installed",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("shells output is missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := Shells(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}
	var got ShellsReport
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil || got.Default.Kind != "fish" || got.Current.Kind != "zsh" || len(got.Installed) != 3 || !got.Installed[0].Integrated {
		t.Fatalf("shells --json = %+v, %v", got, err)
	}
}

func TestInstallShellMentionsOtherInstalledShells(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)

	if err := InstallShell(context.Background(), env, nil, false); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "Also installed: zsh, bash") || !strings.Contains(out.String(), "--shell fish,zsh,bash") {
		t.Fatalf("install-shell should mention the other installed shells:\n%s", out.String())
	}
}

func TestDoctorWarnsWhenTheCurrentShellLacksTheIntegration(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "✓ fish integration is installed") || !strings.Contains(out.String(), "! you are using zsh right now") {
		t.Fatalf("doctor should cover the default and the current shell:\n%s", out.String())
	}
}

// TestPluginFilesMatchTheLoader keeps the plugin-manager files in the
// repository identical in effect to what install-shell writes.
func TestPluginFilesMatchTheLoader(t *testing.T) {
	for file, kind := range map[string]shell.Kind{"../../conf.d/gitpersona.fish": shell.Fish, "../../gitpersona.plugin.zsh": shell.Zsh} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(data), loader[kind]) {
			t.Errorf("%s does not end with the loader install-shell writes:\n%s", file, data)
		}
	}
}

func TestBashLoginFileFollowsBashsOwnOrder(t *testing.T) {
	w := newWorld(t)
	profile := filepath.Join(w.home, ".profile")
	mustWrite(t, profile, "export FROM_PROFILE=1\n", 0o644)
	env, _ := w.env(t, w.home)

	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Bash}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(w.home, ".bash_profile")); !os.IsNotExist(err) {
		t.Fatal("install-shell must not create ~/.bash_profile, which would hide ~/.profile")
	}
	if p, _ := os.ReadFile(profile); !strings.HasPrefix(string(p), "export FROM_PROFILE=1\n") || !strings.Contains(string(p), "BASH_VERSION") {
		t.Fatalf("~/.profile should get a bash-only loader and keep its content:\n%s", p)
	}
	if b, _ := os.ReadFile(filepath.Join(w.home, ".bashrc")); !strings.Contains(string(b), "gitpersona init bash") {
		t.Fatal("~/.bashrc should get the loader")
	}
}

func TestZshGetsZshrcTooBecausePathHelperRunsAfterZshenv(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Zsh}, false); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{".zshenv", ".zshrc"} {
		if b, _ := os.ReadFile(filepath.Join(w.home, f)); !strings.Contains(string(b), "gitpersona init zsh") {
			t.Errorf("~/%s should load the integration", f)
		}
	}
}

func TestIntegrationInstalledOtherwiseIsRecognized(t *testing.T) {
	w := newWorld(t)
	// fisher copies the plugin's conf.d file; zsh users follow docs/integrations.md.
	fisherFile := filepath.Join(w.home, ".config", "fish", "conf.d", "gitpersona.fish")
	mustWrite(t, fisherFile, "command -q gitpersona; and gitpersona init fish | source\n", 0o644)
	mustWrite(t, filepath.Join(w.home, ".zshrc"), "eval \"$(gitpersona init zsh)\"\n", 0o644)
	env, out := shellEnv(t, w)

	if err := Shells(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "integration installed") != 2 {
		t.Fatalf("fisher and hand-made setups should count as installed:\n%s", out.String())
	}
	out.Reset()
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(fisherFile); block.Contains(string(b)) || !strings.Contains(out.String(), "already set up") {
		t.Fatalf("install-shell must not add a second loader to a fisher-managed file:\n%s\n%s", b, out.String())
	}
}

func TestDoctorWarnsWhenGHIsNotRoutedThroughGitPersona(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env) // PATH still finds the fake gh before the shim

	if !strings.Contains(out.String(), "! gh in this shell is not GitPersona's") {
		t.Fatalf("doctor should notice that gh bypasses the shim:\n%s", out.String())
	}
}
