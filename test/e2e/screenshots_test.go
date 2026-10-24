package e2e

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestREADMEScreenshots draws the README's pictures from a demo home: the
// setup page, the repositories check and the terminal, in light and dark.
// It runs only with GITPERSONA_SHOTS set (make screenshots); nothing in the
// pictures comes from the machine that makes them.
func TestREADMEScreenshots(t *testing.T) {
	if os.Getenv("GITPERSONA_SHOTS") == "" {
		t.Skip("set GITPERSONA_SHOTS to a folder to draw the README pictures")
	}
	s := newSandbox(t, false)
	s.repo("code/work/tools", "git@github-work:demo-work/tools.git")
	s.repo("code/client/portal", "git@github-work:demo-org/portal.git")
	s.write(".ssh/config", "Host github-work\n  HostName github.com\n  IdentityFile ~/.ssh/id_work\n", 0o600)
	s.write(".ssh/id_work", "KEY PLACEHOLDER (never read)\n", 0o600)

	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	for _, dark := range []bool{false, true} {
		b.viewport(1100, 820, dark)
		if !dark {
			b.open(link)
		}
		b.waitFor("the ready page", `document.querySelector("#view h3")?.innerText === "Your setup is ready"`)
		b.screenshot("setup" + suffix(dark))
	}
	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")

	// Every line is real output from the demo home.
	terminal := strings.Join([]string{
		"$ cd ~/code/work/api && gitpersona whoami",
		strings.TrimSpace(s.runIn(filepath.Join(s.home, "code/work/api"), "whoami")),
		"",
		"$ cd ~/code/fun/game && gitpersona whoami",
		strings.TrimSpace(s.runIn(filepath.Join(s.home, "code/fun/game"), "whoami")),
		"",
		"$ gitpersona verify",
		strings.TrimSpace(s.run("verify")),
	}, "\n")
	for _, dark := range []bool{false, true} {
		b.viewport(1100, 470, dark)
		showTerminal(b, terminal, dark)
		b.screenshot("terminal" + suffix(dark))
	}

	// A repository that chooses its own push account, for the check to show.
	s.git(filepath.Join(s.home, "code/fun/game"), "config", "credential.https://github.com.username", "demo-work")
	// Back to the page: its session cookie is still valid (one page at a time).
	base, _, _ := strings.Cut(link, "?")
	b.open(base)
	goTo(b, "repositories")
	b.waitFor("the report", `document.querySelector(".summary-badges")`)
	b.eval(`document.querySelectorAll("details.repo").forEach((d, i) => d.open = i < 2)`, nil)
	b.eval(`document.querySelector(".summary-badges").closest(".card").scrollIntoView()`, nil)
	for _, dark := range []bool{false, true} {
		b.viewport(1100, 900, dark)
		time.Sleep(200 * time.Millisecond)
		b.screenshot("repositories" + suffix(dark))
	}
}

func suffix(dark bool) string {
	if dark {
		return "-dark"
	}
	return "-light"
}

// showTerminal opens a page that draws text as a terminal window.
func showTerminal(b *browser, text string, dark bool) {
	b.t.Helper()
	bg, fg, bar := "#ffffff", "#1f2328", "#eaeef2"
	if dark {
		bg, fg, bar = "#0d1117", "#e6edf3", "#161b22"
	}
	page := fmt.Sprintf(`<!doctype html><meta charset="utf-8"><body style="margin:0;background:transparent">
<div style="margin:24px;border-radius:10px;overflow:hidden;box-shadow:0 8px 28px rgb(0 0 0/25%%);background:%s">
<div style="height:30px;background:%s;display:flex;gap:8px;align-items:center;padding-left:14px">
<span style="width:12px;height:12px;border-radius:50%%;background:#ff5f57"></span>
<span style="width:12px;height:12px;border-radius:50%%;background:#febc2e"></span>
<span style="width:12px;height:12px;border-radius:50%%;background:#28c840"></span></div>
<pre style="margin:0;padding:18px 22px;color:%s;font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap">%s</pre>
</div>`, bg, bar, fg, html.EscapeString(strings.TrimRight(text, "\n")))
	b.call("Page.navigate", map[string]any{"url": "data:text/html;charset=utf-8," + url.PathEscape(page)})
	b.waitFor("the terminal", `document.querySelector("pre")`)
}

// dashboardConfig has an account with two identities (two author names),
// as when folders commit under different names.
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

// TestDashboardScreenshots draws the overview and the accounts page in both
// languages and both themes (GITPERSONA_SHOTS).
func TestDashboardScreenshots(t *testing.T) {
	if os.Getenv("GITPERSONA_SHOTS") == "" {
		t.Skip("set GITPERSONA_SHOTS to a folder to draw the pictures")
	}
	s := newSandbox(t, false)
	s.write(".config/gitpersona/config.toml", dashboardConfig, 0o600)
	s.repo("code/infra/terraform", "https://github.com/demo-work/terraform.git")
	s.run("apply")
	link, _ := s.start("ui")
	b := startBrowser(t)
	b.viewport(1280, 900, false)
	b.open(link)
	base, _, _ := strings.Cut(link, "?")
	for _, lang := range []string{"zh", "en"} {
		// The language is read when the page loads.
		b.eval(fmt.Sprintf(`localStorage.setItem("gitpersona-lang", %q)`, lang), nil)
		b.call("Page.reload", map[string]any{})
		b.waitFor("the page", `document.querySelector("#view section h2")`)
		for _, dark := range []bool{false, true} {
			b.viewport(1280, 1000, dark)
			b.call("Page.navigate", map[string]any{"url": base + "#overview"})
			b.waitFor("the numbers", `document.querySelectorAll(".stat.pending").length === 0 && document.querySelector(".stat")`)
			b.screenshot("overview-" + lang + suffix(dark))
			goTo(b, "profiles")
			b.waitFor("the accounts", `document.querySelector(".persona")`)
			b.screenshot("accounts-" + lang + suffix(dark))
		}
	}
	b.viewport(390, 844, false)
	goTo(b, "overview")
	b.waitFor("the numbers", `document.querySelectorAll(".stat.pending").length === 0 && document.querySelector(".stat")`)
	checkFits(b, "the overview on a phone")
	b.screenshot("overview-phone")
	b.checkClean()
}
