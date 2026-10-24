package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

// remotes each demo repository pushes to: its account's own repositories.
var demoRemotes = map[string]string{
	"blog":    "https://github.com/octo-alex/blog.git",
	"billing": "https://github.com/acme-corp/billing.git",
	"sdk":     "https://github.com/octo-alex/sdk.git",
	"hotfix":  "https://gitlab.globex.example/globex/hotfix.git",
	"scratch": "https://github.com/acme-alex/scratch.git",
}

// verifyWorld applies the demo configuration and gives every repository its
// remote.
func verifyWorld(t *testing.T) (world, Env, *bytes.Buffer) {
	t.Helper()
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	for name, url := range demoRemotes {
		runGit(t, w.repos[name], "remote", "add", "origin", url)
	}
	out.Reset()
	return w, env, out
}

// withSSH answers `ssh -G` with config for every host and leaves other
// commands to the real programs.
func withSSH(env Env, config func(host string) string) Env {
	env.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "ssh" && len(args) > 0 && args[0] == "-G" {
			return []byte(config(args[len(args)-1])), nil
		}
		return provider.ExecRunner(ctx, name, args...)
	}
	return env
}

func repoCheck(t *testing.T, r VerifyReport, path string) RepoCheck {
	t.Helper()
	for _, c := range r.Repositories {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("no check for %s in %+v", path, r.Repositories)
	return RepoCheck{}
}

func verify(t *testing.T, env Env, extra ...string) VerifyReport {
	t.Helper()
	r, err := env.VerifyRepos(context.Background(), extra, false)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestVerifyFindsEveryRepositoryAsExpected(t *testing.T) {
	_, env, _ := verifyWorld(t)

	r := verify(t, env, "~/Desktop")

	if r.Counts.Problems != 0 || len(r.Repositories) != 5 {
		t.Fatalf("report = %+v", r)
	}
	billing := repoCheck(t, r, "~/Projects/acme/billing")
	if billing.Verdict != VerdictOK || billing.Profile != "acme" || billing.Author != "Alex (Acme) <1000002+acme-alex@users.noreply.github.com>" {
		t.Errorf("billing = %+v", billing)
	}
	push := billing.Remotes[0]
	if push.PushAs != "acme-alex" || push.Via != "gh token" || push.Address != "github.com/acme-corp/billing" || push.Protocol != "https" {
		t.Errorf("billing push = %+v", push)
	}
	if sdk := repoCheck(t, r, "~/Projects/acme/oss/sdk"); sdk.Account != "octo-alex" || sdk.Remotes[0].PushAs != "octo-alex" {
		t.Errorf("the nested rule decides sdk: %+v", sdk)
	}
	if scratch := repoCheck(t, r, "~/Desktop/scratch"); scratch.Reason != "default" || scratch.Verdict != VerdictOK {
		t.Errorf("scratch (default profile) = %+v", scratch)
	}
	if !strings.Contains(strings.Join(r.Missing, " "), "~/Projects/clients/globex") {
		t.Errorf("missing = %v, want the globex folder that does not exist", r.Missing)
	}
}

func TestVerifyFlagsAnAuthorThatIsNotTheProfiles(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "user.email", "someone@else.example")

	c := repoCheck(t, verify(t, env), "~/Projects/acme/billing")

	if c.Verdict != VerdictProblem || !strings.Contains(c.Findings[0].Text, "someone@else.example") ||
		!strings.Contains(c.Findings[0].Text, "--unset user.email") {
		t.Fatalf("check = %+v", c)
	}
}

func TestVerifyNotesAnAccountTheRepositoryChooses(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "credential.https://github.com.username", "octo-alex")

	c := repoCheck(t, verify(t, env), "~/Projects/acme/billing")

	if c.Verdict != VerdictNote || c.Remotes[0].PushAs != "octo-alex" || c.Remotes[0].ChosenBy != "credential.username in the repository" {
		t.Fatalf("check = %+v", c)
	}
}

func TestVerifyFlagsAChosenAccountGHDoesNotHold(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "remote", "set-url", "origin", "https://nobody-here@github.com/acme-corp/billing.git")

	c := repoCheck(t, verify(t, env), "~/Projects/acme/billing")

	if c.Verdict != VerdictProblem || !strings.Contains(findingText(c), "nobody-here is not signed in to gh") {
		t.Fatalf("check = %+v", c)
	}
}

func TestVerifyFlagsATokenInTheRemoteURLWithoutShowingIt(t *testing.T) {
	w, env, out := verifyWorld(t)
	runGit(t, w.repos["blog"], "remote", "set-url", "origin", "https://x-access-token:ghp_demoSecret123@github.com/octo-alex/blog.git")

	err := Verify(context.Background(), env, VerifyOptions{JSON: true})

	if !errors.Is(err, errVerifyProblems) {
		t.Fatalf("Verify() = %v, want problems", err)
	}
	if strings.Contains(out.String(), "ghp_demoSecret123") || strings.Contains(out.String(), "x-access-token") {
		t.Fatalf("the report shows the token:\n%s", out.String())
	}
	var r VerifyReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	c := repoCheck(t, r, "~/Projects/personal/blog")
	if !strings.Contains(findingText(c), "token written in its URL") || c.Remotes[0].PushAs != "" {
		t.Fatalf("check = %+v", c)
	}
}

