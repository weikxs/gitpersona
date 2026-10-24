package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The README's pictures are drawn from demo homes, never from the machine
// that makes them: every section of the console in English and Chinese,
// light and dark, and the terminal. They run only with GITPERSONA_SHOTS set
// (make screenshots). No picture may show a local path.

var languages = []string{"en", "zh"}

func shotsWanted(t *testing.T) {
	t.Helper()
	if os.Getenv("GITPERSONA_SHOTS") == "" {
		t.Skip("set GITPERSONA_SHOTS to a folder to draw the README pictures")
	}
}

func suffix(dark bool) string {
	if dark {
		return "-dark"
	}
	return "-light"
}

// useLanguage makes the page show lang from its next load on.
func useLanguage(b *browser, lang string) {
	b.t.Helper()
	b.eval(fmt.Sprintf(`sessionStorage.setItem("gitpersona-lang", %q)`, lang), nil)
}

// pageShot checks that the page shows no local path and saves it as name.
func pageShot(b *browser, s *sandbox, name string) {
	b.t.Helper()
	var text string
	b.eval(`return document.body.innerText`, &text)
	checkNoLocalPaths(b.t, s, name, text)
	b.screenshot(name)
}

func TestREADMEScreenshotsOfSetup(t *testing.T) {
	shotsWanted(t)
	s := newSandbox(t, false)
	s.useDemoTools()
	s.repo("code/work/tools", "git@github-work:demo-work/tools.git")
	s.repo("code/client/portal", "git@github-work:demo-org/portal.git")
	s.write(".ssh/config", "Host github-work\n  HostName github.com\n  IdentityFile ~/.ssh/id_work\n", 0o600)
	s.write(".ssh/id_work", "KEY PLACEHOLDER (never read)\n", 0o600)

	// What setup proposes in the terminal, before anything is written.
	term := startBrowser(t)
	shootTerminal(term, s, "terminal-setup", "$ gitpersona setup\n"+strings.TrimSpace(s.run("setup")))
	link, _ := s.start("setup", "--web")
	br := startBrowser(t)
	br.viewport(1280, 860, false)
	br.open(link)
	base, _, _ := strings.Cut(link, "?")
	for _, lang := range languages {
		useLanguage(br, lang)
		for _, dark := range []bool{false, true} {
			br.viewport(1280, 860, dark)
			// A new address each time, so the page loads again in the language.
			br.call("Page.navigate", map[string]any{"url": base + "?shot=setup-" + lang + suffix(dark) + "#setup"})
			br.waitFor("the ready page", `document.querySelector("#view form h3")`)
			pageShot(br, s, "setup-"+lang+suffix(dark))
		}
	}

	// The terminal: every line is real output from the demo home.
	click(br, "#view button.primary")
	br.waitFor("setup to finish", `document.getElementById("status").innerText.trim() !== ""`)
	shootTerminal(term, s, "terminal", strings.Join([]string{
		"$ cd ~/code/work/api && gitpersona whoami",
		strings.TrimSpace(s.runIn(filepath.Join(s.home, "code/work/api"), "whoami")),
		"",
		"$ cd ~/code/fun/game && gitpersona whoami",
		strings.TrimSpace(s.runIn(filepath.Join(s.home, "code/fun/game"), "whoami")),
		"",
		"$ gitpersona verify",
		strings.TrimSpace(s.run("verify")),
	}, "\n"))
}

// dashboardConfig has an account with two identities (two author names),
// as when folders commit under different names, and another account.
const dashboardConfig = `version = 1

[[profile]]
id      = "work"
account = "demo-work"
name    = "Demo Worker"
email   = "worker@demo.example"
dirs    = ["~/code/work"]

[[profile]]
id      = "work-infra"
account = "demo-work"
name    = "demo_worker"
email   = "worker@demo.example"
dirs    = ["~/code/infra"]

[[profile]]
id      = "personal"
account = "demo-personal"
name    = "Demo Person"
email   = "102+demo-personal@users.noreply.github.com"
dirs    = ["~/code/fun"]
`

