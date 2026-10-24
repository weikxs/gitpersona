package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

func TestParseGitVersionReadsWhatVendorsPrint(t *testing.T) {
	for out, want := range map[string]gitVersion{
		"git version 2.34.1\n":               {2, 34, 1},
		"git version 2.39.5 (Apple Git-154)": {2, 39, 5},
		"git version 2.45.2.windows.1":       {2, 45, 2},
		"git version 2.36":                   {2, 36, 0},
		"git version 3.0.0-rc1":              {3, 0, 0},
	} {
		got, ok := parseGitVersion(out)
		if !ok || got != want {
			t.Errorf("parseGitVersion(%q) = %v, %v; want %v", out, got, ok, want)
		}
	}
	for _, out := range []string{"", "git: command not found", "git version", "git version two"} {
		if _, ok := parseGitVersion(out); ok {
			t.Errorf("parseGitVersion(%q) should fail", out)
		}
	}
}

func TestGitVersionsCompareByMajorMinorAndPatch(t *testing.T) {
	for _, tc := range []struct {
		v, other gitVersion
		want     bool
	}{
		{gitVersion{2, 36, 0}, remoteConditionsGit, true},
		{gitVersion{2, 35, 9}, remoteConditionsGit, false},
		{gitVersion{3, 0, 0}, remoteConditionsGit, true},
		{gitVersion{1, 99, 0}, minGit, false},
		{gitVersion{2, 31, 0}, minGit, true},
		{gitVersion{2, 31, 0}, gitVersion{2, 31, 1}, false},
	} {
		if got := tc.v.atLeast(tc.other); got != tc.want {
			t.Errorf("%v.atLeast(%v) = %v, want %v", tc.v, tc.other, got, tc.want)
		}
	}
}

// withGitVersion makes `git version` print out (or fail), leaving every
// other command to the real programs.
func withGitVersion(env Env, out string, err error) Env {
	env.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) == 1 && args[0] == "version" {
			return []byte(out), err
		}
		return provider.ExecRunner(ctx, name, args...)
	}
	return env
}

func TestDoctorChecksTheGitVersion(t *testing.T) {
	w := newWorld(t)
	base, _ := w.env(t, w.home)
	for name, tc := range map[string]struct {
		out        string
		err        error
		ok, warn   bool
		wantInText string
	}{
		"supported":      {out: "git version 2.39.5 (Apple Git-154)", ok: true, wantInText: "Git 2.39.5"},
		"too old":        {out: "git version 2.30.2", wantInText: "Git 2.30.2 is older than 2.31"},
		"cannot be told": {err: errors.New("no git"), warn: true, wantInText: "could not tell"},
	} {
		t.Run(name, func(t *testing.T) {
			c := withGitVersion(base, tc.out, tc.err).checkGitVersion(context.Background())

			if c.ok != tc.ok || c.warn != tc.warn || !strings.Contains(c.label, tc.wantInText) {
				t.Fatalf("check = %+v", c)
			}
		})
	}
}

// On Git before 2.36 a remote includeIf rule never applies, and doctor says
// so instead of saying where it does not apply.
func TestDoctorSaysAnOldGitIgnoresARemoteRule(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	appendRule(t, w, "work", "[user]\n\tname = Alex Acme\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n")
	f := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, f, readFile(t, f)+"[includeIf \"hasconfig:remote.*.url:git@github-side:*/**\"]\n\tpath = ~/.gitconfig-side\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-side"), "[user]\n\tname = Side\n\temail = side@demo.example\n", 0o600)
	runGit(t, w.home, "init", "-q", filepath.Join(w.home, "work/side"))
	runGit(t, filepath.Join(w.home, "work/side"), "remote", "add", "origin", "git@github-side:demo-org/side.git")
	env, out := w.env(t, w.home)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	_ = Doctor(context.Background(), withGitVersion(env, "git version 2.34.1\n", nil))

	text := out.String()
	if !strings.Contains(text, "never applies: Git 2.34.1 ignores hasconfig conditions, which need Git 2.36 or newer") {
		t.Errorf("doctor should say the rule never applies:\n%s", text)
	}
	if strings.Contains(text, "does not apply in your profiles' folders") {
		t.Errorf("doctor should not say where the rule does not apply when it applies nowhere:\n%s", text)
	}
}
