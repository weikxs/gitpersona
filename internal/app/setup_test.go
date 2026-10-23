package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestSetupCreatesAConfigFromSignedInAccounts(t *testing.T) {
	w := newWorld(t)
	path := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	env, out := w.env(t, w.home)

	if err := Setup(context.Background(), env, SetupOptions{Yes: true}); err != nil {
		t.Fatalf("Setup() error: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
	if err := cfg.Validate(w.home, false); err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}
	if cfg.Default != "octo-alex" || len(cfg.Profiles) != 2 {
		t.Fatalf("config = %+v, want two profiles with the active account as default", cfg)
	}
	personal, _ := cfg.ProfileByID("octo-alex")
	acme, _ := cfg.ProfileByID("acme-alex")
	if personal.Email != "alex@octo.example" || personal.Name != "Alex Octo" {
		t.Errorf("a profile should use the account's public name and email, got %+v", personal)
	}
	if acme.Name != "acme-alex" || acme.Email != "1000002+acme-alex@users.noreply.github.com" {
		t.Errorf("without a public name or email a profile falls back to the login and noreply address, got %+v", acme)
	}
	if text := readConfigFile(t, w); !strings.Contains(text, "# public email on GitHub") || !strings.Contains(text, "shows no public email") {
		t.Errorf("config.toml should say where each email came from:\n%s", text)
	}
	if !strings.Contains(out.String(), "gitpersona doctor") {
		t.Errorf("setup should explain the next steps:\n%s", out.String())
	}
}

func TestSetupRefusesToOverwriteAndPrintWritesNothing(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if err := Setup(context.Background(), env, SetupOptions{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Setup() over an existing config = %v", err)
	}
	if err := Setup(context.Background(), env, SetupOptions{Print: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[[profile]]") || readConfigFile(t, w) != demoConfig {
		t.Fatalf("--print should only print:\n%s", out.String())
	}
}

func TestSetupWithoutGHExplains(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env.LookPath = func(string) (string, error) { return "", os.ErrNotExist }

	if err := Setup(context.Background(), env, SetupOptions{Print: true}); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("Setup() without gh = %v", err)
	}
}

func readConfigFile(t *testing.T, w world) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	return string(data)
}

func TestSetupShowsWhatItFoundAndAsksFirst(t *testing.T) {
	_, env := autoWorld(t)
	out := &strings.Builder{}
	env.Stdout = out

	env.Stdin = strings.NewReader("n\n")
	if err := Setup(context.Background(), env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.configFile()); !os.IsNotExist(err) {
		t.Fatal("answering n must write nothing")
	}
	for _, want := range []string{
		"~/code/mine  (its one repository belongs to octo-alex)",
		"SSH key ~/.ssh/id_acme",
		"Pin ~/code/acme/side to octo-alex: the repository sets alex@octo.example",
		"commits will use octo-alex (Alex Octo <alex@octo.example>) instead of Demo Person <person@demo.example>",
		"~/code/other  (its one repository, owned by other-org)",
		"Nothing was written",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the summary should say %q:\n%s", want, out.String())
		}
	}

	env.Stdin = strings.NewReader("\n") // Enter accepts
	if err := Setup(context.Background(), env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.configFile()); err != nil {
		t.Fatalf("Enter should apply the setup: %v", err)
	}
}

func TestSetupWithoutATerminalAsksForYes(t *testing.T) {
	_, env := autoWorld(t)
	env.Stdin = strings.NewReader("")

	if err := Setup(context.Background(), env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(env.configFile()); !os.IsNotExist(err) {
		t.Fatal("without an answer nothing may be written")
	}
}
