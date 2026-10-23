package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/paths"
)

// missingKeyFiles names the key files of p that do not exist. It only checks
// that the files are there; it never opens them.
func missingKeyFiles(p config.Profile, home string) []string {
	var missing []string
	exists := func(path string) bool { return keyFileExists(path, home, true) }
	if p.SSHKey != "" && !exists(p.SSHKey) {
		missing = append(missing, "SSH key "+p.SSHKey)
	}
	if p.Signing.Method == config.SigningSSH && p.Signing.Key != "" && !config.LiteralSSHKey(p.Signing.Key) && !exists(p.Signing.Key) {
		missing = append(missing, "signing key "+p.Signing.Key)
	}
	return missing
}

// keyFileExists reports whether a key file is there. It never opens it.
// Only a definite "no such file" counts as missing: a folder the user keeps
// private (no access) is not looked into. invalid is the answer for a path
// that cannot be expanded.
func keyFileExists(path, home string, invalid bool) bool {
	norm, err := paths.Normalize(path, home)
	if err != nil {
		return invalid
	}
	_, err = os.Stat(norm)
	return !errors.Is(err, os.ErrNotExist)
}

// requireKeyFiles refuses a profile whose key files do not exist: Git would
// fail on every commit (signing) or push (SSH).
func requireKeyFiles(p config.Profile, home string) error {
	missing := missingKeyFiles(p, home)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s does not exist; create it first (for example with ssh-keygen -t ed25519 -f <file>), "+
		"or choose another file", strings.Join(missing, " and "))
}

// checkKeyFiles reports profiles whose key files are missing.
func (e Env) checkKeyFiles(cfg config.Config) []check {
	var checks []check
	for _, p := range cfg.Profiles {
		for _, m := range missingKeyFiles(p, e.Home) {
			checks = append(checks, check{
				label:  fmt.Sprintf("profile %s: %s does not exist", p.ID, m),
				advice: "Git fails to sign or push with this profile; create the key or change the profile",
			})
		}
	}
	return checks
}

// checkDirs warns about directory rules for folders that do not exist, which
// are often typos. A rule for a folder created later is fine, so this is a
// warning only.
func (e Env) checkDirs(cfg config.Config) []check {
	var checks []check
	for _, p := range cfg.Profiles {
		for _, d := range p.Dirs {
			norm, err := paths.Normalize(d, e.Home)
			if err != nil {
				continue
			}
			if _, err := os.Stat(norm); errors.Is(err, os.ErrNotExist) {
				checks = append(checks, check{warn: true,
					label:  fmt.Sprintf("profile %s: directory %s does not exist yet", p.ID, d),
					advice: "the rule applies once it exists; remove it if it is a typo"})
			}
		}
	}
	return checks
}

// FixOldRules is the fix that removes old includeIf identity rules; the page
// offers it as a button.
const FixOldRules = "old-rules"

// checkOldRules looks at the hand-made includeIf identity rules in the
// global Git configuration. GitPersona includes them again among its rules,
// so they keep applying in their folders. Rules a profile takes over are a
// warning with a fix (they are only confusing); the others are fine.
// Rules GitPersona cannot include again, or that yield to a profile given
// their folder, are a warning: GitPersona's rules replace them there.
func (e Env) checkOldRules(ctx context.Context, cfg config.Config) []check {
	owners := ownersOf(cfg)
	var checks []check
	var replaced []string
	text, _ := readOptional(e.resolvedUserGitConfig())
	included, skipped := e.userRules(ctx, cfg, text)
	rules := discover.IdentityRules(ctx, e.Home, e.GlobalGitConfigPath(), e.Run)
	identities := map[string]discover.Rule{}
	for _, r := range rules {
		identities[r.Condition] = r
	}
	// Until apply runs, GitPersona's files do not include a rule added by hand.
	pending := ""
	if changes, err := planApply(ctx, e); err == nil && len(changes) > 0 {
		pending = " once you run `gitpersona apply`"
	}
	overridden := map[string]bool{}
	for _, k := range skipped {
		overridden[k.Condition] = true
		checks = append(checks, e.skippedRuleCheck(k, identities[k.Condition], owners, cfg.Default))
	}
	for _, u := range included {
		if u.Yield {
			overridden[u.Condition] = true
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("your includeIf rule %q is replaced there by the profile you gave that folder", u.Condition),
				advice: "remove the rule from " + e.display(e.GlobalGitConfigPath()) + " if you no longer need it"})
		}
	}
	for _, r := range rules {
		switch {
		case overridden[r.Condition]:
		case e.takenOver(r, owners) && r.Keep == "":
			replaced = append(replaced, r.Dir)
		case e.takenOver(r, owners):
			checks = append(checks, check{ok: true, label: fmt.Sprintf("your includeIf rule for %s stays: %s", r.Dir, r.Keep)})
		default:
			checks = append(checks, check{ok: true, label: fmt.Sprintf("your includeIf rule for %s keeps applying there%s (%s)",
				r.Dir, pending, author(r.Name, r.Email))})
		}
	}
	if len(replaced) > 0 {
		checks = append(checks, check{warn: true,
			label: fmt.Sprintf("%s still has includeIf identity rules that your profiles take over (%s)",
				e.display(e.GlobalGitConfigPath()), strings.Join(replaced, ", ")),
			advice: "a profile with the same identity covers them; `gitpersona doctor --fix` removes them (after a backup)",
			fix:    RemoveOldRules, fixKey: FixOldRules})
	}
	return checks
}

