package app

import (
	"context"
	"encoding/json"
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
	if ok || got.OK || len(got.Checks) == 0 || got.Checks[1].Status != "fail" || got.Checks[1].Advice == "" {
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
