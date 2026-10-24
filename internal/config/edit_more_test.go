package config

import (
	"strings"
	"testing"
)

const withPinAfterProfiles = `version = 1

[[profile]]
id      = "acme"
account = "acme-alex"
name    = "Alex (Acme)"
email   = "1000002+acme-alex@users.noreply.github.com"


[[pin]]
repo    = "~/Projects/acme/tool"
profile = "acme"
`

func TestAddProfileWritesOptionalKeysOnlyWhenSet(t *testing.T) {
	p := Profile{
		ID: "client", Provider: "gitlab", Host: "gitlab.demo.example", Account: "client-alex",
		Name: "Alex", Email: "alex@client.example", SSHKey: "~/.ssh/id_client",
		Signing: Signing{Method: SigningSSH},
	}

	got, err := AddProfile("version = 1\n", p)

	for _, want := range []string{
		`provider = "gitlab"`, `host    = "gitlab.demo.example"`, `ssh_key = "~/.ssh/id_client"`,
		`signing = { method = "ssh" }`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AddProfile() output lacks %q:\n%s", want, got)
		}
	}
	if err != nil {
		t.Fatalf("AddProfile() error: %v", err)
	}
	assertParses(t, got)
}

func TestAddProfileKeepsABlankLineBeforeWhatFollows(t *testing.T) {
	p := Profile{ID: "client", Account: "client-alex", Name: "Alex", Email: "alex@client.example"}

	got, err := AddProfile(withPinAfterProfiles, p)

	if err != nil {
		t.Fatalf("AddProfile() error: %v", err)
	}
	newProfile := strings.Index(got, `id      = "client"`)
	pin := strings.Index(got, "[[pin]]")
	if newProfile < 0 || pin < newProfile || !strings.Contains(got[newProfile:pin], "alex@client.example\"\n\n") {
		t.Fatalf("the new profile should sit between the old one and the pin, blank line before the pin:\n%s", got)
	}
	assertParses(t, got)
}

func TestAddProfileToAnEmptyFileStartsTheFile(t *testing.T) {
	p := Profile{ID: "client", Account: "client-alex", Name: "Alex", Email: "alex@client.example"}

	got, err := AddProfile("", p)

	if err != nil || !strings.Contains(got, `id      = "client"`) {
		t.Fatalf("AddProfile() = %q, %v", got, err)
	}
}

func TestProfileEditsRejectUnknownProfiles(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{"SetProfileField", func() error { _, err := SetProfileField(commented, "ghost", "name", "x"); return err }},
		{"RemoveProfile", func() error { _, err := RemoveProfile(commented, "ghost"); return err }},
		{"SetProfileSigning", func() error {
			_, err := SetProfileSigning(commented, "ghost", Signing{Method: SigningSSH})
			return err
		}},
		{"AddDir", func() error { _, err := AddDir(commented, "ghost", "~/x", editOpts); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()

			if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
				t.Fatalf("error = %v, want one naming the unknown profile", err)
			}
		})
	}
}

func TestSetProfileSigningRefusesASigningTable(t *testing.T) {
	text := strings.Replace(commented, "[[profile]]\nid      = \"acme\"", "[[profile]]\nid      = \"acme\"\nsigning = \"ssh\"", 1)

	_, err := SetProfileSigning(text, "acme", Signing{Method: SigningSSH})

	if err == nil || !strings.Contains(err.Error(), "by hand") {
		t.Fatalf("error = %v, want a request to edit by hand", err)
	}
}

func TestSetProfileSigningWritesAMethodWithoutAKey(t *testing.T) {
	got, err := SetProfileSigning(commented, "acme", Signing{Method: SigningGPG})

	assertEdit(t, got, err, `signing = { method = "gpg" }`)
}

func TestSetDefaultClearingAnAbsentDefaultChangesNothing(t *testing.T) {
	text := "version = 1\n"

	got, err := SetDefault(text, "")

	if err != nil || got != text {
		t.Fatalf("SetDefault() = %q, %v; want the text unchanged", got, err)
	}
}

func TestSetProfileFieldClearingAnAbsentKeyChangesNothing(t *testing.T) {
	got, err := SetProfileField(commented, "globex", "ssh_key", "")

	if err != nil || got != commented {
		t.Fatalf("SetProfileField() changed the text: %v", err)
	}
}

