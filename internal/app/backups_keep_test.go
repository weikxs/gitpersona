package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/shell"
)

func TestUninstallPutsBackTheRulesAfterManyChanges(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w) // ~/.gitconfig has octo-alex's rule for ~/Projects/personal
	env, _ := w.env(t, w.home)
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	env.Now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	ctx := context.Background()
	// A backup older than setup's, so setup's is not the oldest one, which
	// pruning always keeps.
	if err := InstallShell(ctx, env, []shell.Kind{shell.Bash}, false); err != nil {
		t.Fatal(err)
	}
	if err := Setup(ctx, env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := env.loadConfig()
	if err != nil || len(cfg.Profiles) < 2 {
		t.Fatalf("setup made %+v, %v; want two profiles", cfg.Profiles, err)
	}
	for i := range backupsToKeep + 5 {
		if err := SetDefaultProfile(ctx, env, cfg.Profiles[i%2].ID); err != nil {
			t.Fatal(err)
		}
	}

	if err := Uninstall(ctx, env, true); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(w.home, ".gitconfig")); !strings.Contains(got, "[includeIf \"gitdir:~/Projects/personal/\"]") {
		t.Errorf("uninstall should put the rule back after %d more changes:\n%s", backupsToKeep+5, got)
	}
}

func TestPruneKeepsFoldersWithoutAManifestOutOfTheCount(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	env.Now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	// Something else the user keeps there.
	mustWrite(t, filepath.Join(env.backupDir(), "00-notes", "readme.txt"), "mine\n", 0o600)
	ctx := context.Background()
	for i := range backupsToKeep + 3 {
		id := "acme"
		if i%2 == 0 {
			id = "personal"
		}
		if err := SetDefaultProfile(ctx, env, id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := os.Stat(filepath.Join(env.backupDir(), "00-notes", "readme.txt")); err != nil {
		t.Errorf("a folder that is not a backup was pruned: %v", err)
	}
	entries, _ := os.ReadDir(env.backupDir())
	if len(entries) != backupsToKeep+2 { // the newest, the oldest and the user's folder
		t.Errorf("%d entries in the backup folder, want %d", len(entries), backupsToKeep+2)
	}
}
