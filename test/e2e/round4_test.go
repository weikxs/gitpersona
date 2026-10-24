package e2e

import (
	"strings"
	"testing"
)

// Regression tests for customer acceptance round 4.

func TestSetupHandlesOrganizationsEnterAndLeftOutProfiles(t *testing.T) {
	s := newSandbox(t, false)
	s.repo("code/client/api", "https://github.com/demo-org/api.git")
	s.repo("code/client/web", "git@github.com:demo-org/web.git")
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(375, 812, false)
	b.open(link)
	adjustSetup(b)

	// R4-07: a left-out profile with an invalid email must not block Next.
	b.eval(`const sets = document.querySelectorAll("#view form > fieldset");
		const email = sets[1].querySelector("input[type=email]");
		email.value = "not-an-email"; email.dispatchEvent(new Event("change"));
		const box = sets[1].querySelector("input[type=checkbox]");
		box.checked = false; box.dispatchEvent(new Event("change"));`, nil)
	click(b, "#view button.primary")
	b.waitFor("step 2 despite the left-out profile", `document.querySelector("#view h3")?.innerText.startsWith("2.")`)

	// R4-01: an organization's folder is offered for the user to assign.
	setValue(b, "#view .card input", "~/code")
	click(b, "#view .card button")
	b.waitFor("the organization's folder", `document.getElementById("view").innerText.includes("demo-org")`)
	b.eval(`const select = document.querySelector("#view .card select");
		select.value = select.options[1].value; select.dispatchEvent(new Event("change"));`, nil)
	b.waitFor("the folder under work", `[...document.querySelectorAll("#view fieldset .mono")].some((e) => e.innerText === "~/code/client")`)

	// R4-03: Enter in "Add a directory" adds it and stays on step 2.
	b.eval(`const input = [...document.querySelectorAll("#view fieldset input[type=text]")].pop();
		input.value = "~/code/enter-test/";
		input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));`, nil)
	b.waitFor("the added folder", `[...document.querySelectorAll("#view fieldset .mono")].some((e) => e.innerText === "~/code/enter-test")`)
	if heading := b.text("#view h3"); !strings.HasPrefix(heading, "2.") {
		t.Fatalf("Enter in a field moved the wizard on to %q", heading)
	}

	click(b, "#view button.primary")
	b.waitFor("the review", `document.querySelector("#view button.primary") && !document.querySelector("#view button.primary").disabled`)
	// R4-09: diffs show the home as ~, never its absolute path.
	if review := b.text("#view"); strings.Contains(review, s.home) {
		t.Errorf("the review shows the absolute home path %s", s.home)
	}
	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")
	if got := s.email("code/client/api"); got != "worker@demo.example" {
		t.Errorf("the organization's repository commits as %q", got)
	}
	b.checkClean()
}

func TestEverydayEditsConfirmMovesAndRefuseMissingKeys(t *testing.T) {
	s := newSandbox(t, true)
	link, _ := s.start("ui")
	b := startBrowser(t)
	b.viewport(1280, 900, false)
	b.open(link)
	goTo(b, "profiles")

	// R4-04: adding personal's folder to work asks before moving it.
	setValue(b, "article.card form.row input", "~/code/fun/")
	b.eval(`document.querySelector("article.card form.row").requestSubmit()`, nil)
	b.waitFor("the move question", `document.getElementById("confirm").open`)
	if q := b.text("#confirm-title"); !strings.Contains(q, "Move ~/code/fun to work") {
		t.Errorf("confirmation = %q", q)
	}
	click(b, `#confirm button[value=cancel]`)
	b.waitFor("the dialog to close", `!document.getElementById("confirm").open`)
	if got := s.email("code/fun/game"); got == "worker@demo.example" {
		t.Error("cancelling the move still moved the folder")
	}

	// R4-02: a signing key that does not exist is refused with a clear reason.
	click(b, "article.card .actions button:not(.danger)")
	b.waitFor("the edit form", `document.querySelector("article.card form.stack select")`)
	b.eval(`const form = document.querySelector("article.card form.stack");
		form.querySelector("select").value = "ssh";
		form.querySelectorAll("fieldset input")[1].value = "~/.ssh/work_signing.pub";
		form.requestSubmit();`, nil)
	waitStatus(b, "signing key ~/.ssh/work_signing.pub does not exist")

	// R4-06: a restore is reported as a restore.
	click(b, "article.card form.stack button[type=button]") // Cancel
	setValue(b, "#view form.default-form select", "work")
	b.eval(`document.querySelector("#view form.default-form").requestSubmit()`, nil)
	waitStatus(b, "Default profile is now work")
	goTo(b, "backups")
	b.waitFor("the backups", `document.querySelector("#view ul.rows li button")`)
	click(b, "#view ul.rows li button")
	confirmDialog(b)
	waitStatus(b, "Restored")
	if headline := b.text("#status .notice"); strings.HasPrefix(headline, "Nothing to change") {
		t.Errorf("a restore was reported as %q", headline)
	}
	b.checkClean()
}

func TestASecondPageIsRefused(t *testing.T) {
	s := newSandbox(t, true)
	s.start("ui")
	cmdOut := s.run("ui", "--no-open")
	if !strings.Contains(cmdOut, "already open") {
		t.Errorf("a second page should be refused, got:\n%s", cmdOut)
	}
}
