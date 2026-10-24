package app

import (
	"cmp"
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/i18n"
	"github.com/weikxs/gitpersona/internal/paths"
)

// missingKeyFiles names the key files of p that do not exist, as messages to
// show in any language. It only checks that the files are there; it never
// opens them.
func missingKeyFiles(p config.Profile, home string) []i18n.Message {
	var missing []i18n.Message
	exists := func(path string) bool { return keyFileExists(path, home, true) }
	if p.SSHKey != "" && !exists(p.SSHKey) {
		missing = append(missing, i18n.M("SSH key %s", p.SSHKey))
	}
	if p.Signing.Method == config.SigningSSH && p.Signing.Key != "" && !config.LiteralSSHKey(p.Signing.Key) && !exists(p.Signing.Key) {
		missing = append(missing, i18n.M("signing key %s", p.Signing.Key))
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
	what := missing[0]
	if len(missing) > 1 {
		what = i18n.M("%s and %s", missing[0], missing[1])
	}
	return i18n.Errorf("%s does not exist; create it first (for example with ssh-keygen -t ed25519 -f <file>), "+
		"or choose another file", what)
}

// checkKeyFiles reports profiles whose key files are missing.
func (e Env) checkKeyFiles(cfg config.Config) []check {
	var checks []check
	for _, p := range cfg.Profiles {
		for _, m := range missingKeyFiles(p, e.Home) {
			checks = append(checks, check{
				label:  e.tr("profile %s: %s does not exist", p.ID, m),
				advice: e.tr("Git fails to sign or push with this profile; create the key or change the profile"),
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
					label:  e.tr("profile %s: directory %s does not exist yet", p.ID, d),
					advice: e.tr("the rule applies once it exists; remove it if it is a typo")})
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
				label:  e.tr("profile %s takes over your includeIf rule for %s in %s", by.id, r.Dir, r.File),
				advice: e.tr("it is no longer needed; remove it from %s yourself (GitPersona does not change that file)", r.File)})
		case e.takenOver(r, owners) && r.Keep == "":
			replaced = append(replaced, r.Dir)
		case e.takenOver(r, owners):
			// The profile covers the folder and comes after the rule, so what
			// the rule does more than the profile is lost there.
			checks = append(checks, check{warn: true,
				label:  e.tr("profile %s covers your includeIf rule for %s, which does more than a profile can (%s); that is lost there", by.id, r.Dir, r.Keep),
				advice: e.tr("take %s out of the profile (`gitpersona remove` there) to let your rule decide", r.Dir)})
		case overridden && !by.sameIdentity(r.Email, r.Name):
			advice := e.tr("give %s to a profile with that identity, or remove the rule from %s", r.Dir, file)
			if by.id == cfg.Default && !coversWith(e.comparisonKey, by.dirs, r.Dir) {
				advice = e.tr("give %s to a profile with that identity, or remove the rule from %s; or set no default profile, which applies everywhere", r.Dir, file)
			}
			checks = append(checks, check{warn: true,
				label: e.tr("profile %s overrides your includeIf rule for %s: commits there use %s instead of %s",
					by.id, r.Dir, author(by.name, by.email), author(r.Name, r.Email)),
				advice: advice})
		case never[ruleKey(r.File, r.Condition)] != "":
			checks = append(checks, check{warn: true,
				label:  e.tr("your includeIf rule for %s (%s) never applies: %s", r.Dir, author(r.Name, r.Email), never[ruleKey(r.File, r.Condition)]),
				advice: e.tr("Git applies what it reads last; move the rule after that in %s, or remove it", file)})
		default:
			label := e.tr("your includeIf rule for %s applies there (%s)", r.Dir, author(r.Name, r.Email))
			if inside := e.profileDirsInside(r.Dir, owners); len(inside) > 0 {
				label = e.tr("your includeIf rule for %s applies there (%s), except in %s, which you gave to profiles",
					r.Dir, author(r.Name, r.Email), strings.Join(inside, ", "))
			}
			checks = append(checks, check{ok: true, label: label})
		}
	}
	checks = append(checks, e.checkCoveredKeptRules(ctx, cfg)...)
	var scanned *discover.Found // the profiles' folders, scanned once if needed
	version, knownVersion := e.gitVersion(ctx)
	for _, r := range e.otherIdentityRules(ctx) {
		// Git before 2.36 skips "hasconfig:" conditions: such a rule never
		// applies anywhere, which says more than any warning below.
		if strings.HasPrefix(r.Condition, "hasconfig:") && knownVersion && !version.atLeast(remoteConditionsGit) {
			checks = append(checks, check{warn: true,
				label: e.tr("your includeIf rule %q (%s) never applies: Git %s ignores hasconfig conditions, which need Git %s or newer",
					r.Condition, author(r.Name, r.Email), version, remoteConditionsGit.short()),
				advice: e.tr("update Git if you want the rule, or remove it")})
			continue
		}
		// A rule whose identity a profile has is covered: pin a repository to
		// that profile where the rule should decide (setup said so).
		represented := slices.ContainsFunc(owners, func(o ruleOwner) bool {
			return o.sameIdentity(r.Email, cmp.Or(r.Name, e.globalIdentity(ctx).Name))
		})
		if cfg.Default == "" && len(ownerDirs(owners)) > 0 && refining(r.Condition) && !represented {
			if scanned == nil {
				found := e.scanProfileDirs(ctx, owners)
				scanned = &found
			}
			ruleAuthor := author(cmp.Or(r.Name, e.globalIdentity(ctx).Name), r.Email)
			affected, known := e.reposMatchingRule(ctx, r.Condition, ruleAuthor, owners, *scanned, false)
			if known && len(affected) == 0 {
				continue // no repository in the profiles' folders follows it today
			}
			label := e.tr("your includeIf rule %q (%s) does not apply in your profiles' folders, where the profile decides",
				r.Condition, author(r.Name, r.Email))
			if known {
				label = e.tr("your includeIf rule %q (%s) does not apply in your profiles' folders, where the profile decides: %d of their repositories match it, such as %s",
					r.Condition, author(r.Name, r.Email), len(affected), affected[0])
			}
			checks = append(checks, check{warn: true, label: label,
				advice: e.tr("pin the repositories where it should apply to a profile with that identity, or remove the rule")})
		}
		if cfg.Default != "" {
			checks = append(checks, check{warn: true,
				label: e.tr("the default profile %s overrides your includeIf rule %q, which sets %s",
					cfg.Default, r.Condition, author(r.Name, r.Email)),
				advice: e.tr("set no default profile, or remove the rule if you no longer need it")})
		}
	}
	if len(replaced) > 0 {
		checks = append(checks, check{warn: true,
			label: e.tr("%s still has includeIf identity rules that your profiles take over (%s)",
				e.display(e.GlobalGitConfigPath()), strings.Join(replaced, ", ")),
			advice: e.tr("a profile with the same identity covers them; `gitpersona doctor --fix` removes them (after a backup)"),
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
				label: e.tr("profile %s has no signing, so `git commit -S` in its directories uses profile %s's SSH key", p.ID, s),
				advice: e.tr("Git cannot unset a signing key and your Git settings name none; give the profile its own key, " +
					"or signing = { method = \"none\" } to make `git commit -S` refuse there")})
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
					places = append(places, e.tr("%s (%s)", d, p.ID))
				}
			}
		}
		for _, pin := range cfg.Pins {
			if coversWith(e.comparisonKey, []string{r.Dir}, pin.Repo) {
				places = append(places, e.tr("%s (pin, %s)", pin.Repo, pin.Profile))
			}
		}
		if len(places) > 0 {
			checks = append(checks, check{warn: true,
				label: e.tr("your includeIf rule for %s does more than a profile can (%s); that is lost in %s",
					r.Dir, r.Keep, strings.Join(places, ", ")),
				advice: e.tr("take those folders out of the profiles (`gitpersona remove`, `gitpersona unpin`) to let your rule decide there")})
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

// scanProfileDirs looks for the repositories in the profiles' folders.
func (e Env) scanProfileDirs(ctx context.Context, owners []ruleOwner) discover.Found {
	roots, err := e.ResolveScanRoots(ownerDirs(owners))
	if err != nil {
		return discover.Found{}
	}
	return e.scannedRepos(ctx, roots)
}
