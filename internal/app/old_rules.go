package app

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/fsutil"
)

// oldRulesCommand names the backups RemoveOldRules makes.
const oldRulesCommand = "remove old includeIf rules"

// RemoveOldRules deletes from the global Git configuration, after a backup,
// the hand-made includeIf identity rules a profile takes over: one with the
// same email covers the rule's directory. GitPersona's rules already come
// after them and win, so this only tidies up; every other rule and setting
// stays.
func RemoveOldRules(ctx context.Context, e Env) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	path := e.resolvedUserGitConfig()
	current, err := readOptional(path)
	if err != nil {
		return err
	}
	updated, removed, kept := e.withoutOldRules(ctx, current, ownersOf(cfg), nil)
	for _, k := range kept {
		fmt.Fprintf(e.Stdout, "Kept the includeIf rule for %s: %s.\n", k.Dir, k.Why)
	}
	if len(removed) == 0 {
		fmt.Fprintln(e.Stdout, "No old includeIf identity rules that a profile takes over; nothing to change.")
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

// ruleOwner is what an old rule is compared with: a profile's id, email and
// directories.
type ruleOwner struct {
	id, email string
	dirs      []string
}

func ownersOf(cfg config.Config) []ruleOwner {
	owners := make([]ruleOwner, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		owners[i] = ruleOwner{id: p.ID, email: p.Email, dirs: p.Dirs}
	}
	return owners
}

// takenOver reports whether a profile with the rule's email covers the
// rule's directory, so the rule adds nothing.
func (e Env) takenOver(r discover.Rule, owners []ruleOwner) bool {
	return slices.ContainsFunc(owners, func(o ruleOwner) bool {
		return strings.EqualFold(o.email, r.Email) && coversWith(e.comparisonKey, o.dirs, r.Dir)
	})
}

// overriddenBy returns the profile that applies in dir instead of an old
// rule there, because GitPersona's rules come last: the most specific
// directory that covers it, or else the default. ok is false when none does.
func (e Env) overriddenBy(dir string, owners []ruleOwner, defaultID string) (ruleOwner, bool) {
	best, found := -1, -1
	for i, o := range owners {
		for _, d := range o.dirs {
			if coversWith(e.comparisonKey, []string{d}, dir) && len(e.comparisonKey(d)) > best {
				best, found = len(e.comparisonKey(d)), i
			}
		}
	}
	if found < 0 {
		found = slices.IndexFunc(owners, func(o ruleOwner) bool { return o.id == defaultID && defaultID != "" })
	}
	if found < 0 {
		return ruleOwner{}, false
	}
	return owners[found], true
}

// withoutOldRules removes from text, the content of the global Git
// configuration, the includeIf identity rules that owners take over, only
// for the directories in only when it is not nil. It returns the new text,
// the directories whose rules went, and the rules taken over that stay
// because they do more.
func (e Env) withoutOldRules(ctx context.Context, text string, owners []ruleOwner, only []string) (string, []string, []KeptRule) {
	replaced, kept := e.classifyOldRules(ctx, owners, only)
	conditions, removed := make([]string, len(replaced)), make([]string, len(replaced))
	for i, r := range replaced {
		conditions[i], removed[i] = r.Condition, r.Dir
	}
	return withoutIncludeIfSections(text, conditions), removed, kept
}

// classifyOldRules returns the hand-made includeIf identity rules that
// owners take over (only for the directories in only, when it is not nil),
// split into those that can go and those that stay because they do more.
func (e Env) classifyOldRules(ctx context.Context, owners []ruleOwner, only []string) ([]discover.Rule, []KeptRule) {
	var replaced []discover.Rule
	kept := []KeptRule{}
	for _, r := range discover.IdentityRules(ctx, e.Home, e.resolvedUserGitConfig(), e.Run) {
		switch {
		case only != nil && !slices.Contains(only, r.Dir), !e.takenOver(r, owners):
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
