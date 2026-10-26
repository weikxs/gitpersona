package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/weikxs/gitpersona/internal/backup"
	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/i18n"
)

// Rollback lists the backups (when which is empty) or restores one ("latest"
// or a snapshot name). Restoring needs yes, and the current state is backed
// up first so the rollback itself can be undone.
func Rollback(ctx context.Context, e Env, which string, yes bool) error {
	infos, err := backup.List(e.backupDir())
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(e.Stdout, e.tr("No backups yet."))
		return nil
	}
	if which == "" {
		for i, info := range infos {
			name := filepath.Base(info.Dir)
			if i == 0 {
				name = e.tr("%s (latest)", name)
			}
			made := ""
			if info.Manifest.Command != "" {
				made = e.tr("  by `gitpersona %s`", info.Manifest.Command)
			}
			fmt.Fprintf(e.Stdout, "%s%s\n    %s\n", name, made, strings.Join(e.entryNames(info.Manifest), ", "))
		}
		fmt.Fprintln(e.Stdout, "\n"+e.tr("Restore one with: gitpersona rollback <name|latest> --yes"))
		return nil
	}

	chosen, err := pickSnapshot(infos, which)
	if err != nil {
		return err
	}
	names := e.entryNames(chosen.Manifest)
	if !yes {
		return e.trErr("rollback would restore %s; run `gitpersona rollback %s --yes`", strings.Join(names, ", "), which)
	}
	paths := make([]string, len(chosen.Manifest.Entries))
	for i, en := range chosen.Manifest.Entries {
		paths[i] = en.Path
	}
	undo, err := e.restoreSnapshot(chosen.Dir, "rollback "+which, paths)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, e.tr("Restored %s from %s.\nThe state before the rollback is saved in %s.",
		strings.Join(names, ", "), filepath.Base(chosen.Dir), e.display(undo)))
	return e.regenerateAfterRollback(ctx)
}

// restoreSnapshot backs up paths and then restores the snapshot in dir,
// without being interrupted in between. It returns the new backup.
func (e Env) restoreSnapshot(dir, command string, paths []string) (string, error) {
	defer e.Writes.Writing()()
	undo, err := backup.SnapshotFor(e.backupDir(), e.Now(), command, paths)
	if err != nil {
		return "", err
	}
	if err := backup.Restore(dir); err != nil {
		// Restore reads every stored copy first; a failure while writing
		// leaves files of both states, so the state before is put back.
		if rerr := backup.Restore(undo); rerr != nil {
			return "", e.trErr("%w; putting the previous files back also failed (%v): they are saved in %s", err, rerr, e.display(undo))
		}
		return "", e.trErr("%w; nothing was changed: every file was put back as it was (backup in %s)", err, e.display(undo))
	}
	return undo, nil
}

// regenerateAfterRollback makes the generated files match the restored
// configuration. A backup only holds the files its command changed, so the
// profile files the restored rules include may have changed since. When the
// restored global Git configuration no longer includes GitPersona (a backup
// from before it was set up), nothing is regenerated.
func (e Env) regenerateAfterRollback(ctx context.Context) error {
	gitconfig, err := readOptional(e.resolvedUserGitConfig())
	if err != nil || !block.Contains(gitconfig) {
		return err
	}
	if _, err := os.Stat(e.configFile()); err != nil {
		return nil
	}
	fmt.Fprintln(e.Stdout, e.tr("Regenerating the Git configuration for the restored settings:"))
	return Apply(ctx, e, ApplyOptions{Yes: true, NoPrune: true})
}

func pickSnapshot(infos []backup.Info, which string) (backup.Info, error) {
	if which == "latest" {
		return infos[0], nil
	}
	for _, info := range infos {
		if filepath.Base(info.Dir) == which {
			return info, nil
		}
	}
	return backup.Info{}, i18n.Errorf("no backup named %q; run `gitpersona rollback` to list them", which)
}

func (e Env) entryNames(m backup.Manifest) []string {
	names := make([]string, len(m.Entries))
	for i, en := range m.Entries {
		names[i] = e.display(en.Path)
	}
	return names
}

// Uninstall removes the GitPersona block from the global Git configuration
// and deletes the generated files. config.toml and the backups are kept.
func Uninstall(_ context.Context, e Env, yes bool) error {
	current, err := readOptional(e.resolvedUserGitConfig())
	if err != nil {
		return err
	}
	restored := e.removedRules(current)
	changes, err := e.planUninstall()
	if err != nil {
		return err
	}
	if !yes {
		names := make([]string, len(changes))
		for i, c := range changes {
			names[i] = e.display(c.path)
		}
		return e.trErr("uninstall would change %s; run `gitpersona uninstall --yes`", strings.Join(names, ", "))
	}
	if err := e.applyChanges(changes, ApplyOptions{Yes: true, Command: "uninstall"}); err != nil {
		return err
	}
	for _, r := range restored {
		fmt.Fprintln(e.Stdout, e.tr("Put back your includeIf rule %q, which GitPersona had taken over.", r.condition))
	}
	_ = os.Remove(filepath.Join(e.ConfigDir, "profiles")) // only succeeds when empty
	_ = os.Remove(e.shimDir())
	fmt.Fprintln(e.Stdout, e.tr("GitPersona is uninstalled. Your configuration stays in %s.", e.display(e.configFile())))
	return nil
}

