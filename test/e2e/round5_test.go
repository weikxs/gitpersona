package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for customer acceptance round 5.

func TestSetupStepsFollowBrowserHistoryAndOfferNoDefault(t *testing.T) {
	s := newSandbox(t, false)
	s.write(".gitconfig", "[user]\n\tname = Demo Person\n\temail = person@demo.example\n", 0o600)
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(375, 812, false)
	b.open(link)
	adjustSetup(b)

	// R5-05: the identity used outside every directory today is named, and
	// keeping it is a choice.
	if page := b.text("#view"); !strings.Contains(page, "Demo Person <person@demo.example>") {
		t.Errorf("step 1 should name today's global identity:\n%s", page)
	}
	b.eval(`const none = document.querySelector("#view input[type=radio][value='']");
		none.checked = true; none.dispatchEvent(new Event("change"));`, nil)

	// R5-04: Back and Forward move between the steps; a reload keeps them.
	click(b, "#view button.primary")
	b.waitFor("step 2", `document.querySelector("#view h3")?.innerText.startsWith("2.")`)
	b.eval(`history.back()`, nil)
	b.waitFor("step 1 after Back", `document.querySelector("#view h3")?.innerText.startsWith("1.")`)
	b.eval(`history.forward()`, nil)
	b.waitFor("step 2 after Forward", `document.querySelector("#view h3")?.innerText.startsWith("2.")`)
	b.eval(`location.reload()`, nil)
	b.waitFor("step 2 after a reload", `document.querySelector("#view h3")?.innerText.startsWith("2.")`)

	// R5-07: an absolute path is compared like the ~/ form.
	addDir := func(dir string) {
		b.eval(`const input = [...document.querySelectorAll("#view fieldset input[type=text]")].pop();
			input.value = "`+dir+`";
			input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));`, nil)
	}
	addDir("~/code/shared")
	b.waitFor("the added folder", `[...document.querySelectorAll("#view fieldset .mono")].some((e) => e.innerText === "~/code/shared")`)
	addDir(filepath.Join(s.home, "code", "shared") + "/")
	waitStatus(b, "~/code/shared is already in the list")

	click(b, "#view button.primary")
	b.waitFor("the review", `document.querySelector("#view button.primary") && !document.querySelector("#view button.primary").disabled`)
	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")
	if got := s.email("Desktop/scratch"); got != "person@demo.example" {
		t.Errorf("with no default, a repository outside every rule should keep the global identity, got %q", got)
	}
	b.checkClean()
}

func TestAbsolutePathsAreConfirmedAndShownWithTilde(t *testing.T) {
	s := newSandbox(t, true)
	link, _ := s.start("ui")
	b := startBrowser(t)
	b.viewport(1280, 900, false)
	b.open(link)
	goTo(b, "profiles")

	// R5-02: personal's folder typed as an absolute path still asks first.
	setValue(b, "article.card form.row input", filepath.Join(s.home, "code", "fun"))
	b.eval(`document.querySelector("article.card form.row").requestSubmit()`, nil)
	b.waitFor("the move question", `document.getElementById("confirm").open`)
	if q := b.text("#confirm-title"); q != "Move ~/code/fun to work?" {
		t.Errorf("confirmation = %q", q)
	}
	confirmDialog(b)
	waitStatus(b, "~/code/fun now uses profile work")
	if page := b.text("body"); strings.Contains(page, s.home) {
		t.Errorf("the page shows the home path %s", s.home)
	}

	// R5-11: the shell startup files are named before they are written.
	goTo(b, "shells")
	if b.text("#view form button.primary") != "" {
		click(b, "#view form button.primary")
		b.waitFor("the file list", `document.getElementById("confirm").open`)
		if text := b.text("#confirm-text"); !strings.Contains(text, "~/") {
			t.Errorf("the confirmation should list the files: %q", text)
		}
		click(b, `#confirm button[value=cancel]`)
	}
	b.checkClean()
}
