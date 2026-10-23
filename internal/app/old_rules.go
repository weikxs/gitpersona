package app

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
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
	current, err := readOptional(path)
	if err != nil {
		return err
	}
	updated, removed, kept := e.withoutOldRules(ctx, current, nil)
	for _, k := range kept {
		fmt.Fprintf(e.Stdout, "Kept the includeIf rule for %s: %s.\n", k.Dir, k.Why)
	}
	if len(removed) == 0 {
		fmt.Fprintln(e.Stdout, "No old includeIf identity rules that GitPersona replaces; nothing to change.")
		return nil
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
	for _, dir := range removed {
		fmt.Fprintf(e.Stdout, "Removed the old includeIf rule for %s.\n", dir)
	}
	return nil
}

// withoutOldRules removes from text, the content of the global Git
// configuration, the includeIf identity rules for dirs (every one when dirs
// is nil) that a profile fully replaces. It returns the new text, the
// directories whose rules went, and the rules that stay with the reason.
func (e Env) withoutOldRules(ctx context.Context, text string, dirs []string) (string, []string, []KeptRule) {
	replaced, kept := e.classifyOldRules(ctx, dirs)
	conditions, removed := make([]string, len(replaced)), make([]string, len(replaced))
	for i, r := range replaced {
		conditions[i], removed[i] = r.Condition, r.Dir
	}
	return withoutIncludeIfSections(text, conditions), removed, kept
}

// classifyOldRules splits the hand-made includeIf identity rules for dirs
// (every one when dirs is nil) into those a profile replaces and those that
// stay because they do more.
func (e Env) classifyOldRules(ctx context.Context, dirs []string) ([]discover.Rule, []KeptRule) {
	var replaced []discover.Rule
	kept := []KeptRule{}
	for _, r := range discover.IdentityRules(ctx, e.Home, e.resolvedUserGitConfig(), e.Run) {
		switch {
		case dirs != nil && !slices.Contains(dirs, r.Dir):
		case r.Keep != "":
			kept = append(kept, KeptRule{Dir: r.Dir, Why: r.Keep})
		default:
			replaced = append(replaced, r)
		}
	}
	return replaced, kept
}

// KeptRule is a hand-made includeIf rule that stays because it does more
// than a profile can, and why.
type KeptRule struct {
	Dir string `json:"dir"`
	Why string `json:"why"`
}

// includeIfHeader matches an includeIf section header and captures its
// quoted condition. Section names are case-insensitive in Git.
var includeIfHeader = regexp.MustCompile(`(?i)^\s*\[\s*includeif\s+"((?:[^"\\]|\\.)*)"\s*\]`)

// withoutIncludeIfSections removes from a Git configuration the includeIf
// sections for conditions: each header and every setting up to the next
// section header. Comments and blank lines stay, unlike with `git config
// --remove-section`, which also drops comments that follow a section (such
// as GitPersona's own block marker).
func withoutIncludeIfSections(text string, conditions []string) string {
	lines := strings.SplitAfter(text, "\n")
	out := make([]string, 0, len(lines))
	removing := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "["):
			m := includeIfHeader.FindStringSubmatch(trimmed)
			removing = m != nil && slices.Contains(conditions, unescapeSubsection(m[1]))
			if removing {
				continue
			}
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";"):
			// Blank lines and comments stay (GitPersona's block marker may
			// follow a section); the settings around them still go.
			out = append(out, line)
			continue
		}
		if !removing {
			out = append(out, line)
		}
	}
	return strings.Join(out, "")
}

// unescapeSubsection undoes the \" and \\ escapes of a quoted subsection.
func unescapeSubsection(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