func TestVerifyNotesARemoteOnAnotherHost(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["blog"], "remote", "add", "mirror", "https://codeberg.example/octo/blog.git")

	c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

	if c.Verdict != VerdictNote || !strings.Contains(findingText(c), "codeberg.example") || c.Remotes[0].Via == "" {
		t.Fatalf("check = %+v", c)
	}
}

func TestVerifyNotesARepositoryOfYourOtherAccount(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "remote", "set-url", "origin", "https://github.com/octo-alex/billing.git")

	c := repoCheck(t, verify(t, env), "~/Projects/acme/billing")

	if c.Verdict != VerdictNote || !strings.Contains(findingText(c), "belongs to your account octo-alex") {
		t.Fatalf("check = %+v", c)
	}
}

func TestVerifyTellsTheSSHAccountByTheProfilesKey(t *testing.T) {
	w, env, _ := verifyWorld(t)
	personal := filepath.Join(w.home, ".ssh", "id_personal")
	acme := filepath.Join(w.home, ".ssh", "id_acme")
	mustWrite(t, personal, "not a real key\n", 0o600)
	mustWrite(t, acme, "not a real key\n", 0o600)
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	cfg := strings.Replace(readFile(t, cfgFile), "dirs    = [\"~/Projects/personal\"", "ssh_key = \"~/.ssh/id_personal\"\ndirs    = [\"~/Projects/personal\"", 1)
	cfg = strings.Replace(cfg, "dirs    = [\"~/Projects/acme\"]", "ssh_key = \"~/.ssh/id_acme\"\ndirs    = [\"~/Projects/acme\"]", 1)
	mustWrite(t, cfgFile, cfg, 0o600)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	runGit(t, w.repos["blog"], "remote", "set-url", "origin", "git@github.com:octo-alex/blog.git")
	runGit(t, w.repos["billing"], "remote", "set-url", "origin", "git@github.com:acme-corp/billing.git")
	env = withSSH(env, func(string) string {
		return "hostname github.com\nidentityfile " + acme + "\ncontrolpath /tmp/demo-%C\n"
	})

	r := verify(t, env)

	blog := repoCheck(t, r, "~/Projects/personal/blog")
	if blog.Remotes[0].PushAs != "octo-alex" || blog.Remotes[0].Protocol != "ssh" || blog.Remotes[0].Keys[0] != "~/.ssh/id_personal" {
		t.Fatalf("blog push = %+v", blog.Remotes[0])
	}
	if !strings.Contains(findingText(blog), "ssh also offers ~/.ssh/id_acme, the key of acme-alex") {
		t.Errorf("blog findings = %+v, want a note about the alias key", blog.Findings)
	}
	if strings.Contains(findingText(blog), "shares one connection") {
		t.Errorf("GitPersona's command never shares a connection: %+v", blog.Findings)
	}
	if billing := repoCheck(t, r, "~/Projects/acme/billing"); billing.Remotes[0].PushAs != "acme-alex" || billing.Verdict == VerdictProblem {
		t.Errorf("billing = %+v", billing)
	}
}

func TestVerifyFlagsSSHConnectionsSharedBetweenKeys(t *testing.T) {
	w, env, _ := verifyWorld(t)
	one := filepath.Join(w.home, ".ssh", "id_one")
	two := filepath.Join(w.home, ".ssh", "id_two")
	mustWrite(t, one, "x\n", 0o600)
	mustWrite(t, two, "x\n", 0o600)
	runGit(t, w.repos["blog"], "remote", "set-url", "origin", "git@gh-one:octo-alex/blog.git")
	runGit(t, w.repos["sdk"], "remote", "set-url", "origin", "git@gh-two:octo-alex/sdk.git")
	env = withSSH(env, func(host string) string {
		key := map[string]string{"gh-one": one, "gh-two": two}[host]
		return "hostname github.com\nidentityfile " + key + "\ncontrolpath /tmp/master-demo\n"
	})

	r := verify(t, env)

	for _, path := range []string{"~/Projects/personal/blog", "~/Projects/acme/oss/sdk"} {
		c := repoCheck(t, r, path)
		if c.Verdict != VerdictProblem || !strings.Contains(findingText(c), "shares one connection to github.com") {
			t.Errorf("%s = %+v", path, c)
		}
	}
}

