package textdiff

import (
	"strings"
	"testing"
)

func TestUnifiedShowsChangedLinesWithContext(t *testing.T) {
	before := "a\nb\nc\nd\ne\nf\ng\n"
	after := "a\nb\nc\nD\ne\nf\ng\nh\n"

	got := Unified(before, after, "~/.gitconfig")

	want := "--- ~/.gitconfig\n+++ ~/.gitconfig\n@@\n a\n b\n c\n-d\n+D\n e\n f\n g\n+h\n"
	if got != want {
		t.Fatalf("Unified() =\n%s\nwant\n%s", got, want)
	}
}

func TestUnifiedSeparatesDistantChanges(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = string(rune('a' + i))
	}
	before := strings.Join(lines, "\n") + "\n"
	changed := append([]string{"A"}, lines[1:19]...)
	after := strings.Join(append(changed, "T"), "\n") + "\n"

	got := Unified(before, after, "f")

	if strings.Count(got, "@@") != 2 || strings.Contains(got, " j\n") {
		t.Fatalf("two distant changes should give two hunks without the middle lines:\n%s", got)
	}
}

func TestUnifiedForNewAndRemovedFilesAndNoChange(t *testing.T) {
	if got := Unified("", "x\n", "new"); !strings.Contains(got, "+x\n") || !strings.HasPrefix(got, "--- /dev/null\n") {
		t.Errorf("new file diff:\n%s", got)
	}
	if got := Unified("x\n", "", "old"); !strings.Contains(got, "-x\n") || !strings.Contains(got, "+++ /dev/null\n") {
		t.Errorf("removed file diff:\n%s", got)
	}
	if got := Unified("same\n", "same\n", "f"); got != "" {
		t.Errorf("no change should give an empty diff, got:\n%s", got)
	}
}
