package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// keyOnlyRuleWorld has a rule of the user's that sets only a key for tags in
// ~/special, and a profile in ~/special/work.
func keyOnlyRuleWorld(t *testing.T, config string) Env {
	t.Helper()
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/special/\"]\n\tpath = ~/.gitconfig-special\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-special"), "[user]\n\tsigningkey = ~/.ssh/tags.pub\n[tag]\n\tgpgSign = true\n"+
		"[core]\n\tsshCommand = ssh -p 2222\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), config, 0o600)
	env, _ := w.env(t, w.home)
	return env
}

const specialProfileConfig = `version = 1

[[profile]]
id      = "personal"
account = "octo-alex"
name    = "Alex Octo"
email   = "1000001+octo-alex@users.noreply.github.com"
dirs    = ["~/special/work"]
`

func TestKeyOnlyRulesListTheSettingsOnlyAProfileCannotReplace(t *testing.T) {
	env := keyOnlyRuleWorld(t, specialProfileConfig)

	rules := env.keyOnlyRules(context.Background())

	if len(rules) != 1 {
		t.Fatalf("keyOnlyRules() = %+v, want the one key-only rule", rules)
	}
	if rules[0].Dir == "" || !strings.Contains(rules[0].Keep, "user.signingkey") || !strings.Contains(rules[0].Keep, "core.sshcommand") {
		t.Errorf("rule = %+v, want it to say which settings it keeps", rules[0])
	}
	if rules[0].File != "" {
		t.Errorf("rule file = %q, want none for the main Git configuration", rules[0].File)
	}
}

func TestKeyOnlyRulesIgnoreRulesThatSetAnIdentity(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/special/\"]\n\tpath = ~/.gitconfig-special\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-special"), "[user]\n\temail = special@demo.example\n\tsigningkey = ~/.ssh/tags.pub\n", 0o600)
	env, _ := w.env(t, w.home)

	if rules := env.keyOnlyRules(context.Background()); len(rules) != 0 {
		t.Errorf("keyOnlyRules() = %+v, want none: an identity rule is not key-only", rules)
	}
}

func TestDoctorWarnsWhenAProfileFolderLiesInsideARuleThatDoesMore(t *testing.T) {
	env := keyOnlyRuleWorld(t, specialProfileConfig)
	var out strings.Builder
	env.Stdout, env.Stderr = &out, &out

	Doctor(context.Background(), env)

	for _, want := range []string{"does more than a profile can", "~/special/work (personal)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorWarnsWhenAPinLiesInsideARuleThatDoesMore(t *testing.T) {
	config := specialProfileConfig + "\n[[pin]]\nrepo    = \"~/special/repo\"\nprofile = \"personal\"\n"
	env := keyOnlyRuleWorld(t, config)
	var out strings.Builder
	env.Stdout, env.Stderr = &out, &out

	Doctor(context.Background(), env)

	if want := "~/special/repo (pin, personal)"; !strings.Contains(out.String(), want) {
		t.Errorf("doctor output lacks %q:\n%s", want, out.String())
	}
}

func TestDoctorStaysQuietWhenNoProfileTouchesTheRulesFolder(t *testing.T) {
	config := strings.Replace(specialProfileConfig, "~/special/work", "~/elsewhere/work", 1)
	env := keyOnlyRuleWorld(t, config)
	var out strings.Builder
	env.Stdout, env.Stderr = &out, &out

	Doctor(context.Background(), env)

	if strings.Contains(out.String(), "does more than a profile can") {
		t.Errorf("doctor warned about a rule no profile reaches:\n%s", out.String())
	}
}
