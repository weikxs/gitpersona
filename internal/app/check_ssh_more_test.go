package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// sshCheckWorld gives the globex profile an SSH key file and a fake ssh that
// prints answer on standard error and exits with status.
func sshCheckWorld(t *testing.T, answer string, status int) (Env, *strings.Builder) {
	t.Helper()
	w := newWorld(t)
	script := "#!/bin/sh\ncat >&2 <<'EOF'\n" + answer + "EOF\nexit " + strconv.Itoa(status) + "\n"
	if status == 255 {
		script = "#!/bin/sh\ncat >&2 <<'EOF'\n" + answer + "EOF\nexit 255\n"
	}
	mustWrite(t, filepath.Join(filepath.Dir(w.home), "bin", "ssh"), script, 0o700)
	cfg := strings.Replace(demoConfig, `email    = "alex@globex.example"`, `email    = "alex@globex.example"`+"\n"+`ssh_key  = "~/.ssh/id_globex"`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), cfg, 0o600)
	mustWrite(t, filepath.Join(w.home, ".ssh", "id_globex"), "KEY PLACEHOLDER (never read)\n", 0o600)
	env, _ := w.env(t, w.home)
	var out strings.Builder
	env.Stdout = &out
	return env, &out
}

func TestCheckSSHSaysWhenTheHostSignsTheKeyInWithoutNamingAnAccount(t *testing.T) {
	env, out := sshCheckWorld(t, "Authenticated to gitlab.globex.example ([192.0.2.1]:22) using \"publickey\".\n", 0)

	err := CheckSSH(context.Background(), env, CheckSSHOptions{Profile: "globex"})

	if err != nil || !strings.Contains(out.String(), "? globex: the key signs in to gitlab.globex.example, but the host does not say as which account") {
		t.Errorf("err %v\n%s", err, out.String())
	}
}

func TestCheckSSHShowsWhatSSHSaidWhenTheKeyDidNotSignIn(t *testing.T) {
	env, out := sshCheckWorld(t, "", 255)

	err := CheckSSH(context.Background(), env, CheckSSHOptions{})

	if err == nil || !strings.Contains(out.String(), "the key did not sign in to gitlab.globex.example: no answer") {
		t.Errorf("err %v\n%s", err, out.String())
	}
}

func TestCheckSSHSaysWhenTheHostAnswersWithoutSigningTheKeyIn(t *testing.T) {
	env, out := sshCheckWorld(t, "banner: nothing useful\n", 0)

	err := CheckSSH(context.Background(), env, CheckSSHOptions{})

	if err == nil || !strings.Contains(out.String(), "✗ globex: the key did not sign in to gitlab.globex.example: banner: nothing useful") {
		t.Errorf("err %v\n%s", err, out.String())
	}
}

func TestCheckSSHSaysSSHIsMissingWithoutPretendingTheKeyFailed(t *testing.T) {
	env, out := sshCheckWorld(t, "", 0)
	env.LookPath = func(string) (string, error) { return "", os.ErrNotExist }

	err := CheckSSH(context.Background(), env, CheckSSHOptions{})

	if err == nil || !strings.Contains(out.String(), "✗ ssh not found") {
		t.Errorf("err %v\n%s", err, out.String())
	}
}

func TestCheckSSHRefusesAnUnknownProfileAndAProfileWithoutAKey(t *testing.T) {
	env, _ := sshCheckWorld(t, "", 0)
	ctx := context.Background()

	if err := CheckSSH(ctx, env, CheckSSHOptions{Profile: "no-such-profile"}); err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("unknown profile: %v", err)
	}
	if err := CheckSSH(ctx, env, CheckSSHOptions{Profile: "acme"}); err == nil || !strings.Contains(err.Error(), "has no ssh_key") {
		t.Errorf("profile without a key: %v", err)
	}
}

func TestCheckSSHSaysSoWhenNoProfileHasAKey(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if err := CheckSSH(context.Background(), env, CheckSSHOptions{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "No profile has an ssh_key") {
		t.Errorf("output = %q", out.String())
	}
}
