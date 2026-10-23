package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// onboardingBudget is the time from starting setup to a working
// configuration that the one-click setup must stay within.
const onboardingBudget = 30 * time.Second

func TestOneClickSetupOrganizesWhatTheUserHas(t *testing.T) {
	s := newSandbox(t, false)
	// An organization's repository reached through demo-work's SSH alias.
	s.repo("code/client/api", "git@github-work:demo-org/api.git")
	s.repo("code/work/tools", "git@github-work:demo-work/tools.git")
	s.write(".ssh/config", "Host github-work\n  HostName github.com\n  IdentityFile ~/.ssh/id_work\n", 0o600)
	s.write(".ssh/id_work", "KEY PLACEHOLDER (never read)\n", 0o600)
	// A repository in work's folder that sets demo-personal's email.
	s.repo("code/work/side", "https://github.com/demo-personal/side.git")
	s.git(filepath.Join(s.home, "code/work/side"), "config", "user.email", "person@demo.example")

	started := time.Now()
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(375, 812, false)
	b.open(link)
	b.waitFor("the ready page", `document.querySelector("#view h3")?.innerText === "Your setup is ready"`)
	ready := time.Since(started)

	page := b.text("#view")
	for _, want := range []string{"~/code/work", "~/code/client", "SSH key ~/.ssh/id_work", "~/old/personal",
		"Pin ~/code/work/side to demo-personal", "Load GitPersona in", "Remove the includeIf rules for ~/old/personal"} {
		if !strings.Contains(page, want) {
			t.Errorf("the ready page should show %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, s.home) {
		t.Errorf("the ready page shows the home path %s", s.home)
	}
	checkFits(b, "the ready page")

	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")
	took := time.Since(started)
	t.Logf("one-click setup took %s (page ready after %s)", took.Round(100*time.Millisecond), ready.Round(100*time.Millisecond))
	if took > onboardingBudget {
		t.Errorf("one-click setup took %s, want at most %s", took.Round(time.Second), onboardingBudget)
	}

	for rel, want := range map[string]string{
		"code/work/api":      "worker@demo.example",
		"code/client/api":    "worker@demo.example",
		"code/fun/game":      "person@demo.example",
		"old/personal/notes": "person@demo.example",
	} {
		if got := s.email(rel); got != want {
			t.Errorf("%s commits as %q, want %q", rel, got, want)
		}
	}
	side := filepath.Join(s.home, "code/work/side")
	if got := s.git(side, "config", "credential.https://github.com.username"); got != "demo-personal" {
		t.Errorf("the pinned repository pushes as %q, want demo-personal", got)
	}
	if got := s.git(filepath.Join(s.home, "code/client/api"), "config", "core.sshCommand"); !strings.Contains(got, "id_work") {
		t.Errorf("work's repositories should use work's SSH key, got %q", got)
	}
	if doctor := s.run("doctor"); strings.Contains(doctor, "includeIf identity rules") || strings.Contains(doctor, "✗") {
		t.Errorf("right after one-click setup doctor should have nothing to fix:\n%s", doctor)
	}
	b.checkClean()
}

func TestCommandLineSetupInOneStep(t *testing.T) {
	s := newSandbox(t, false)

	out := s.run("setup", "--yes", "--no-shell")

	for _, want := range []string{"~/code/work  (all 3 repositories belong to demo-work)", "GitPersona is set up"} {
		if !strings.Contains(out, want) {
			t.Errorf("setup should say %q:\n%s", want, out)
		}
	}
	if got := s.email("code/work/api"); got != "worker@demo.example" {
		t.Errorf("code/work/api commits as %q", got)
	}
	if strings.Contains(out, s.home) {
		t.Errorf("setup output shows the home path %s:\n%s", s.home, out)
	}
}

func TestChoicesTurnedOffOnTheReadyPageAllStayOff(t *testing.T) {
	s := newSandbox(t, false)
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(1280, 900, false)
	b.open(link)
	b.waitFor("the ready page", `document.querySelector("#view h3")?.innerText === "Your setup is ready"`)

	// Untick every choice one after the other: old rules, then the shell.
	b.eval(`for (const box of document.querySelectorAll("#view fieldset input[type=checkbox]")) {
		box.checked = false; box.dispatchEvent(new Event("change"));
	}`, nil)
	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")

	if got := s.git(s.home, "config", "--file", filepath.Join(s.home, ".gitconfig"), "--get", "includeIf.gitdir:~/old/personal/.path"); got == "" {
		t.Error("the old includeIf rule was removed although that was unticked")
	}
	for _, f := range []string{".zshrc", ".zshenv", ".bashrc", ".bash_profile", ".config/fish/conf.d/gitpersona.fish"} {
		if data, err := os.ReadFile(filepath.Join(s.home, f)); err == nil && strings.Contains(string(data), "gitpersona") {
			t.Errorf("%s loads GitPersona although the shell was unticked", f)
		}
	}
	b.checkClean()
}