// skippedRuleCheck warns about a rule GitPersona cannot include again: its
// rules override it there, and when it sets an identity, says which one
// commits use instead.
func (e Env) skippedRuleCheck(k skippedRule, id discover.Rule, owners []ruleOwner, defaultID string) check {
	c := check{warn: true,
		label: fmt.Sprintf("GitPersona's rules override your includeIf rule %q: %s", k.Condition, k.Why),
		advice: "GitPersona cannot include it again after its own rules; move those settings out of the rule's file, " +
			"or give its folder to a profile"}
	if id.Email == "" {
		return c
	}
	if by, ok := e.overriddenBy(id.Dir, owners, defaultID); ok && !by.sameIdentity(id.Email, id.Name) {
		c.label += fmt.Sprintf(", so commits in %s use %s instead of %s", id.Dir, author(by.name, by.email), author(id.Name, id.Email))
	}
	if coversWith(e.comparisonKey, ownerDirs(owners), id.Dir) {
		c.advice = "move those settings out of the rule's file, so GitPersona can include it again after its own rules"
	}
	return c
}

// ownerDirs returns every profile directory.
func ownerDirs(owners []ruleOwner) []string {
	var dirs []string
	for _, o := range owners {
		dirs = append(dirs, o.dirs...)
	}
	return dirs
}

// author writes an identity as Git shows it: Name <email>, or <email>.
func author(name, email string) string {
	if name == "" {
		return "<" + email + ">"
	}
	return name + " <" + email + ">"
}

// checkSigning catches signing that goes wrong (a GPG key written as a file
// is already refused by the configuration's validation): a profile without
// signing whose directories or pins lie inside an SSH-signing profile's
// (or that profile is the default): there `git commit -S` uses that key,
// because Git cannot unset a key and the user has none of their own.
func (e Env) checkSigning(ctx context.Context, cfg config.Config) []check {
	var checks []check
	var signers []config.Profile
	for _, p := range cfg.Profiles {
		if p.Signing.Method == config.SigningSSH {
			signers = append(signers, p)
		}
	}
	if len(signers) == 0 {
		return checks
	}
	opts, err := e.generateOptions(ctx)
	if err != nil || opts.FallbackSigning.SigningKey != "" || opts.FallbackSigning.Format != "ssh" {
		return checks
	}
	for _, p := range cfg.Profiles {
		if p.Signing.Method != "" {
			continue
		}
		if s, ok := e.enclosingSigner(p, cfg, signers); ok {
			checks = append(checks, check{warn: true,
				label: fmt.Sprintf("profile %s has no signing, so `git commit -S` in its directories uses profile %s's SSH key", p.ID, s),
				advice: "Git cannot unset a signing key and your Git settings name none; give the profile its own key, " +
					"or signing = { method = \"none\" } to make `git commit -S` refuse there"})
		}
	}
	return checks
}

// enclosingSigner returns the signing profile whose settings Git reads
// before p's in p's directories or pins: the default, or one whose
// directory encloses them.
func (e Env) enclosingSigner(p config.Profile, cfg config.Config, signers []config.Profile) (string, bool) {
	places := slices.Clone(p.Dirs)
	for _, pin := range cfg.Pins {
		if pin.Profile == p.ID {
			places = append(places, pin.Repo)
		}
	}
	for _, s := range signers {
		if s.ID == p.ID {
			continue
		}
		for _, place := range places {
			if s.ID == cfg.Default || coversWith(e.comparisonKey, s.Dirs, place) {
				return s.ID, true
			}
		}
	}
	return "", false
}