func (e Env) planUninstall() ([]change, error) {
	changes, err := e.removeShellBlocks()
	if err != nil {
		return nil, err
	}
	gitconfig := e.resolvedUserGitConfig()
	current, err := readOptional(gitconfig)
	if err != nil {
		return nil, err
	}
	if block.Contains(current) {
		without, err := block.Remove(current)
		if err != nil {
			return nil, err
		}
		perm := os.FileMode(0)
		if info, err := os.Stat(gitconfig); err == nil {
			perm = info.Mode().Perm()
		}
		without = withRules(without, e.removedRules(without))
		// A file that only held the GitPersona block was created by it.
		changes = append(changes, change{path: gitconfig, content: without, perm: perm, remove: strings.TrimSpace(without) == ""})
	}
	for _, entry := range []string{e.ghEntry(), e.glabEntry()} {
		if _, err := os.Lstat(entry); err == nil {
			changes = append(changes, change{path: entry, remove: true})
		}
	}
	generated, _ := filepath.Glob(filepath.Join(e.ConfigDir, "profiles", "*.gitconfig"))
	generated = append(generated, filepath.Join(e.ConfigDir, generate.RulesFile))
	for _, path := range generated {
		if _, err := os.Stat(path); err == nil {
			changes = append(changes, change{path: path, remove: true})
		}
	}
	return changes, nil
}

// ruleSection is an includeIf section of a Git configuration: its
// condition, its text (the header and its settings), and the header of the
// section that follows it, if any, with which occurrence of that header it
// is (headers such as [core] can repeat).
type ruleSection struct {
	condition, text, before string
	beforeN                 int
	nextRule                string // the condition of the next includeIf rule
}

// removedRules returns the includeIf sections that setup or `doctor --fix`
// removed from the global Git configuration (as their backups record) and
// that current does not have, oldest backup first.
func (e Env) removedRules(current string) []ruleSection {
	infos, err := backup.List(e.backupDir())
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for _, s := range includeIfSections(current) {
		have[s.condition] = true
	}
	var out []ruleSection
	for i := len(infos) - 1; i >= 0; i-- {
		data, err := os.ReadFile(filepath.Join(infos[i].Dir, removedRulesFile))
		if err != nil {
			continue
		}
		for _, s := range includeIfSections(string(data)) {
			if !have[s.condition] {
				have[s.condition] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// includeIfSections returns the includeIf sections of a Git configuration,
// each with its header and settings (not the comments around them), keeping
// each line's own ending, and the header that follows it. In a record of
// removed rules, a beforeMarker line ("<occurrence> <header>") says that.
func includeIfSections(text string) []ruleSection {
	var out []ruleSection
	inside, marked, markedN, markedNext := false, "", 0, ""
	seen := map[string]int{} // header -> occurrences so far
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, strings.TrimSpace(nextRuleMarker)); ok {
			markedNext = strings.TrimSpace(rest)
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, strings.TrimSpace(beforeMarker)); ok {
			n, header, _ := strings.Cut(strings.TrimSpace(rest), " ")
			markedN, _ = strconv.Atoi(n)
			marked = header
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			seen[trimmed]++
			if inside && out[len(out)-1].before == "" {
				out[len(out)-1].before, out[len(out)-1].beforeN = trimmed, seen[trimmed]
			}
			m := includeIfHeader.FindStringSubmatch(trimmed)
			inside = m != nil
			if inside {
				condition := unescapeSubsection(m[1])
				for k := len(out) - 1; k >= 0 && out[k].nextRule == "" && markedNext == "" && marked == ""; k-- {
					out[k].nextRule = condition // the first rule after each earlier one
				}
				out = append(out, ruleSection{condition: condition, before: marked, beforeN: markedN, nextRule: markedNext})
				marked, markedN, markedNext = "", 0, ""
			}
		}
		if inside && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
			if !strings.HasSuffix(line, "\n") {
				line += "\n"
			}
			out[len(out)-1].text += line
		}
	}
	return out
}

// withRules puts sections back into a Git configuration's text: each before
// the section that followed it, when that is there; the others at the end,
// in order. Placing one can make room for another, so it goes round until
// nothing more fits.
func withRules(text string, sections []ruleSection) string {
	pending := slices.Clone(sections)
	for placed := true; placed && len(pending) > 0; {
		placed = false
		for i := len(pending) - 1; i >= 0; i-- {
			s := pending[i]
			at := -1
			if s.before != "" {
				at = sectionLine(text, s.before, s.beforeN)
			}
			if at < 0 && s.nextRule != "" {
				at = ruleLine(text, s.nextRule) // the section after it is gone: before the next rule
			}
			if at >= 0 {
				text = text[:at] + s.text + text[at:]
				pending = slices.Delete(pending, i, i+1)
				placed = true
			}
		}
	}
	if len(pending) > 0 && text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	for _, s := range pending {
		text += s.text
	}
	return text
}

// ruleLine returns where the header of the includeIf rule for condition
// starts in text, or -1.
func ruleLine(text, condition string) int {
	at := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if m := includeIfHeader.FindStringSubmatch(strings.TrimSpace(line)); m != nil && unescapeSubsection(m[1]) == condition {
			return at
		}
		at += len(line)
	}
	return -1
}

// sectionLine returns where the n-th line whose trimmed text is header
// starts in text, or -1.
func sectionLine(text, header string, n int) int {
	at, count := 0, 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.TrimSpace(line) == header {
			if count++; count == max(n, 1) {
				return at
			}
		}
		at += len(line)
	}
	return -1
}
