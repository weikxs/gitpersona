package app

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// checkOldRules warns about hand-made includeIf identity rules left in the
// global Git configuration from before GitPersona. GitPersona's rules come
// last and win, but the old ones are confusing to keep.
func (e Env) checkOldRules(ctx context.Context) []check {
	old, err := discover.Existing(ctx, e.Home, e.GlobalGitConfigPath(), e.Run)
	if err != nil || len(old) == 0 {
		return nil
	}
	var dirs []string
	for _, id := range old {
		dirs = append(dirs, id.Dirs...)
	}
	c := check{warn: true,
		label: fmt.Sprintf("%s still has includeIf identity rules from before GitPersona (%s)",
			e.display(e.GlobalGitConfigPath()), strings.Join(dirs, ", ")),
		advice: "GitPersona's rules come after them and win; they also set other things, so remove them by hand when you no longer need them"}
	if replaced, _ := e.classifyOldRules(ctx, nil); len(replaced) > 0 {
		c.advice = "GitPersona's rules come after them and win; `gitpersona doctor --fix` removes those a profile replaces (after a backup)"
		c.fix, c.fixKey = func(ctx context.Context, fixer Env) error { return RemoveOldRules(ctx, fixer) }, FixOldRules
	}
	return []check{c}
}