func TestDirEditsRejectDirsThatAreNotArrays(t *testing.T) {
	text := strings.Replace(commented, `dirs    = ["~/Projects/acme"]`, `dirs    = "~/Projects/acme"`, 1)

	_, addErr := AddDir(text, "acme", "~/x", editOpts)
	_, _, removeErr := RemoveDir(text, "~/Projects/acme", editOpts)

	if addErr == nil || removeErr == nil {
		t.Fatalf("AddDir error = %v, RemoveDir error = %v; want both to refuse", addErr, removeErr)
	}
}

func TestRemoveDirRejectsARelativePath(t *testing.T) {
	if _, _, err := RemoveDir(commented, "relative/dir", editOpts); err == nil {
		t.Fatal("RemoveDir() of a relative path succeeded, want an error")
	}
}

func TestPinEditsHandleOddExistingPins(t *testing.T) {
	const noProfile = "version = 1\n\n[[pin]]\nrepo = \"~/Projects/acme/tool\"\n"
	const noRepo = "version = 1\n\n[[pin]]\nprofile = \"acme\"\n\n[[pin]]\nrepo = \"~/Projects/acme/tool\"\nprofile = \"acme\"\n"

	_, noProfileErr := AddPin(noProfile, "~/Projects/acme/tool", "acme", editOpts)
	_, relativeErr := RemovePin(noRepo, "relative/repo", editOpts)
	got, noRepoErr := AddPin(noRepo, "~/Projects/acme/tool", "globex", editOpts)

	if noProfileErr == nil {
		t.Error("AddPin() on a pin without a profile key succeeded, want an error")
	}
	if relativeErr == nil {
		t.Error("RemovePin() of a relative path succeeded, want an error")
	}
	if noRepoErr != nil || !strings.Contains(got, `profile = "globex"`) {
		t.Errorf("AddPin() = %q, %v; want the pin that has a repo updated", got, noRepoErr)
	}
}

func TestAddPinRepairsAMalformedProfileValue(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"value missing", "profile = "},
		{"not a string", "profile = oops"},
		{"unterminated single quote", "profile = 'acme"},
		{"unterminated double quote", `profile = "acme`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := "version = 1\n\n[[pin]]\nrepo = \"~/Projects/acme/tool\"\n" + tc.line + "\n"

			got, err := AddPin(text, "~/Projects/acme/tool", "globex", editOpts)

			if err != nil || !strings.Contains(got, `profile = "globex"`) {
				t.Fatalf("AddPin() = %q, %v; want the value replaced", got, err)
			}
		})
	}
}

func TestStringValuesWithEscapesAndSingleQuotesAreRead(t *testing.T) {
	text := "version = 1\n\n[[profile]]\nid = 'acme'\nname = \"A \\\"quoted\\\" name\"\naccount = \"acme-alex\"\nemail = \"a@b.example\"\n"

	got, err := SetProfileField(text, "acme", "name", "Alex")

	assertEdit(t, got, err, `name = "Alex"`)
}

func TestValidateRejectsPinsWithControlCharacters(t *testing.T) {
	cfg := Config{Version: CurrentVersion, Profiles: []Profile{validProfile("personal")},
		Pins: []Pin{{Repo: "~/Projects/a\nb", Profile: "personal"}}}

	err := cfg.Validate(home, false)

	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("Validate() error = %v, want one about a control character", err)
	}
}

func TestShellIntegrationOffOnlyWhenExplicitlyDisabled(t *testing.T) {
	off, on := false, true
	tests := []struct {
		name string
		set  *bool
		want bool
	}{
		{"unset", nil, false},
		{"enabled", &on, false},
		{"disabled", &off, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Config{ShellIntegration: tc.set}).ShellIntegrationOff(); got != tc.want {
				t.Errorf("ShellIntegrationOff() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidAccountAndValidHostFollowTheProfileSyntax(t *testing.T) {
	tests := []struct {
		value                 string
		wantAccount, wantHost bool
	}{
		{"octo-alex", true, true},
		{"gitlab.demo.example:8443", false, true},
		{"it's;me", false, false},
		{"", false, false},
	}
	for _, tc := range tests {
		if got := ValidAccount(tc.value); got != tc.wantAccount {
			t.Errorf("ValidAccount(%q) = %v, want %v", tc.value, got, tc.wantAccount)
		}
		if got := ValidHost(tc.value); got != tc.wantHost {
			t.Errorf("ValidHost(%q) = %v, want %v", tc.value, got, tc.wantHost)
		}
	}
}
