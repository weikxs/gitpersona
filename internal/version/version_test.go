package version

import "testing"

func TestStringIncludesVersionCommitAndDate(t *testing.T) {
	info := Info{Version: "1.2.3", Commit: "abc1234", Date: "2026-01-02T03:04:05Z"}

	got := info.String()

	want := "gitpersona 1.2.3 (commit abc1234, built 2026-01-02T03:04:05Z)"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestCurrentDefaultsToDevBuild(t *testing.T) {
	got := Current()

	if got.Version != "dev" || got.Commit != "none" || got.Date != "unknown" {
		t.Fatalf("Current() = %+v, want the dev defaults", got)
	}
}
