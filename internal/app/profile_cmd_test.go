package app

import (
	"context"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestProfileCommandsKeepCommentsAndStayValid(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.home)
	client := config.Profile{ID: "client", Account: "client-alex", Name: "Alex", Email: "alex@client.example", Dirs: []string{"~/Projects/client"}}

	if err := AddProfile(context.Background(), env, client); err != nil {
		t.Fatalf("AddProfile() error: %v", err)
	}
	if err := UpdateProfile(context.Background(), env, "client", ProfileChanges{Name: ptr("Alex C."), Email: ptr("alex.c@client.example")}); err != nil {
		t.Fatalf("UpdateProfile() error: %v", err)
	}
	if err := SetDefaultProfile(context.Background(), env, "client"); err != nil {
		t.Fatalf("SetDefaultProfile() error: %v", err)
	}
	text := readConfig(t, w)
	if !strings.Contains(text, "# demo setup") || !strings.Contains(text, `name    = "Alex C."`) || !strings.Contains(text, `default = "client"`) {
		t.Fatalf("config after profile commands:\n%s", text)
	}
	if got := runGit(t, w.repos["scratch"], "config", "user.email"); got != "alex.c@client.example" {
		t.Fatalf("the new default should apply at once, git uses %q", got)
	}

	if err := UpdateProfile(context.Background(), env, "client", ProfileChanges{Email: ptr("not an email")}); err == nil {
		t.Fatal("an invalid email was accepted")
	}
}

func TestRemoveProfileAlsoRemovesItsPinsAndDefault(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.repos["scratch"])
	if err := Pin(context.Background(), env, "acme"); err != nil {
		t.Fatal(err)
	}

	if err := RemoveProfile(context.Background(), env, "acme"); err != nil {
		t.Fatalf("RemoveProfile() error: %v", err)
	}

	text := readConfig(t, w)
	if strings.Contains(text, "acme-alex") || strings.Contains(text, "[[pin]]") || strings.Contains(text, "default =") {
		t.Fatalf("profile, its pin and the default should be gone:\n%s", text)
	}
	if err := RemoveProfile(context.Background(), env, "ghost"); err == nil {
		t.Fatal("removing an unknown profile succeeded")
	}
}

func ptr(s string) *string { return &s }
