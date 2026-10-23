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
	if p.Signing.Method == config.SigningSSH && p.Signing.Key != "" && !exists(p.Signing.Key) {
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
// global Git configuration. Rules a profile takes over are a warning with a
// fix (they are only confusing); rules that stay because they do more are
// fine; a rule whose identity no profile has is a warning when a profile
// overrides it, because GitPersona's rules come last.
func (e Env) checkOldRules(ctx context.Context, cfg config.Config) []check {
	owners := ownersOf(cfg)
	var checks []check
	var replaced []string
	for _, r := range discover.IdentityRules(ctx, e.Home, e.GlobalGitConfigPath(), e.Run) {
		by, overridden := e.overriddenBy(r.Dir, owners, cfg.Default)
		overridden = overridden && !by.sameIdentity(r.Email, r.Name) // the same identity is no loss
		switch {
		case e.takenOver(r, owners) && r.Keep == "":
			replaced = append(replaced, r.Dir)
		case e.takenOver(r, owners):
			checks = append(checks, check{ok: true, label: fmt.Sprintf("your includeIf rule for %s stays: %s", r.Dir, r.Keep)})
		case overridden:
			checks = append(checks, check{warn: true,
				label: fmt.Sprintf("profile %s overrides your includeIf rule for %s, so commits there do not use %s", by.id, r.Dir, author(r.Name, r.Email)),
				advice: fmt.Sprintf("add %s to a profile with that identity (or pin its repositories); remove the rule from %s by hand if you no longer need it",
					r.Dir, e.display(e.GlobalGitConfigPath()))})
		default:
			checks = append(checks, check{ok: true, label: fmt.Sprintf("your includeIf rule for %s still applies there (%s)", r.Dir, r.Email)})
		}
	}
	if len(replaced) > 0 {
		checks = append(checks, check{warn: true,
			label: fmt.Sprintf("%s still has includeIf identity rules that your profiles take over (%s)",
				e.display(e.GlobalGitConfigPath()), strings.Join(replaced, ", ")),
			advice: "GitPersona's rules come after them and win; `gitpersona doctor --fix` removes them (after a backup)",
			fix:    RemoveOldRules, fixKey: FixOldRules})
	}
	return checks
}

// author writes an identity as Git shows it: Name <email>, or <email>.
func author(name, email string) string {
	if name == "" {
		return "<" + email + ">"
	}
	return name + " <" + email + ">"
}

// checkSigning catches signing that cannot work: a GPG key written as a
// file path (a key from an SSH setup read as GPG), and a profile without
// signing whose directories or pins lie inside an SSH-signing profile's
// (or that profile is the default): there `git commit -S` uses that key,
// because Git cannot unset a key and the user has none of their own.
func (e Env) checkSigning(ctx context.Context, cfg config.Config) []check {
	var checks []check
	var signers []config.Profile
	for _, p := range cfg.Profiles {
		if p.Signing.Method == config.SigningGPG && (strings.ContainsRune(p.Signing.Key, '/') || strings.HasSuffix(p.Signing.Key, ".pub")) {
			checks = append(checks, check{
				label:  fmt.Sprintf("profile %s signs with GPG, but its key %s is a file, as SSH signing uses", p.ID, p.Signing.Key),
				advice: "Git fails every commit there; set signing = { method = \"ssh\", key = \"" + p.Signing.Key + "\" }"})
		}
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
