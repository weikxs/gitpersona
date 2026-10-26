package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/weikxs/gitpersona/internal/shell"
)

func TestSavingAChangeTellsShellPromptsToAskAgain(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	generation := filepath.Join(env.ConfigDir, shell.GenerationFile)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(generation)
	if err != nil || len(first) == 0 {
		t.Fatalf("no generation after apply: %q, %v", first, err)
	}

	here, _ := w.env(t, w.repos["scratch"])
	if err := Use(context.Background(), here, "personal"); err != nil {
		t.Fatal(err)
	}

	if second, _ := os.ReadFile(generation); string(second) == string(first) {
		t.Fatalf("generation unchanged after a saved change: %q", second)
	}
}
