package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAnIdentitySigningInTheGlobalSSHFormatStaysSSH(t *testing.T) {
	s := newSandbox(t, false)
	s.write(".gitconfig", "[gpg]\n\tformat = ssh\n[gpg \"ssh\"]\n\tdefaultKeyCommand = ssh-add -L\n"+
		"[includeIf \"gitdir:~/work/\"]\n\tpath = ~/.gc-work\n", 0o600)
	s.write(".gc-work", "[user]\n\tname = Demo Worker\n\temail = worker@demo.example\n\tsigningkey = ~/.ssh/id_work.pub\n"+
		"[commit]\n\tgpgsign = true\n[credential \"https://github.com\"]\n\tusername = demo-work\n", 0o600)
	s.write(".ssh/id_work.pub", "KEY PLACEHOLDER (never read)\n", 0o600)
	s.repo("work/a", "git@github.com:demo-work/a.git")

	out := s.run("setup", "--yes", "--no-shell")

	if strings.Contains(out, "signs commits (gpg)") {
		t.Errorf("setup read SSH signing as GPG:\n%s", out)
	}
	if got := s.git(filepath.Join(s.home, "work/a"), "config", "gpg.format"); got != "ssh" {
		t.Errorf("~/work signs with format %q, want ssh as before setup", got)
	}
	if doctor := s.run("doctor"); strings.Contains(doctor, "✗") {
		t.Errorf("doctor after setup:\n%s", doctor)
	}
}

func TestSectionsThatNeedAConfigurationPointToSetup(t *testing.T) {
	s := newSandbox(t, false)
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(375, 812, false)
	b.open(link)
	b.waitFor("the ready page", `document.querySelector("#view h3")?.innerText === "Your setup is ready"`)
	checkFits(b, "the ready page")

	for _, section := range []string{"profiles", "apply"} {
		b.eval(`location.hash = "`+section+`"`, nil)
		b.waitFor(section, `document.querySelector("#view a[href='#setup']") !== null`)
		if page := b.text("#view"); !strings.Contains(page, "No configuration yet. Set GitPersona up first") {
			t.Errorf("%s without a configuration shows:\n%s", section, page)
		}
	}
	click(b, "#view a[href='#setup']")
	b.waitFor("back on the ready page", `document.querySelector("#view h3")?.innerText === "Your setup is ready"`)
	b.checkClean()
}
