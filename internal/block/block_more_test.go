package block

import "testing"

func TestUpsertLastRejectsMalformedMarkers(t *testing.T) {
	if _, err := UpsertLast(Begin+"\nbody\n", "x\n"); err == nil {
		t.Fatal("UpsertLast() accepted a begin marker without an end marker")
	}
}

func TestRenderWritesTheRegionExactlyAsUpsertDoes(t *testing.T) {
	const body = "[include]\n\tpath = rules.gitconfig\n"

	got, err := Upsert("", body)

	if err != nil || Render(body) != got {
		t.Fatalf("Render() = %q, Upsert() = %q, %v; want them equal", Render(body), got, err)
	}
}
