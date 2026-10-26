package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGhAccountsStampChangesWhenGhRewritesItsAccountFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	if got := ghAccountsStamp(t.TempDir()); got != "none" {
		t.Fatalf("stamp without hosts.yml = %q, want none", got)
	}
	hosts := filepath.Join(dir, "hosts.yml")
	mustWrite(t, hosts, "github.com:\n    user: octo-alex\n", 0o600)
	before := ghAccountsStamp(t.TempDir())

	mustWrite(t, hosts, "github.com:\n    user: acme-alex\n    users:\n        acme-alex:\n", 0o600)
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(hosts, later, later); err != nil {
		t.Fatal(err)
	}

	if after := ghAccountsStamp(t.TempDir()); after == before || before == "none" {
		t.Fatalf("stamp before %q, after %q; want a change after gh rewrote hosts.yml", before, after)
	}
}
