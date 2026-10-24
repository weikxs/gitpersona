package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledHelperSuggestsOnlyWhatIsThere(t *testing.T) {
	execPath := t.TempDir()
	withLibsecret := t.TempDir()
	if err := os.WriteFile(filepath.Join(withLibsecret, "git-credential-libsecret"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	none := func(string) (string, error) { return "", errors.New("not found") }
	gcm := func(name string) (string, error) {
		if name == "git-credential-manager" {
			return "/usr/local/bin/" + name, nil
		}
		return "", errors.New("not found")
	}

	for name, tc := range map[string]struct {
		goos, execPath string
		lookPath       func(string) (string, error)
		want           string
	}{
		"linux without a keychain helper":       {"linux", execPath, none, ""},
		"linux with libsecret in Git's folder":  {"linux", withLibsecret, none, "libsecret"},
		"linux with Git Credential Manager":     {"linux", withLibsecret, gcm, "manager"},
		"macOS always has osxkeychain":          {"darwin", execPath, none, "osxkeychain"},
		"macOS prefers osxkeychain over GCM":    {"darwin", execPath, func(n string) (string, error) { return "/x/" + n, nil }, "osxkeychain"},
		"an unknown exec path is not looked in": {"linux", "", none, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := installedHelper(tc.goos, tc.execPath, tc.lookPath); got != tc.want {
				t.Fatalf("installedHelper() = %q, want %q", got, tc.want)
			}
		})
	}
}
