package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// filesUnder maps every file and link under the roots to its content or
// link target; missing roots are skipped. Backups are left out.
func filesUnder(t *testing.T, skip string, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || path == skip {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			switch {
			case d.Type()&fs.ModeSymlink != 0:
				target, _ := os.Readlink(path)
				out[path] = "-> " + target
			case !d.IsDir():
				data, _ := os.ReadFile(path)
				out[path] = string(data)
			}
			return nil
		})
	}
	return out
}

func TestAFailedWritePutsEveryFileBack(t *testing.T) {
	for name, fails := range map[string]func(n int, c change) bool{
		"the second change":           func(n int, _ change) bool { return n == 2 },
		"the global Git config, last": func(_ int, c change) bool { return filepath.Base(c.path) == ".gitconfig" },
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			env, out := w.env(t, w.home)
			gitconfig := filepath.Join(w.home, ".gitconfig")
			mustWrite(t, gitconfig, "[user]\n\tname = Alex\n", 0o644)
			before := filesUnder(t, env.backupDir(), env.ConfigDir, gitconfig)
			writes := 0
			env.write = func(c change) error {
				if writes++; fails(writes, c) {
					return errors.New("disk full")
				}
				return c.write()
			}

			err := Apply(context.Background(), env, ApplyOptions{})

			if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "put back") {
				t.Fatalf("Apply() = %v, want the write error and that the files were put back", err)
			}
			if writes < 2 {
				t.Fatalf("only %d writes; the test needs a failure after a successful write", writes)
			}
			after := filesUnder(t, env.backupDir(), env.ConfigDir, gitconfig)
			if len(after) != len(before) {
				t.Errorf("files after the failed apply: %v, want only %v\n%s", keys(after), keys(before), out)
			}
			for path, content := range before {
				if after[path] != content {
					t.Errorf("%s = %q after the failed apply, want %q", path, after[path], content)
				}
			}
		})
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestWritesWaitForTheGuard(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env.Writes = &WriteGuard{}
	env.Writes.Hold() // an interrupted process that is about to exit

	done := make(chan error, 1)
	go func() { done <- Apply(context.Background(), env, ApplyOptions{}) }()

	select {
	case err := <-done:
		t.Fatalf("Apply() finished (%v) while the guard was held; it must not start writing", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := os.Stat(filepath.Join(w.home, ".gitconfig")); err == nil {
		t.Fatal("a file was written while the guard was held")
	}
	env.Writes.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("Apply() after the guard was released: %v", err)
	}
}

func TestApplyRefusesAPlanThatIsNotTheOneReviewed(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	reviewed, err := PreviewPlan(ctx, env)
	if err != nil || reviewed.Hash == "" || len(reviewed.Changes) == 0 {
		t.Fatalf("PreviewPlan() = %+v, %v", reviewed, err)
	}
	// The configuration changes after the preview (another tab, the command line).
	cfgFile := filepath.Join(env.ConfigDir, "config.toml")
	mustWrite(t, cfgFile, strings.Replace(readFile(t, cfgFile), `name    = "Alex Octo"`, `name    = "Alex Changed"`, 1), 0o600)

	err = Apply(ctx, env, ApplyOptions{Plan: reviewed.Hash})

	if !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("Apply() = %v, want ErrPlanChanged", err)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".gitconfig")); err == nil {
		t.Fatal("apply wrote files that were not reviewed")
	}
	again, err := PreviewPlan(ctx, env)
	if err != nil || again.Hash == reviewed.Hash {
		t.Fatalf("a new preview should have a new plan: %v", err)
	}
	if err := Apply(ctx, env, ApplyOptions{Plan: again.Hash}); err != nil {
		t.Fatalf("Apply() of the reviewed plan = %v", err)
	}
}
