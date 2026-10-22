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

	// No blank line: that is how Remove knows the newline was added.
	if err != nil || !strings.HasPrefix(got, "[user]\n"+Begin) {
		t.Fatalf("Upsert() = %q, %v; want the original line terminated, directly followed by the block", got, err)
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

func TestUpsertNormalizesBodyWithoutTrailingNewline(t *testing.T) {
	got, err := Upsert("", "[include]")

	if err != nil || got != Begin+"\n[include]\n"+End+"\n" {
		t.Fatalf("Upsert() = %q, %v", got, err)
	}
}

func TestUpsertWithEmptyBodyKeepsOnlyMarkers(t *testing.T) {
	got, err := Upsert("", "")

	if err != nil || got != Begin+"\n"+End+"\n" {
		t.Fatalf("Upsert() = %q, %v", got, err)
	}
}

func TestMarkersWithCRLFLineEndingsAreRecognized(t *testing.T) {
	content := "[user]\r\n" + Begin + "\r\nold\r\n" + End + "\r\n"

	if !Contains(content) {
		t.Fatal("Contains() did not recognize markers with CRLF line endings")
	}
}

func TestUpsertLastMovesTheBlockToTheEnd(t *testing.T) {
	before := "[alias]\n\tst = status\n\n" + Begin + "\nold\n" + End + "\n[user]\n\temail = me@old.example\n"

	got, err := UpsertLast(before, body)

	want := "[alias]\n\tst = status\n\n[user]\n\temail = me@old.example\n\n" + Begin + "\n" + body + End + "\n"
	if err != nil || got != want {
		t.Fatalf("UpsertLast() = %q, %v\nwant %q", got, err, want)
	}
}

func TestUpsertLastIsIdempotentWhenAlreadyLast(t *testing.T) {
	once, _ := UpsertLast("[alias]\n\tst = status\n", body)
	twice, err := UpsertLast(once, body)

	if err != nil || once != twice {
		t.Fatalf("UpsertLast() changed a file whose block was already last:\n%q\n%q", once, twice)
	}
}

func TestContentAfter(t *testing.T) {
	last, _ := Upsert("[alias]\n", body)
	trailingComment := last + "\n# just a comment\n\n"
	trailingSetting := last + "[user]\n\temail = me@old.example\n"

	if ContentAfter(last) || ContentAfter(trailingComment) || ContentAfter("[user]\n") {
		t.Error("ContentAfter() reported settings where there are none after the block")
	}
	if !ContentAfter(trailingSetting) {
		t.Error("ContentAfter() missed a setting after the block")
	}
}

func TestRemoveRestoresAFileWithoutTrailingNewline(t *testing.T) {
	original := "export EDITOR=vim"
	withBlock, err := Upsert(original, body)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Remove(withBlock)

	if err != nil || got != original {
		t.Fatalf("Remove(Upsert(%q)) = %q, want the original exactly", original, got)
	}
}
