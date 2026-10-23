package e2e

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A veteran's home: hand-made includeIf identities with SSH keys and
// signing, SSH aliases per account, and a repository that differs from its
// folder. Nothing names the personal account except the key and the
// repositories.
func veteranSandbox(t *testing.T) *sandbox {
	s := newSandbox(t, false)
	s.write(".gitconfig", "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/work/\"]\n\tpath = ~/.gitconfig-work\n"+
		"[includeIf \"gitdir/i:~/oss/\"]\n\tpath = ~/.gitconfig-oss\n", 0o600)
	s.write(".gitconfig-work", "[user]\n\tname = Demo Worker\n\temail = worker@demo.example\n\tsigningkey = ~/.ssh/id_work.pub\n"+
		"[gpg]\n\tformat = ssh\n[commit]\n\tgpgsign = true\n[core]\n\tsshCommand = ssh -i ~/.ssh/id_work -o IdentitiesOnly=yes\n", 0o600)
	s.write(".gitconfig-oss", "[user]\n\tname = Demo Personal\n\temail = personal@demo.example\n"+
		"[core]\n\tsshCommand = ssh -i ~/.ssh/id_personal -o IdentitiesOnly=yes\n", 0o600)
	s.write(".ssh/config", "Host github-work\n  HostName github.com\n  IdentityFile ~/.ssh/id_work\n"+
		"Host github-personal\n  HostName github.com\n  IdentityFile ~/.ssh/id_personal\n", 0o600)
	for _, key := range []string{"id_work", "id_work.pub", "id_personal"} {
		s.write(".ssh/"+key, "KEY PLACEHOLDER (never read)\n", 0o600)
	}
	s.repo("work/api", "git@github-work:demo-work/api.git")
	s.repo("work/odd", "git@github-personal:demo-personal/odd.git")
	s.git(filepath.Join(s.home, "work/odd"), "config", "user.email", "personal@demo.example")
	s.repo("oss/tool", "https://github.com/demo-personal/tool.git")
	return s
}

func TestSetupKeepsAVeteransIdentitiesWhereTheyBelong(t *testing.T) {
	s := veteranSandbox(t)

	out := s.run("setup", "--yes", "--no-shell")

	if !strings.Contains(out, "GitPersona is set up") {
		t.Fatalf("setup failed:\n%s", out)
	}
	oss := filepath.Join(s.home, "oss/tool")
	for what, want := range map[string]string{
		"user.email":                             "personal@demo.example",
		"credential.https://github.com.username": "demo-personal",
	} {
		if got := s.git(oss, "config", what); got != want {
			t.Errorf("~/oss/tool %s = %q, want %q (the folder was set up for personal use)\n%s", what, got, want, out)
		}
	}
	if got := s.git(oss, "config", "core.sshCommand"); !strings.Contains(got, "id_personal") {
		t.Errorf("~/oss/tool uses SSH command %q, want the personal key", got)
	}
	odd := filepath.Join(s.home, "work/odd")
	if got := s.git(odd, "config", "credential.https://github.com.username"); got != "demo-personal" {
		t.Errorf("the pinned ~/work/odd pushes as %q", got)
	}
	if got := s.git(odd, "config", "user.signingkey"); strings.Contains(got, "id_work") {
		t.Errorf("~/work/odd would sign with the work key %q", got)
	}
	if got := s.git(filepath.Join(s.home, "work/api"), "config", "user.signingkey"); !strings.Contains(got, "id_work.pub") {
		t.Errorf("work keeps its signing key, got %q", got)
	}

	doctor := s.run("doctor", "--fix")
	for _, bad := range []string{"✗", "overrides", "integration is not installed", "Removed the old"} {
		if strings.Contains(doctor, bad) {
			t.Errorf("right after setup, doctor --fix should find nothing to do (%q):\n%s", bad, doctor)
		}
	}
	gitconfig := s.git(s.home, "config", "--file", filepath.Join(s.home, ".gitconfig"), "--list")
	if strings.Contains(gitconfig, "includeif.gitdir") {
		t.Errorf("both old rules are taken over and should be gone:\n%s", gitconfig)
	}
}

func TestALeftOutIdentityCanBeGivenAnAccountOnTheReadyPage(t *testing.T) {
	s := newSandbox(t, false)
	s.write(".gitconfig", "[user]\n\tname = Someone\n"+
		"[includeIf \"gitdir:~/old/personal/\"]\n\tpath = ~/.gitconfig-personal\n"+
		"[includeIf \"gitdir:~/side/\"]\n\tpath = ~/.gitconfig-side\n", 0o600)
	s.write(".gitconfig-side", "[user]\n\tname = Side Person\n\temail = side@demo.example\n", 0o600)
	s.repo("side/thing", "")
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(375, 812, false)
	b.open(link)
	b.waitFor("the ready page", `document.querySelector("#view h3")?.innerText === "Your setup is ready"`)

	page := b.text("#view")
	for _, want := range []string{"Side Person <side@demo.example> · ~/side", "Your rule keeps applying there."} {
		if !strings.Contains(page, want) {
			t.Errorf("the ready page should say %q:\n%s", want, page)
		}
	}

	// The preview follows the boxes.
	b.eval(`document.querySelector("#view details.change").open = true`, nil)
	b.waitFor("the preview", `document.querySelectorAll("#view details.change details").length > 0`)
	var before int
	b.eval(`return document.querySelectorAll("#view details.change details").length`, &before)
	b.eval(`const box = [...document.querySelectorAll("#view fieldset label.check")].find((l) => l.innerText.startsWith("Load GitPersona"))?.querySelector("input");
		box.checked = false; box.dispatchEvent(new Event("change"));`, nil)
	b.waitFor("the preview without the shell file", `document.querySelectorAll("#view details.change details").length === `+strconv.Itoa(before-1))

	b.eval(`const select = document.querySelector('#view select[aria-label="Account for side@demo.example"]');
		select.value = [...select.options].find((o) => o.text === "Use it for demo-work").value;
		select.dispatchEvent(new Event("change"));`, nil)
	// demo-personal already has the ~/old/personal identity, so only demo-work is offered.
	b.waitFor("the identity in demo-work's profile", `document.querySelector("#view").innerText.includes("~/side · your existing setup")`)
	checkFits(b, "the ready page with a left-out identity")
	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")

	if got := s.email("side/thing"); got != "side@demo.example" {
		t.Errorf("~/side/thing commits as %q, want side@demo.example", got)
	}
	if got := s.git(s.home, "config", "--file", filepath.Join(s.home, ".gitconfig"), "--get", "includeIf.gitdir:~/side/.path"); got != "" {
		t.Errorf("the ~/side rule is taken over and should be gone, still %q", got)
	}
	b.checkClean()
}
