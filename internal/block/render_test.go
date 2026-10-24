package block

import (
	"strings"
	"testing"
)

func TestRenderIsTheRegionUpsertWritesIntoAnEmptyFile(t *testing.T) {
	body := "[include]\n\tpath = ~/rules.gitconfig\n"

	got := Render(body)

	written, err := Upsert("", body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, Begin) || !strings.Contains(got, body) || !strings.Contains(got, End) {
		t.Errorf("Render() = %q, want the body between the markers", got)
	}
	if !strings.Contains(written, got) {
		t.Errorf("Upsert() = %q does not contain Render() = %q", written, got)
	}
}
