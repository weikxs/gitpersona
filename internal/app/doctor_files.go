package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
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
// global Git configuration. GitPersona's rules come after them, so in a
// profile's directories (and everywhere, with a default) GitPersona wins.
// Rules a profile takes over are a warning with a fix (they are only
// confusing); rules a profile with another identity overrides are a
// warning; the others apply as before and are fine.
func (e Env) checkOldRules(ctx context.Context, cfg config.Config) []check {
	owners := ownersOf(cfg)
	var checks []check
	var replaced []string
	never := e.neverApplied(ctx)
	for _, r := range e.identityRules(ctx) {
		by, overridden := e.overriddenBy(r.Dir, owners, cfg.Default)
		file := cmp.Or(r.File, e.display(e.GlobalGitConfigPath()))
		switch {
		case e.takenOver(r, owners) && r.Keep == "" && r.File != "":
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("profile %s takes over your includeIf rule for %s in %s", by.id, r.Dir, r.File),
				advice: "it is no longer needed; remove it from " + r.File + " yourself (GitPersona does not change that file)"})
		case e.takenOver(r, owners) && r.Keep == "":
			replaced = append(replaced, r.Dir)
		case e.takenOver(r, owners):
			// The profile covers the folder and comes after the rule, so what
			// the rule does more than the profile is lost there.
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("profile %s covers your includeIf rule for %s, which does more than a profile can (%s); that is lost there", by.id, r.Dir, r.Keep),
				advice: fmt.Sprintf("take %s out of the profile (`gitpersona remove` there) to let your rule decide", r.Dir)})
		case overridden && !by.sameIdentity(r.Email, r.Name):
			advice := fmt.Sprintf("give %s to a profile with that identity, or remove the rule from %s", r.Dir, file)
			if by.id == cfg.Default && !coversWith(e.comparisonKey, by.dirs, r.Dir) {
				advice += "; or set no default profile, which applies everywhere"
			}
			checks = append(checks, check{warn: true,
				label: fmt.Sprintf("profile %s overrides your includeIf rule for %s: commits there use %s instead of %s",
					by.id, r.Dir, author(by.name, by.email), author(r.Name, r.Email)),
				advice: advice})
		case never[ruleKey(r.File, r.Condition)] != "":
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("your includeIf rule for %s (%s) never applies: %s", r.Dir, author(r.Name, r.Email), never[ruleKey(r.File, r.Condition)]),
				advice: "Git applies what it reads last; move the rule after that in " + file + ", or remove it"})
		default:
			label := fmt.Sprintf("your includeIf rule for %s applies there (%s)", r.Dir, author(r.Name, r.Email))
			if inside := e.profileDirsInside(r.Dir, owners); len(inside) > 0 {
				label += ", except in " + strings.Join(inside, ", ") + ", which you gave to profiles"
			}
			checks = append(checks, check{ok: true, label: label})
		}
	}
	checks = append(checks, e.checkCoveredKeptRules(ctx, cfg)...)
	for _, r := range e.otherIdentityRules(ctx) {
		if cfg.Default == "" && len(ownerDirs(owners)) > 0 && refining(r.Condition) {
			checks = append(checks, check{warn: true,
				label: fmt.Sprintf("your includeIf rule %q (%s) does not apply in your profiles' folders, where the profile decides",
					r.Condition, author(r.Name, r.Email)),
				advice: "pin the repositories where it should apply to a profile with that identity, or remove the rule"})
		}
		if cfg.Default != "" {
			checks = append(checks, check{warn: true,
				label: fmt.Sprintf("the default profile %s overrides your includeIf rule %q, which sets %s",
					cfg.Default, r.Condition, author(r.Name, r.Email)),
				advice: "set no default profile, or remove the rule if you no longer need it"})
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

// profileDirsInside returns the profile directories strictly inside dir.
func (e Env) profileDirsInside(dir string, owners []ruleOwner) []string {
	var inside []string
	for _, o := range owners {
		for _, d := range o.dirs {
			if coversWith(e.comparisonKey, []string{dir}, d) && e.comparisonKey(d) != e.comparisonKey(dir) {
				inside = append(inside, d)
			}
		}
	}
	return inside
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

// checkCoveredKeptRules warns about rules of the user's that do more than a
// profile can (a key for tags, x509 signing, an SSH port) where a profile's
// directory or pin lies in their folder: there the profile wins, and that
// is lost. Identity rules a profile covers whole are reported above.
func (e Env) checkCoveredKeptRules(ctx context.Context, cfg config.Config) []check {
	var checks []check
	for _, r := range e.keptRules(ctx) {
		var places []string
		for _, p := range cfg.Profiles {
			for _, d := range p.Dirs {
				inside := coversWith(e.comparisonKey, []string{r.Dir}, d)
				around := coversWith(e.comparisonKey, []string{d}, r.Dir)
				same := e.comparisonKey(d) == e.comparisonKey(r.Dir)
				if (inside || around) && (r.Email == "" || !same) { // an identity rule a profile covers whole is reported above
					places = append(places, d+" ("+p.ID+")")
				}
			}
		}
		for _, pin := range cfg.Pins {
			if coversWith(e.comparisonKey, []string{r.Dir}, pin.Repo) {
				places = append(places, pin.Repo+" (pin, "+pin.Profile+")")
			}
		}
		if len(places) > 0 {
			checks = append(checks, check{warn: true,
				label: fmt.Sprintf("your includeIf rule for %s does more than a profile can (%s); that is lost in %s",
					r.Dir, r.Keep, strings.Join(places, ", ")),
				advice: "take those folders out of the profiles (`gitpersona remove`, `gitpersona unpin`) to let your rule decide there"})
		}
	}
	return checks
}

// ownerDirs returns every profile directory.
func ownerDirs(owners []ruleOwner) []string {
	var dirs []string
	for _, o := range owners {
		dirs = append(dirs, o.dirs...)
	}
	return dirs
}
