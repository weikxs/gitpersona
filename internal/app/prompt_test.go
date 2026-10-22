package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// promptBudget is the time one prompt may take; shells run it on every
// command, so it must stay far below what a person notices.
const promptBudget = 10 * time.Millisecond

func TestPromptShowsWhatGitUses(t *testing.T) {
	w := newWorld(t)
	runGit(t, w.repos["blog"], "-c", "user.email=x@example.com", "-c", "user.name=x", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(w.home, "Projects", "acme", "blog-wt")
	runGit(t, w.repos["blog"], "worktree", "add", "-q", wt)
	notes := filepath.Join(w.home, "Projects", "personal", "notes")
	if err := os.MkdirAll(notes, 0o700); err != nil {
		t.Fatal(err)
	}

	for dir, want := range map[string]string{
		w.repos["sdk"]:                        "octo-alex",   // nested rule
		filepath.Join(w.repos["sdk"], ".git"): "octo-alex",   // inside .git
		filepath.Join(w.repos["billing"]):     "acme-alex",   // rule
		w.repos["hotfix"]:                     "globex-alex", // pin
		wt:                                    "octo-alex",   // worktree follows its main repository
		notes:                                 "acme-alex",   // not a repository: the default applies
	} {
		env, out := w.env(t, dir)

		Prompt(env, "")

		if got := strings.TrimSpace(out.String()); got != want {
			t.Errorf("prompt in %s = %q, want %q", strings.TrimPrefix(dir, w.home), got, want)
		}
	}
}

func TestPromptFormatPlaceholders(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.repos["billing"])

	Prompt(env, "[{profile}:{account}@{host}]")

	if got := strings.TrimSpace(out.String()); got != "[acme:acme-alex@github.com]" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestPromptIsSilentWhenNothingAppliesOrConfigIsBroken(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), "version = = broken", 0o600)
	env, out := w.env(t, w.repos["billing"])

	Prompt(env, "")

	if out.Len() != 0 {
		t.Fatalf("prompt with a broken config printed %q, want nothing", out.String())
	}
}

func TestPromptIsFast(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, filepath.Join(w.repos["sdk"]))
	const runs = 50

	start := time.Now()
	for range runs {
		Prompt(env, "")
	}
	perRun := time.Since(start) / runs

	if perRun > promptBudget {
		t.Fatalf("prompt takes %v per run, budget is %v", perRun, promptBudget)
	}
}
