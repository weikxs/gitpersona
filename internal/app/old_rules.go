package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/fsutil"
)

// oldRulesCommand names the backups RemoveOldRules makes.
const oldRulesCommand = "remove old includeIf rules"

// RemoveOldRules deletes the hand-made includeIf identity rules from the
// global Git configuration, after a backup. GitPersona's rules already come
// after them and win, so this only tidies up; every other setting stays.
func RemoveOldRules(ctx context.Context, e Env) error {
	path := e.resolvedUserGitConfig()
	conditions := discover.IdentityConditions(ctx, e.Home, path, e.Run)
	if len(conditions) == 0 {
		fmt.Fprintln(e.Stdout, "No old includeIf identity rules; nothing to change.")
		return nil
	}
	current, err := readOptional(path)
	if err != nil {
		return err
	}
	updated, err := e.withoutSections(ctx, current, conditions)
	if err != nil {
		return err
	}
	perm := fsutil.FilePerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm() // the file belongs to the user; keep its mode
		if perm&userWritablePerm == 0 {
			return fmt.Errorf("%s is read-only; make it writable (chmod u+w) to remove the old rules", e.display(e.userGitConfig()))
		}
	}
	if err := e.applyChanges([]change{{path: path, content: updated, perm: perm}},
		ApplyOptions{Command: oldRulesCommand}); err != nil {
		return err
	}
	for _, c := range conditions {
		fmt.Fprintf(e.Stdout, "Removed the old includeIf rule for %s.\n", strings.TrimSuffix(strings.TrimPrefix(
			strings.TrimPrefix(c, "gitdir/i:"), "gitdir:"), "/"))
	}
	return nil
}

// withoutSections lets Git remove the includeIf sections from a copy of
// text, so the result is exactly what Git itself would write.
func (e Env) withoutSections(ctx context.Context, text string, conditions []string) (string, error) {
	tmp, err := os.CreateTemp("", "gitpersona-gitconfig-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	for _, c := range conditions {
		if out, err := e.Run(ctx, "git", "config", "--file", tmp.Name(), "--remove-section", "includeIf."+c); err != nil {
			return "", fmt.Errorf("could not remove the includeIf rule %q: %v %s", c, err, strings.TrimSpace(string(out)))
		}
	}
	data, err := os.ReadFile(tmp.Name())
	return string(data), err
}
