package config

import (
	"strings"
	"testing"
)

func TestValidAccountAndHostAcceptOnlyWhatGitHostsAllow(t *testing.T) {
	for _, tc := range []struct {
		value   string
		account bool
		host    bool
	}{
		{"octo-alex", true, true},
		{"demo.example", true, true},
		{"gitlab.demo.example:8443", false, true},
		{"-leading-dash", false, false},
		{"has space", false, false},
		{"under_score", true, false},
		{"", false, false},
	} {
		if got := ValidAccount(tc.value); got != tc.account {
			t.Errorf("ValidAccount(%q) = %v, want %v", tc.value, got, tc.account)
		}
		if got := ValidHost(tc.value); got != tc.host {
			t.Errorf("ValidHost(%q) = %v, want %v", tc.value, got, tc.host)
		}
	}
}

func TestShellIntegrationIsOffOnlyWhenExplicitlyTurnedOff(t *testing.T) {
	on, off := true, false
	for name, tc := range map[string]struct {
		setting *bool
		want    bool
	}{
		"not set":      {nil, false},
		"set to true":  {&on, false},
		"set to false": {&off, true},
	} {
		if got := (Config{ShellIntegration: tc.setting}).ShellIntegrationOff(); got != tc.want {
			t.Errorf("%s: ShellIntegrationOff() = %v, want %v", name, got, tc.want)
		}
	}
}

func TestSetProfileSigningWritesAKeyOrJustTheMethod(t *testing.T) {
	text := "version = 1\n\n[[profile]]\nid = \"demo\"\naccount = \"octo-alex\"\n"

	withKey, err := SetProfileSigning(text, "demo", Signing{Method: SigningSSH, Key: "~/.ssh/demo.pub"})
	noKey, errNone := SetProfileSigning(text, "demo", Signing{Method: "none"})

	if err != nil || !strings.Contains(withKey, `signing = { method = "ssh", key = "~/.ssh/demo.pub" }`) {
		t.Errorf("with a key: %q, %v", withKey, err)
	}
	if errNone != nil || !strings.Contains(noKey, `signing = { method = "none" }`) {
		t.Errorf("without a key: %q, %v", noKey, errNone)
	}
}

func TestScanStringReadsBothTOMLStringFormsAndRejectsBrokenOnes(t *testing.T) {
	for name, tc := range map[string]struct {
		text    string
		want    string
		wantEnd int
		wantErr bool
	}{
		"basic string":         {`"a b" rest`, "a b", 5, false},
		"escaped quote":        {`"a\"b"`, `a"b`, 6, false},
		"literal string":       {`'C:\x' rest`, `C:\x`, 6, false},
		"unterminated basic":   {`"abc`, "", 0, true},
		"unterminated literal": {`'abc`, "", 0, true},
		"not a string":         {`abc`, "", 0, true},
		"nothing left":         {``, "", 0, true},
	} {
		got, end, err := scanString(tc.text, 0)
		if (err != nil) != tc.wantErr || got != tc.want || (!tc.wantErr && end != tc.wantEnd) {
			t.Errorf("%s: scanString(%q) = %q, %d, %v; want %q, %d, error=%v", name, tc.text, got, end, err, tc.want, tc.wantEnd, tc.wantErr)
		}
	}
}
