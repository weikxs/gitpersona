package block

import (
	"strings"
	"testing"
)

const body = "[include]\n\tpath = /tmp/home/.config/gitpersona/rules.gitconfig\n"

func TestUpsertAppendsBlockSeparatedByBlankLine(t *testing.T) {
	original := "[user]\n\tname = Alex\n"

	got, err := Upsert(original, body)

	want := original + "\n" + Begin + "\n" + body + End + "\n"
	if err != nil || got != want {
		t.Fatalf("Upsert() = %q, %v\nwant %q", got, err, want)
	}
}

func TestUpsertIntoEmptyContent(t *testing.T) {
	got, err := Upsert("", body)

	if err != nil || got != Begin+"\n"+body+End+"\n" {
		t.Fatalf("Upsert(\"\") = %q, %v", got, err)
	}
}

func TestUpsertAddsMissingTrailingNewline(t *testing.T) {
	got, err := Upsert("[user]", body)

	if err != nil || !strings.HasPrefix(got, "[user]\n\n"+Begin) {
		t.Fatalf("Upsert() = %q, %v; want the original line terminated and a blank line before the block", got, err)
	}
}

func TestUpsertReplacesExistingBlockInPlace(t *testing.T) {
	before := "[user]\n\tname = Alex\n\n" + Begin + "\nold body\n" + End + "\n[alias]\n\tst = status\n"

	got, err := Upsert(before, body)

	want := "[user]\n\tname = Alex\n\n" + Begin + "\n" + body + End + "\n[alias]\n\tst = status\n"
	if err != nil || got != want {
		t.Fatalf("Upsert() = %q, %v\nwant %q", got, err, want)
	}
}

func TestUpsertIsIdempotent(t *testing.T) {
	once, _ := Upsert("[user]\n\tname = Alex\n", body)
	twice, err := Upsert(once, body)

	if err != nil || once != twice {
		t.Fatalf("second Upsert() changed the content:\n%q\n%q", once, twice)
	}
}

func TestRemoveRestoresOriginalExactly(t *testing.T) {
	for _, original := range []string{"", "[user]\n\tname = Alex\n", "[user]\n\n[alias]\n\tst = status\n"} {
		withBlock, err := Upsert(original, body)
		if err != nil {
			t.Fatal(err)
		}

		got, err := Remove(withBlock)

		if err != nil || got != original {
			t.Errorf("Remove(Upsert(%q)) = %q, %v; want the original", original, got, err)
		}
	}
}

func TestRemoveWithoutBlockIsNoOp(t *testing.T) {
	got, err := Remove("[user]\n")

	if err != nil || got != "[user]\n" {
		t.Fatalf("Remove() = %q, %v", got, err)
	}
}

func TestContains(t *testing.T) {
	withBlock, _ := Upsert("", body)

	if !Contains(withBlock) || Contains("[user]\n") {
		t.Fatal("Contains() did not detect the block correctly")
	}
}

func TestMalformedMarkersAreRejected(t *testing.T) {
	tests := map[string]string{
		"begin without end": Begin + "\nbody\n",
		"end without begin": "body\n" + End + "\n",
		"end before begin":  End + "\n" + Begin + "\n",
		"two blocks":        Begin + "\na\n" + End + "\n" + Begin + "\nb\n" + End + "\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Upsert(content, body); err == nil {
				t.Error("Upsert() accepted malformed markers")
			}
			if _, err := Remove(content); err == nil {
				t.Error("Remove() accepted malformed markers")
			}
		})
	}
}
