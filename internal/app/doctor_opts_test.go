package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/shell"
)

func TestDoctorFixRepairsWhatItCan(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.LoginShell = func(context.Context) shell.Detected {
		return shell.Detected{Path: "/opt/homebrew/bin/fish", Kind: shell.Fish, Source: "dscl"}
	}
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	runGit(t, w.home, "config", "--global", "user.email", "me@old.example") // lands after the block
	out.Reset()

	ok := DoctorWith(context.Background(), env, DoctorOptions{Fix: true})

	if !ok {
		t.Fatalf("doctor --fix should end healthy:\n%s", out.String())
	}
	if got := runGit(t, w.repos["blog"], "config", "user.email"); got != "1000001+octo-alex@users.noreply.github.com" {
		t.Fatalf("after --fix git uses %q", got)
	}
	if _, installed := env.integrated(shell.Fish); !installed {
		t.Fatal("--fix should install the login shell's integration")
	}
	if !strings.Contains(out.String(), "Fixing:") {
		t.Fatalf("doctor --fix should say what it fixes:\n%s", out.String())
	}
}

func TestDoctorJSON(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	ok := DoctorWith(context.Background(), env, DoctorOptions{JSON: true})

	var got DoctorReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("doctor --json is not JSON: %v\n%s", err, out.String())
	}
	outdated := slices.IndexFunc(got.Checks, func(c DoctorCheck) bool {
		return strings.HasPrefix(c.Label, "generated Git configuration is out of date")
	})
	if ok || got.OK || outdated < 0 || got.Checks[outdated].Status != "fail" || got.Checks[outdated].Advice == "" {
		t.Fatalf("doctor --json before apply = %+v", got)
	}
}

func TestWhoamiCheckFailsOnMismatch(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.repos["billing"])
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := WhoamiWith(context.Background(), env, WhoamiOptions{Check: true}); err != nil {
		t.Fatalf("whoami --check when Git agrees = %v", err)
	}

	runGit(t, w.repos["billing"], "config", "--local", "user.email", "local@example.com")

	if err := WhoamiWith(context.Background(), env, WhoamiOptions{Check: true}); err == nil || !strings.Contains(err.Error(), "git config --local --unset user.email") {
		t.Fatalf("whoami --check on mismatch = %v", err)
	}
}

func TestDoctorFixRepairsWhatItCanEvenWhenARemovalIsPending(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.ConfigDir, "profiles", "stray.gitconfig"), "# stray\n", 0o600)
	if err := os.Remove(filepath.Join(env.ConfigDir, "profiles", "personal.gitconfig")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(env.backupDir())

	DoctorWith(context.Background(), env, DoctorOptions{Fix: true})

	if _, err := os.Stat(filepath.Join(env.ConfigDir, "profiles", "personal.gitconfig")); err != nil {
		t.Fatalf("--fix should regenerate the missing profile file:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(env.ConfigDir, "profiles", "stray.gitconfig")); err != nil {
		t.Fatal("--fix must never remove files")
	}
	if !strings.Contains(out.String(), "apply --yes") {
		t.Fatalf("--fix should say how to remove the stray file:\n%s", out.String())
	}
	after, _ := os.ReadDir(env.backupDir())
	if len(after) < len(before) {
		t.Fatal("--fix must not prune backups")
	}
}

func TestDoctorJSONWithFixIsValidJSON(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	var stderr strings.Builder
	env.Stderr = &stderr

	DoctorWith(context.Background(), env, DoctorOptions{Fix: true, JSON: true})

	var got DoctorReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if !strings.Contains(stderr.String(), "Fixing:") {
		t.Fatalf("progress of --fix should go to stderr:\n%s", stderr.String())
	}
}
