package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// Rules setup cannot turn into profiles keep doing what they did: an
// identity no account is tied to, x509 signing, and a rule on a branch.
func TestRulesSetupDoesNotTakeOverKeepWorking(t *testing.T) {
	s := newSandbox(t, false)
	s.write(".gitconfig", "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/oss/\"]\n\tpath = ~/.gc-oss\n"+
		"[includeIf \"gitdir:~/x509/\"]\n\tpath = ~/.gc-x509\n"+
		"[includeIf \"onbranch:release/**\"]\n\tpath = ~/.gc-release\n", 0o600)
	s.write(".gc-oss", "[user]\n\tname = Demo OSS\n\temail = oss@demo.example\n", 0o600)
	s.write(".gc-x509", "[user]\n\tname = Demo Person\n\temail = person@demo.example\n\tsigningkey = 0xDEADBEEF\n"+
		"[gpg]\n\tformat = x509\n[commit]\n\tgpgsign = true\n[credential \"https://github.com\"]\n\tusername = demo-personal\n", 0o600)
	s.write(".gc-release", "[user]\n\temail = release@demo.example\n", 0o600)
	s.repo("oss/tool", "https://github.com/demo-personal/tool.git")
	s.repo("x509/app", "https://github.com/demo-personal/app.git")

	out := s.run("setup", "--yes", "--no-shell")

	if !strings.Contains(out, "GitPersona is set up") {
		t.Fatalf("setup failed:\n%s", out)
	}
	oss, x509 := filepath.Join(s.home, "oss/tool"), filepath.Join(s.home, "x509/app")
	if got := s.git(oss, "config", "user.email"); got != "oss@demo.example" {
		t.Errorf("~/oss commits as %q after setup, want oss@demo.example as before", got)
	}
	if got := s.git(x509, "config", "gpg.format"); got != "x509" {
		t.Errorf("~/x509 signs with format %q after setup, want x509 as before", got)
	}
	s.git(oss, "checkout", "-q", "-b", "release/1")
	if got := s.git(oss, "config", "user.email"); got != "release@demo.example" {
		t.Errorf("on a release branch ~/oss commits as %q, want release@demo.example from the branch rule", got)
	}
	whoami := s.runIn(oss, "whoami", "--check")
	if !strings.Contains(whoami, "from your own includeIf rule") {
		t.Errorf("whoami should explain the user's own rule:\n%s", whoami)
	}
	if doctor := s.run("doctor"); strings.Contains(doctor, "✗") {
		t.Errorf("doctor after setup:\n%s", doctor)
	}
}
