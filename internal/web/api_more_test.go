package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellsAreInstalledFromThePage(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/shells", map[string][]string{"shells": {"zsh"}})

	if !got.OK {
		t.Fatalf("install shells = %+v", got)
	}
	found := false
	for _, name := range []string{".zshrc", ".zshenv", ".zprofile"} {
		data, err := os.ReadFile(filepath.Join(w.home, name))
		found = found || (err == nil && strings.Contains(string(data), "gitpersona"))
	}
	if !found {
		t.Error("no zsh startup file got the GitPersona block")
	}
}

func TestInstallingAnUnknownShellIsRefusedAndWritesNothing(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/shells", map[string][]string{"shells": {"tcsh"}})

	if got.OK || got.Error == "" {
		t.Fatalf("install of tcsh = %+v, want an error", got)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".zshrc")); !os.IsNotExist(err) {
		t.Error("a refused install wrote a startup file")
	}
}

func TestInstallingShellsWithAMalformedBodyIsRefused(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/shells", "not an object")

	if got.OK || !strings.Contains(got.Error, "invalid request") {
		t.Fatalf("install shells = %+v, want an invalid request", got)
	}
}

func TestRollbackNeedsAnExistingBackupName(t *testing.T) {
	w := newAPIWorld(t)
	w.call(t, "POST", "/api/apply", map[string]bool{}) // there is a backup, but not under that name
	for name, body := range map[string]any{
		"no name":       map[string]string{"name": ""},
		"unknown name":  map[string]string{"name": "20990101T000000.000000000Z"},
		"not an object": "nope",
	} {
		t.Run(name, func(t *testing.T) {
			got := w.call(t, "POST", "/api/rollback", body)

			if got.OK || got.Error == "" {
				t.Fatalf("rollback = %+v, want an error", got)
			}
		})
	}
}

func TestRollbackWithNoBackupsSaysSoWithoutAnError(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/rollback", map[string]string{"name": "20990101T000000.000000000Z"})

	if !got.OK || !strings.Contains(got.Log, "No backups yet") {
		t.Fatalf("rollback = %+v, want a note that there are no backups", got)
	}
}

func TestRollbackWithoutAChoiceAsksForOne(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/rollback", map[string]string{})

	if got.OK || !strings.Contains(got.Error, "choose a backup") {
		t.Fatalf("rollback = %+v, want it to ask for a backup", got)
	}
}

func TestRemovingAFolderThatBelongsToNoProfileIsRefused(t *testing.T) {
	w := newAPIWorld(t)
	for name, body := range map[string]any{
		"a relative path": map[string]string{"dir": "relative/dir"},
		"an unused path":  map[string]string{"dir": "~/Projects/nobody"},
		"a bad body":      "nope",
	} {
		t.Run(name, func(t *testing.T) {
			got := w.call(t, "DELETE", "/api/dirs", body)

			if got.OK || got.Error == "" {
				t.Fatalf("remove dir = %+v, want an error", got)
			}
		})
	}
}

func TestRemovingAFolderTakesItOutOfItsProfile(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "DELETE", "/api/dirs", map[string]string{"dir": "~/Projects/personal"})

	if !got.OK {
		t.Fatalf("remove dir = %+v", got)
	}
	text, _ := os.ReadFile(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if strings.Contains(string(text), "Projects/personal") {
		t.Errorf("the folder is still configured:\n%s", text)
	}
}

func TestPinAndUnpinRefuseBadRequests(t *testing.T) {
	w := newAPIWorld(t)
	for name, tc := range map[string]struct {
		method string
		body   any
	}{
		"pin with a bad body":             {"POST", "nope"},
		"pin with a relative repository":  {"POST", map[string]string{"repo": "relative/repo", "profile": "acme"}},
		"unpin with a bad body":           {"DELETE", "nope"},
		"unpin with a relative repo":      {"DELETE", map[string]string{"repo": "relative/repo"}},
		"unpin a repository never pinned": {"DELETE", map[string]string{"repo": "~/Projects/personal/blog"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := w.call(t, tc.method, "/api/pins", tc.body)

			if got.OK || got.Error == "" {
				t.Fatalf("%s /api/pins = %+v, want an error", tc.method, got)
			}
		})
	}
}

func TestAddingAFolderThatDoesNotExistYetSaysSo(t *testing.T) {
	w := newAPIWorld(t)

	got := w.call(t, "POST", "/api/dirs", map[string]string{"dir": "~/Projects/not-yet", "profile": "acme"})

	if !got.OK || !strings.Contains(got.Log, "does not exist yet") {
		t.Fatalf("add dir = %+v, want success with a note that the folder does not exist yet", got)
	}
}