func TestVerifyAsksTheSSHHostWhenToldTo(t *testing.T) {
	w, env, _ := verifyWorld(t)
	key := filepath.Join(w.home, ".ssh", "id_any")
	mustWrite(t, key, "x\n", 0o600)
	runGit(t, w.repos["billing"], "remote", "set-url", "origin", "git@github.com:acme-corp/billing.git")
	fakeSSH := filepath.Join(w.home, "fake-bin", "ssh")
	mustWrite(t, fakeSSH, "#!/bin/sh\necho \"$*\" >> \""+filepath.Join(w.home, "ssh.log")+"\"\n"+
		"echo \"Hi octo-alex! You've successfully authenticated, but GitHub does not provide shell access.\" >&2\nexit 1\n", 0o700)
	env = withSSH(env, func(string) string { return "hostname github.com\nidentityfile " + key + "\n" })
	env.LookPath = func(name string) (string, error) {
		if name == "ssh" {
			return fakeSSH, nil
		}
		return os.Getenv("PATH"), errors.New("not used")
	}

	r, err := env.VerifyRepos(context.Background(), nil, true)
	if err != nil {
		t.Fatal(err)
	}

	c := repoCheck(t, r, "~/Projects/acme/billing")
	if c.Remotes[0].PushAs != "octo-alex" || !c.Remotes[0].Greeted || c.Verdict != VerdictProblem ||
		!strings.Contains(findingText(c), "pushes as octo-alex, not as the profile's acme-alex") {
		t.Fatalf("check = %+v", c)
	}
	log := readFile(t, filepath.Join(w.home, "ssh.log"))
	if !strings.Contains(log, "ControlPath=none") || !strings.Contains(log, "BatchMode=yes") || !strings.Contains(log, "git@github.com") {
		t.Errorf("ssh ran with %q", log)
	}
}

func TestVerifyPrintsOnlyWhatNeedsAttentionUnlessAll(t *testing.T) {
	w, env, out := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "credential.https://github.com.username", "octo-alex")

	if err := Verify(context.Background(), env, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "· ~/Projects/acme/billing") || strings.Contains(text, "~/Projects/personal/blog") ||
		!strings.Contains(text, "3 as expected, 1 with notes, 0 with problems") {
		t.Fatalf("output:\n%s", text)
	}
	out.Reset()
	if err := Verify(context.Background(), env, VerifyOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "✓ ~/Projects/personal/blog") || !strings.Contains(out.String(), "push    origin → github.com/octo-alex/blog as octo-alex (gh token)") {
		t.Fatalf("--all output:\n%s", out.String())
	}
}

func findingText(c RepoCheck) string {
	var parts []string
	for _, f := range c.Findings {
		parts = append(parts, f.Level+": "+f.Text)
	}
	return strings.Join(parts, "\n")
}

func TestVerifyHelpers(t *testing.T) {
	for url, want := range map[string][2]string{
		"https://github.com/o/r.git":            {"https", "github.com/o/r"},
		"https://u:secret@github.com/o/r":       {"https", "github.com/o/r"},
		"git@github-work:Org/repo.git":          {"ssh", "github-work/Org/repo"},
		"ssh://git@gitlab.example:2222/g/p.git": {"ssh", "gitlab.example:2222/g/p"},
		"/srv/git/repo.git":                     {"other", "local repository"},
	} {
		if p, a := remoteProtocol(url), remoteAddress(url); p != want[0] || a != want[1] {
			t.Errorf("%s: protocol %q address %q, want %v", url, p, a, want)
		}
	}
	if !urlHasPassword("https://u:p@h/x") || urlHasPassword("https://u@h/x") || urlHasPassword("git@h:x") {
		t.Error("urlHasPassword")
	}
	account, provider, ours := gitPersonaHelper(`!'/opt/bin/gitpersona' credential --provider github --host 'github.com' --account 'octo-alex'`)
	if account != "octo-alex" || provider != "github" || !ours {
		t.Errorf("gitPersonaHelper = %q %q %v", account, provider, ours)
	}
	if _, _, ours := gitPersonaHelper("osxkeychain"); ours {
		t.Error("osxkeychain is not GitPersona")
	}
	if got := identityFlags(`ssh -i '/k/a b' -o IdentityFile=/k/c -i/k/d`); strings.Join(got, "|") != "/k/a b|/k/c|/k/d" {
		t.Errorf("identityFlags = %q", got)
	}
	for command, want := range map[string]string{
		"":                               "/tmp/m",
		"ssh -o ControlPath=none":        "",
		"ssh -S none":                    "",
		"ssh -i k -o IdentitiesOnly=yes": "/tmp/m",
		"ssh -i k -o IdentitiesOnly=yes -oControlPath=none": "",
	} {
		if got := sharedConnection(command, "/tmp/m"); got != want {
			t.Errorf("sharedConnection(%q) = %q, want %q", command, got, want)
		}
	}
	if sharedConnection("", "none") != "" {
		t.Error("ControlPath none shares nothing")
	}
	if got := sshCommandArgs("ssh -i '/k/a b' -o IdentitiesOnly=yes -v host extra"); strings.Join(got, "|") != "-i|/k/a b|-o|IdentitiesOnly=yes|-v" {
		t.Errorf("sshCommandArgs = %q", got)
	}
	h := parseSSHConfig("user git\nhostname github.com\nidentityfile ~/.ssh/a\ncontrolpath /tmp/x\n", "alias")
	if h.hostname != "github.com" || len(h.files) != 1 || h.controlPath != "/tmp/x" {
		t.Errorf("parseSSHConfig = %+v", h)
	}
}
