// Package e2e drives the configuration page in a real headless Chrome
// against a gitpersona gitpersonaBin in a demo home under /tmp. The tests run only
// with GITPERSONA_E2E=1 (make e2e), because they need Chrome.
package e2e

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var gitpersonaBin string

func TestMain(m *testing.M) {
	if os.Getenv("GITPERSONA_E2E") != "1" {
		fmt.Println("skipping end-to-end tests; run them with GITPERSONA_E2E=1 (make e2e)")
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "gitpersona-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gitpersonaBin = filepath.Join(dir, "gitpersona")
	if out, err := exec.Command("go", "build", "-o", gitpersonaBin, "../../cmd/gitpersona").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build gitpersona: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeGH knows two demo accounts: demo-work shows a public email,
// demo-personal hides it. It never prints a token.
const fakeGH = `#!/bin/sh
case "$*" in
  "auth status"*) echo '{"hosts":{"github.com":[{"login":"demo-work","state":"success","active":true},{"login":"demo-personal","state":"success","active":false}]}}' ;;
  *"users/demo-work") echo '{"login":"demo-work","id":101,"name":"Demo Worker","email":"worker@demo.example"}' ;;
  *"users/demo-personal") echo '{"login":"demo-personal","id":102,"name":null,"email":null}' ;;
  *) exit 1 ;;
esac
`

const demoConfig = `version = 1
default = "personal"

[[profile]]
id      = "work"
account = "demo-work"
name    = "Demo Worker"
email   = "worker@demo.example"
dirs    = ["~/code/work"]

[[profile]]
id      = "personal"
account = "demo-personal"
name    = "Demo Person"
email   = "102+demo-personal@users.noreply.github.com"
dirs    = ["~/code/fun"]
`

// sandbox is a demo home with repositories and a fake gh.
type sandbox struct {
	t    *testing.T
	home string
	env  []string
}

func newSandbox(t *testing.T, withConfig bool) *sandbox {
	t.Helper()
	root := sandboxPath(t, t.TempDir())
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	s := &sandbox{t: t, home: home, env: []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"GIT_CONFIG_NOSYSTEM=1",
		"PATH=" + bin + string(os.PathListSeparator) + filepath.Dir(git) + string(os.PathListSeparator) + "/usr/bin:/bin",
		"TMPDIR=" + root,
	}}
	s.write("../bin/gh", fakeGH, 0o700)
	// A hand-made includeIf identity for demo-personal, as many users have.
	s.write(".gitconfig", "[user]\n\tname = Someone\n[includeIf \"gitdir:~/old/personal/\"]\n\tpath = ~/.gitconfig-personal\n", 0o600)
	s.write(".gitconfig-personal", "[user]\n\tname = Demo Person\n\temail = person@demo.example\n"+
		"[credential \"https://github.com\"]\n\tusername = demo-personal\n", 0o600)
	for _, r := range []string{"api", "web", "docs"} {
		s.repo("code/work/"+r, "git@github.com:demo-work/"+r+".git")
	}
	s.repo("code/fun/game", "https://github.com/demo-personal/game.git")
	s.repo("old/personal/notes", "")
	s.repo("Desktop/scratch", "")
	if withConfig {
		s.write(".config/gitpersona/config.toml", demoConfig, 0o600)
	}
	return s
}

func (s *sandbox) write(rel, content string, perm os.FileMode) {
	s.t.Helper()
	path := filepath.Join(s.home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sandbox) repo(rel, remote string) {
	s.t.Helper()
	dir := filepath.Join(s.home, rel)
	s.git(s.home, "init", "-q", dir)
	if remote != "" {
		s.git(dir, "remote", "add", "origin", remote)
	}
}

// git runs git in dir with the sandbox's home and returns its output.
func (s *sandbox) git(dir string, args ...string) string {
	s.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = s.env
	out, err := cmd.CombinedOutput()
	if err != nil && !(len(args) > 0 && args[0] == "config") {
		s.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (s *sandbox) email(rel string) string {
	return s.git(filepath.Join(s.home, rel), "config", "user.email")
}

var linkPattern = regexp.MustCompile(`http://127\.0\.0\.1:\d+/\?key=[0-9a-f]+\S*`)

// start runs gitpersona with args (ui or setup --web) and returns the
// one-time link it prints, and a channel that is closed when it exits.
func (s *sandbox) start(args ...string) (string, <-chan struct{}) {
	s.t.Helper()
	cmd := exec.Command(gitpersonaBin, append(args, "--no-open")...)
	cmd.Env, cmd.Dir = s.env, s.home
	out, err := cmd.StdoutPipe()
	if err != nil {
		s.t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	s.t.Cleanup(func() { _ = cmd.Process.Kill(); <-exited })

	links := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			if link := linkPattern.FindString(scanner.Text()); link != "" {
				links <- link
			}
		}
	}()
	select {
	case link := <-links:
		return link, exited
	case <-time.After(waitTimeout):
		s.t.Fatal("gitpersona printed no link")
		return "", exited
	}
}

// Page helpers.

func setValue(b *browser, selector, value string) {
	b.t.Helper()
	b.eval(fmt.Sprintf(`const el = document.querySelector(%q);
		el.value = %q;
		el.dispatchEvent(new Event("input", { bubbles: true }));
		el.dispatchEvent(new Event("change", { bubbles: true }));`, selector, value), nil)
}

func click(b *browser, selector string) {
	b.t.Helper()
	var found bool
	b.eval(fmt.Sprintf(`const el = document.querySelector(%q); if (!el) return false; el.click(); return true;`, selector), &found)
	if !found {
		b.t.Fatalf("nothing to click at %s\npage: %s", selector, b.text("#view"))
	}
}

// confirmDialog accepts the confirmation dialog once it is open.
func confirmDialog(b *browser) {
	b.t.Helper()
	b.waitFor("the confirmation dialog", `document.getElementById("confirm").open`)
	click(b, "#confirm-ok")
}

func waitStatus(b *browser, want string) {
	b.t.Helper()
	b.waitFor(fmt.Sprintf("status %q", want), fmt.Sprintf(`document.getElementById("status").innerText.includes(%q)`, want))
}

func goTo(b *browser, section string) {
	b.t.Helper()
	b.eval(fmt.Sprintf(`location.hash = %q`, section), nil)
	b.waitFor("section "+section, fmt.Sprintf(`location.hash === "#%s" && document.querySelector("#view section h2")`, section))
}

// checkFits fails when the page scrolls sideways.
func checkFits(b *browser, where string) {
	b.t.Helper()
	var size struct{ Scroll, Inner int }
	b.eval(`return { Scroll: document.documentElement.scrollWidth, Inner: innerWidth }`, &size)
	if size.Scroll > size.Inner {
		b.t.Errorf("%s scrolls sideways: content is %dpx wide on a %dpx screen", where, size.Scroll, size.Inner)
	}
}

func TestFirstRunSetupInThreeSteps(t *testing.T) {
	s := newSandbox(t, false)
	link, _ := s.start("setup", "--web")
	b := startBrowser(t)
	b.viewport(375, 812, false)
	b.open(link)

	// Step 1: identities, with each email's source.
	b.waitFor("step 1", `document.querySelector("#view h3")?.innerText.startsWith("1.")`)
	var emails []string
	b.eval(`return [...document.querySelectorAll("#view input[type=email]")].map((e) => e.value)`, &emails)
	if len(emails) != 2 || emails[0] != "worker@demo.example" || emails[1] != "person@demo.example" {
		t.Fatalf("step 1 should propose the public email and reuse the existing one, got %v", emails)
	}
	if page := b.text("#view"); !strings.Contains(page, "public email on this account") || !strings.Contains(page, "From your existing setup") {
		t.Errorf("step 1 should say where each email comes from:\n%s", page)
	}
	checkFits(b, "setup step 1")
	setValue(b, `#view fieldset input[type=text]:not([pattern])`, "Demo Worker (Work)")
	click(b, "#view button.primary")

	// Step 2: directories from the existing setup and a scan.
	b.waitFor("step 2", `document.querySelector("#view h3")?.innerText.startsWith("2.")`)
	setValue(b, "#view .card input", "~/code")
	click(b, "#view .card button")
	b.waitFor("scan suggestions", `document.getElementById("view").innerText.includes("~/code/work")`)
	if page := b.text("#view"); !strings.Contains(page, "all 3 repositories") || !strings.Contains(page, "~/old/personal") {
		t.Errorf("step 2 should show the existing directory and suggestions with reasons:\n%s", page)
	}
	checkFits(b, "setup step 2")
	click(b, "#view button.primary")

	// Step 3: review every file, then apply.
	b.waitFor("the review", `document.querySelector("#view button.primary") && !document.querySelector("#view button.primary").disabled`)
	if review := b.text("#view"); !strings.Contains(review, "config.toml") || !strings.Contains(review, `name    = "Demo Worker (Work)"`) {
		t.Errorf("the review should show config.toml with the edited name:\n%s", review)
	}
	checkFits(b, "setup step 3")
	if _, err := os.Stat(filepath.Join(s.home, ".config", "gitpersona", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("nothing may be written before Apply")
	}
	click(b, "#view button.primary")
	waitStatus(b, "GitPersona is set up")
	b.waitFor("the health checks", `document.getElementById("view").innerText.includes("Health checks")`)
	checkFits(b, "setup done")

	for repo, want := range map[string]string{
		"code/work/api":      "worker@demo.example",
		"code/fun/game":      "person@demo.example",
		"old/personal/notes": "person@demo.example",
		"Desktop/scratch":    "worker@demo.example", // the default profile
	} {
		if got := s.email(repo); got != want {
			t.Errorf("%s commits as %q, want %q", repo, got, want)
		}
	}
	b.checkClean()
}

func TestEditingProfilesDirectoriesPinsAndBackups(t *testing.T) {
	s := newSandbox(t, true)
	link, _ := s.start("ui")
	b := startBrowser(t)
	b.viewport(1280, 900, true)
	b.open(link)

	goTo(b, "apply")
	b.waitFor("the diffs", `document.querySelector("#view pre.diff")`)
	click(b, "#view button.primary")
	waitStatus(b, "Applied")
	b.waitFor("up to date", `document.getElementById("view").innerText.includes("Up to date")`)
	if got := s.email("code/work/api"); got != "worker@demo.example" {
		t.Fatalf("after apply code/work/api commits as %q", got)
	}

	goTo(b, "profiles")
	// Add a directory to the work profile (the first card).
	setValue(b, "article.card form.row input", "~/old/personal")
	b.eval(`document.querySelector("article.card form.row").requestSubmit()`, nil)
	waitStatus(b, "now uses profile work")
	if got := s.email("old/personal/notes"); got != "worker@demo.example" {
		t.Errorf("after adding the directory old/personal/notes commits as %q", got)
	}

	// Pin a repository under work's directory to personal (the second card).
	b.eval(`const forms = document.querySelectorAll("article.card")[1].querySelectorAll("form.row");
		const pin = forms[forms.length - 1];
		pin.querySelector("input").value = "~/code/work/docs";
		pin.requestSubmit();`, nil)
	waitStatus(b, "now uses personal")
	if got := s.email("code/work/docs"); got != "102+demo-personal@users.noreply.github.com" {
		t.Errorf("the pinned repository commits as %q", got)
	}

	// Remove the directory again, through the confirmation dialog.
	click(b, `article.card button[aria-label="Remove ~/old/personal"]`)
	confirmDialog(b)
	waitStatus(b, "Removed ~/old/personal")

	// Change the work profile's name.
	click(b, "article.card .actions button:not(.danger)")
	b.waitFor("the edit form", `document.querySelector("article.card form.stack")`)
	setValue(b, `article.card form.stack input[autocomplete=name]`, "Demo Worker Renamed")
	b.eval(`document.querySelector("article.card form.stack").requestSubmit()`, nil)
	waitStatus(b, "Saved work")
	if got := s.git(filepath.Join(s.home, "code/work/api"), "config", "user.name"); got != "Demo Worker Renamed" {
		t.Errorf("after the edit code/work/api commits as %q", got)
	}

	// Undo the rename with the newest backup.
	goTo(b, "backups")
	b.waitFor("the backups", `document.querySelectorAll("#view ul.rows li").length >= 4`)
	click(b, "#view ul.rows li button")
	confirmDialog(b)
	waitStatus(b, "Restored")
	if got := s.git(filepath.Join(s.home, "code/work/api"), "config", "user.name"); got != "Demo Worker" {
		t.Errorf("after restoring code/work/api commits as %q", got)
	}

	goTo(b, "overview")
	setValue(b, "#view form.row input", "~/code/work/docs")
	b.eval(`document.querySelector("#view form.row").requestSubmit()`, nil)
	b.waitFor("the answer", `document.querySelector("#view dl.facts")`)
	if answer := b.text("#view dl.facts"); !strings.Contains(answer, "personal") || !strings.Contains(answer, "pinned repository") {
		t.Errorf("the resolver should explain the pin:\n%s", answer)
	}
	b.checkClean()
}

func TestEveryViewFitsPhoneAndDesktop(t *testing.T) {
	s := newSandbox(t, true)
	link, _ := s.start("ui")
	b := startBrowser(t)
	b.open(link)
	for _, screen := range []struct {
		width, height int
		dark          bool
	}{{375, 812, false}, {375, 812, true}, {1280, 900, false}} {
		b.viewport(screen.width, screen.height, screen.dark)
		for _, section := range []string{"overview", "profiles", "apply", "shells", "backups", "setup"} {
			goTo(b, section)
			time.Sleep(300 * time.Millisecond) // sections that load more data
			checkFits(b, fmt.Sprintf("%s at %dpx (dark %v)", section, screen.width, screen.dark))
		}
	}
	b.checkClean()
}

func TestTheLinkWorksOnceAndClosingStopsGitPersona(t *testing.T) {
	s := newSandbox(t, true)
	link, exited := s.start("ui")
	b := startBrowser(t)
	b.open(link)

	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("a second use of the link got status %d, want it refused", resp.StatusCode)
	}

	click(b, "#close")
	b.waitFor("the closed page", `document.getElementById("view").innerText.includes("GitPersona has stopped")`)
	select {
	case <-exited:
	case <-time.After(waitTimeout):
		t.Error("gitpersona kept running after Close")
	}
	b.checkClean()
}

// run runs gitpersona to completion and returns its combined output.
func (s *sandbox) run(args ...string) string {
	s.t.Helper()
	cmd := exec.Command(gitpersonaBin, args...)
	cmd.Env, cmd.Dir = s.env, s.home
	out, _ := cmd.CombinedOutput()
	return string(out)
}
