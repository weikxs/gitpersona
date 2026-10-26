package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentEditsBothSurvive(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	if err := Apply(ctx, env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	// The page and the command line (or two requests of the page) at once.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, dir := range []string{"~/Projects/personal", "~/Projects/clients/globex"} {
		wg.Go(func() {
			e, _ := w.env(t, w.home)
			errs[i] = Remove(ctx, e, dir)
		})
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("Remove() error: %v", err)
		}
	}
	cfg := readFile(t, filepath.Join(env.ConfigDir, "config.toml"))
	for _, dir := range []string{"~/Projects/personal\"", "~/Projects/clients/globex\""} {
		if strings.Contains(cfg, dir) {
			t.Errorf("config.toml still has %s; one edit overwrote the other:\n%s", dir, cfg)
		}
	}
}

func TestAnEditStopsWhenTheConfigurationChangedUnderneath(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	cfgFile := filepath.Join(env.ConfigDir, "config.toml")
	text, cfg, err := env.readConfigText()
	if err != nil {
		t.Fatal(err)
	}
	// Someone saves the file by hand after this edit read it.
	mustWrite(t, cfgFile, text+"\n# edited by hand\n", 0o600)
	edited := strings.Replace(text, `"~/Projects/personal", `, "", 1)

	err = env.saveEdited(context.Background(), "remove", text, edited, withoutDir(cfg, "personal", "~/Projects/personal", env.editOptions()))

	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("saveEdited() = %v, want it to refuse because the file changed", err)
	}
	if !strings.Contains(readFile(t, cfgFile), "# edited by hand") {
		t.Fatal("the hand edit was overwritten")
	}
}

func TestTheLockWaitsForTheOtherHolder(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	held, unlock, err := env.lockConfig()
	if err != nil || !held.configLocked {
		t.Fatalf("lockConfig() = %v", err)
	}
	// Nested calls in the same command do not wait for themselves.
	_, nested, err := held.lockConfig()
	if err != nil {
		t.Fatalf("nested lockConfig() = %v", err)
	}
	nested()

	other, _ := w.env(t, w.home)
	other.lockWait = noLockWait
	if _, _, err := other.lockConfig(); err == nil || !strings.Contains(err.Error(), "another gitpersona") {
		t.Fatalf("lockConfig() while held = %v, want a clear error", err)
	}
	unlock()
	_, again, err := other.lockConfig()
	if err != nil {
		t.Fatalf("lockConfig() after release = %v", err)
	}
	again()
	if info, err := os.Stat(filepath.Join(env.ConfigDir, ".lock")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("lock file: %v, %v; want it private", info, err)
	}
}