// consoleSections are the console's sections with what each waits for
// before its picture is taken.
var consoleSections = []struct{ hash, name, ready string }{
	{"overview", "overview", `document.querySelectorAll(".stat.pending").length === 0 && document.querySelector(".stat")`},
	{"profiles", "accounts", `document.querySelector(".persona")`},
	{"repositories", "repositories", `document.querySelector(".summary-badges")`},
	{"overview-health", "health", `document.querySelector("#overview-health li")`},
	{"shells", "shells", `document.querySelector("#view .card dl, #view .card ul")`},
	{"backups", "backups", `document.querySelector("#view ul.rows li")`},
}

func TestREADMEScreenshotsOfTheConsole(t *testing.T) {
	shotsWanted(t)
	s := newSandbox(t, false)
	s.useDemoTools()
	s.write(".config/gitpersona/config.toml", dashboardConfig, 0o600)
	s.repo("code/infra/terraform", "https://github.com/demo-work/terraform.git")
	s.repo("code/infra/dns", "https://github.com/demo-work/dns.git")
	s.run("apply")
	s.run("install-shell", "--shell", "zsh")
	// One repository chooses its own push account, another its own author,
	// for the check to show.
	s.git(filepath.Join(s.home, "code/fun/game"), "config", "credential.https://github.com.username", "demo-work")
	s.git(filepath.Join(s.home, "code/infra/dns"), "config", "user.email", "someone@else.example")

	term := startBrowser(t)
	shootTerminal(term, s, "terminal-doctor", "$ gitpersona doctor\n"+strings.TrimSpace(s.run("doctor")))
	shootTerminal(term, s, "terminal-verify", "$ gitpersona verify\n"+strings.TrimSpace(s.run("verify")))

	link, _ := s.start("ui")
	br := startBrowser(t)
	br.viewport(1280, 900, false)
	br.open(link)
	base, _, _ := strings.Cut(link, "?")
	shootSections := func(prefix string, sections ...string) {
		for _, lang := range languages {
			useLanguage(br, lang)
			for _, dark := range []bool{false, true} {
				br.viewport(1280, 900, dark)
				br.call("Page.navigate", map[string]any{"url": base + "?shot=" + prefix + lang + suffix(dark) + "#overview"})
				br.waitFor("the overview", consoleSections[0].ready)
				for _, name := range sections {
					shootSection(br, s, name, lang, dark)
				}
			}
		}
	}
	shootSections("", "overview", "accounts", "repositories", "health", "shells", "backups")

	// A change waiting to be applied, for Review & apply.
	s.write(".config/gitpersona/config.toml", strings.Replace(dashboardConfig,
		`dirs    = ["~/code/fun"]`, `dirs    = ["~/code/fun", "~/code/side"]`, 1), 0o600)
	shootSections("apply-", "apply")

	br.viewport(390, 844, false)
	goTo(br, "overview")
	br.waitFor("the numbers", consoleSections[0].ready)
	checkFits(br, "the overview on a phone")
	br.checkClean()
}

// shootSection opens one section of the console and saves its picture.
func shootSection(br *browser, s *sandbox, name, lang string, dark bool) {
	br.t.Helper()
	if name == "apply" {
		goTo(br, "apply")
		br.waitFor("the changes", `document.querySelector("#view details.change")`)
		pageShot(br, s, "apply-"+lang+suffix(dark))
		return
	}
	for _, section := range consoleSections {
		if section.name != name {
			continue
		}
		if section.hash == "overview-health" {
			goTo(br, "overview")
			br.eval(`location.hash = "overview-health"`, nil)
			br.waitFor("the health checks", section.ready)
			br.eval(`document.getElementById("overview-health").scrollIntoView()`, nil)
		} else {
			goTo(br, section.hash)
			br.waitFor(section.name, section.ready)
		}
		if name == "repositories" {
			br.eval(`document.querySelectorAll("details.repo").forEach((d, i) => d.open = i < 2)`, nil)
		}
		time.Sleep(150 * time.Millisecond)
		pageShot(br, s, name+"-"+lang+suffix(dark))
		return
	}
	br.t.Fatalf("no section %q", name)
}
