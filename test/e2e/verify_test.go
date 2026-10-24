package e2e

import (
	"strings"
	"testing"
)

// The Repositories page shows, per profile, who commits and who pushes in
// every repository, and opens the ones that would use another identity.
func TestRepositoriesPageShowsWhoCommitsAndPushesWhere(t *testing.T) {
	s := newSandbox(t, true)
	s.run("apply")
	// One repository commits as someone else; another chooses its own push account.
	s.git(s.home+"/code/work/web", "config", "user.email", "someone@else.example")
	s.git(s.home+"/code/fun/game", "config", "credential.https://github.com.username", "demo-work")

	link, _ := s.start("ui")
	b := startBrowser(t)
	b.viewport(1280, 900, false)
	b.open(link)
	goTo(b, "repositories")
	b.waitFor("the report", `document.querySelector(".summary-badges")`)

	// textContent: notes are folded away, and their text still counts.
	var page string
	b.eval(`return document.querySelector("#view").textContent`, &page)
	for _, want := range []string{
		"1 with problems", "1 with notes", "2 unconfirmed",
		"work · demo-work · Demo Worker <worker@demo.example>",
		"~/code/work/web", "someone@else.example",
		"pushes as demo-work",
		"chosen by credential.username in the repository",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not say %q:\n%s", want, page)
		}
	}
	var openProblem bool
	b.eval(`return [...document.querySelectorAll("details.repo[open]")].some((d) => d.innerText.includes("~/code/work/web"))`, &openProblem)
	if !openProblem {
		t.Error("the repository with a problem should be open")
	}
	b.screenshot("repositories")

	click(b, `.filters button:nth-child(2)`) // Problems
	var shown int
	b.eval(`return document.querySelectorAll("details.repo").length`, &shown)
	if shown != 1 {
		t.Errorf("the Problems filter shows %d repositories, want 1", shown)
	}
	b.viewport(375, 812, true)
	checkFits(b, "repositories at 375px (dark)")
	b.checkClean()
}
