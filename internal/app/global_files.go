package app

// The files of the user's global Git configuration, read the way Git reads
// them, so GitPersona's fallbacks are the user's own settings (also those in
// dotfiles), never its own.

import (
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/paths"
)

// xdgGitConfig is ~/.config/git/config (under XDG_CONFIG_HOME), which Git
// reads before the global file unless GIT_CONFIG_GLOBAL is set; "" then.
func (e Env) xdgGitConfig() string {
	if e.GlobalGitConfig != "" {
		return ""
	}
	return filepath.Join(filepath.Dir(e.ConfigDir), "git", "config")
}

// maxIncludeDepth is how deep Git follows includes.
const maxIncludeDepth = 10

// includedFile resolves a plain include path written in file, or returns ""
// for GitPersona's own files.
func (e Env) includedFile(path, file string) string {
	if !strings.HasPrefix(path, "~/") && !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(file), path)
	}
	norm, err := paths.Normalize(path, e.Home)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(paths.ResolveSymlinks(norm), paths.ResolveSymlinks(e.ConfigDir)+string(filepath.Separator)) {
		return ""
	}
	return norm
}

// includeHeader matches a plain [include] section header.
var includeHeader = regexp.MustCompile(`(?i)^\s*\[\s*include\s*\]`)

// gitValue reads a Git configuration value: an inline comment outside
// quotes ends it, quotes are removed and \" and \\ unescaped.
func gitValue(raw string) string {
	var b strings.Builder
	quoted := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '\\' && i+1 < len(raw):
			i++
			b.WriteByte(unescape(raw[i]))
		case c == '"':
			quoted = !quoted
		case (c == '#' || c == ';') && !quoted:
			return strings.TrimSpace(b.String())
		default:
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// unescape returns the character a backslash escape stands for in a Git
// configuration value.
func unescape(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'b':
		return '\b'
	}
	return c
}

// globalFiles are the files of the user's global configuration in the order
// Git reads them: ~/.config/git/config, the files the global file includes,
// then the global file (an approximation: Git reads an included file where
// the include stands). GitPersona's own files are left out.
func (e Env) globalFiles() []string {
	var files []string
	if xdg := e.xdgGitConfig(); xdg != "" {
		files = append(files, xdg)
	}
	text, err := readOptional(e.resolvedUserGitConfig())
	if err == nil {
		seen := map[string]bool{}
		var walk func(text, file string, depth int)
		walk = func(text, file string, depth int) {
			for _, path := range plainIncludes(text) {
				included := e.includedFile(path, file)
				if included == "" || seen[included] || depth >= maxIncludeDepth {
					continue
				}
				seen[included] = true
				content, err := readOptional(included)
				if err != nil {
					continue
				}
				walk(content, included, depth+1)
				files = append(files, included)
			}
		}
		walk(text, e.userGitConfig(), 0)
	}
	return append(files, e.resolvedUserGitConfig())
}

// plainIncludes returns the paths of a configuration's [include] sections.
func plainIncludes(text string) []string {
	var out []string
	inside := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			m := includeHeader.FindString(trimmed)
			inside = m != ""
			trimmed = strings.TrimSpace(trimmed[len(m):])
		}
		if key, value, ok := strings.Cut(trimmed, "="); inside && ok && strings.EqualFold(strings.TrimSpace(key), "path") {
			out = append(out, gitValue(value))
		}
	}
	return out
}

// signingKeys are the settings a profile resets when a rule of the user's
// sets them where the profile applies.
var signingKeys = []string{"user.signingkey", "gpg.format", "commit.gpgsign", "tag.gpgsign"}

// keyRules returns the user's own includeIf rules that set signing or an
// SSH command, from every global file; text is the global file's content
// as it will be (rules in it that are gone do not count).
func (e Env) keyRules(ctx context.Context, text string) []generate.KeyRule {
	present := map[string]bool{}
	for _, s := range includeIfSections(text) {
		present[s.condition] = true
	}
	main := e.resolvedUserGitConfig()
	var out []generate.KeyRule
	for _, file := range e.globalFiles() {
		for _, r := range discover.RulesSettings(ctx, e.Home, file, e.Run) {
			if file == main && !present[r.Condition] {
				continue
			}
			identity := r.Keys["user.email"] || r.Keys["user.name"]
			k := generate.KeyRule{
				Signing: slices.ContainsFunc(signingKeys, func(key string) bool { return r.Keys[key] }),
				// An SSH command matters when it is another identity's key; a
				// proxy or a port alone may stay.
				SSH:      r.Keys["core.sshcommand"] && (identity || r.SSHKey),
				Identity: identity,
			}
			if !k.Signing && !k.SSH {
				continue
			}
			// The rule's own condition, unless it is relative to its file.
			if !strings.Contains(r.Condition, ":./") {
				k.Condition = r.Condition
			}
			// A folder compared as text: not for patterns, nor for a
			// case-insensitive rule on a case-sensitive disk (it applies
			// anywhere then, which is the safe side).
			plain := !strings.ContainsAny(r.Dir, "*?[") && (!strings.HasPrefix(r.Condition, "gitdir/i:") || e.CaseInsensitive)
			if r.Dir != "" && r.Dir != "~" && plain {
				if norm, err := paths.Normalize(r.Dir, e.Home); err == nil {
					k.Dir = norm
				}
			}
			out = append(out, k)
		}
	}
	return out
}

// identityRules returns the includeIf identity rules of every file of the
// global configuration, in the order Git reads them. Rules outside the
// global file itself carry their File: GitPersona does not edit it.
func (e Env) identityRules(ctx context.Context) []discover.Rule {
	main := e.resolvedUserGitConfig()
	var rules []discover.Rule
	for _, file := range e.globalFiles() {
		for _, r := range discover.IdentityRules(ctx, e.Home, file, e.Run) {
			if file != main {
				r.File = e.display(file)
			}
			rules = append(rules, r)
		}
	}
	return rules
}

// otherIdentityRules returns the identity rules that are not directory rules
// (onbranch, hasconfig, the whole home) of every global file.
func (e Env) otherIdentityRules(ctx context.Context) []discover.Rule {
	var rules []discover.Rule
	for _, file := range e.globalFiles() {
		rules = append(rules, discover.OtherIdentityRules(ctx, e.Home, file, e.Run)...)
	}
	return rules
}

// existingIdentities returns the identities of the includeIf rules of every
// global file; one identity set in several files is one, with all its
// directories.
func (e Env) existingIdentities(ctx context.Context) []discover.Identity {
	var ids []discover.Identity
	for _, file := range e.globalFiles() {
		found, _ := discover.Existing(ctx, e.Home, file, e.Run)
		for _, id := range found {
			i := slices.IndexFunc(ids, func(x discover.Identity) bool { return x.Email == id.Email && x.Name == id.Name })
			if i < 0 {
				ids = append(ids, id)
				continue
			}
			for _, d := range id.Dirs {
				if !slices.Contains(ids[i].Dirs, d) {
					ids[i].Dirs = append(slices.Clone(ids[i].Dirs), d)
				}
			}
		}
	}
	return ids
}

// ExistingIdentities is existingIdentities, for the configuration page.
func (e Env) ExistingIdentities(ctx context.Context) []discover.Identity {
	return e.existingIdentities(ctx)
}

// keyOnlyRules returns the user's directory rules that set no identity but
// signing or an SSH command for their folder (a key for tags, a port): they
// do more than a profile can, so they are kept like such identity rules.
func (e Env) keyOnlyRules(ctx context.Context) []discover.Rule {
	main := e.resolvedUserGitConfig()
	var out []discover.Rule
	for _, file := range e.globalFiles() {
		for _, r := range discover.RulesSettings(ctx, e.Home, file, e.Run) {
			if r.Dir == "" || r.Dir == "~" || r.Keys["user.email"] || r.Keys["user.name"] {
				continue
			}
			var set []string
			for _, k := range append(slices.Clone(signingKeys), "core.sshcommand") {
				if r.Keys[k] {
					set = append(set, k)
				}
			}
			if len(set) == 0 {
				continue
			}
			rule := discover.Rule{Condition: r.Condition, Dir: r.Dir, Keep: "it sets " + strings.Join(set, ", ") + " for its folder"}
			if file != main {
				rule.File = e.display(file)
			}
			out = append(out, rule)
		}
	}
	return out
}

// keptRules returns the user's rules that do more than a profile can: the
// identity rules with a reason to keep, and the rules that only set keys.
func (e Env) keptRules(ctx context.Context) []discover.Rule {
	var out []discover.Rule
	for _, r := range e.identityRules(ctx) {
		if r.Keep != "" {
			out = append(out, r)
		}
	}
	return append(out, e.keyOnlyRules(ctx)...)
}

// ruleKey names one rule: the file it is in ("" for the global file) and
// its condition.
func ruleKey(file, condition string) string { return file + "\x00" + condition }

// orderEvent is something in the global configuration, in the order Git
// reads it, that decides the identity: an identity rule, or an identity set
// for everything (a [user] email outside any rule), from file.
type orderEvent struct {
	rule   *discover.Rule // an identity rule, or nil
	key    string         // the rule's ruleKey
	global bool           // a [user] email set for everything
	file   string         // with ~
}

// readOrder returns the identity rules and global identities of the global
// configuration in the order Git reads them: ~/.config/git/config, then the
// global file, each with the files it includes read where the include
// stands (as often as it stands).
func (e Env) readOrder(ctx context.Context) []orderEvent {
	byRule := map[string]discover.Rule{}
	for _, r := range e.identityRules(ctx) {
		byRule[ruleKey(r.File, r.Condition)] = r
	}
	main := e.resolvedUserGitConfig()
	var out []orderEvent
	var walk func(text, file string, depth int)
	walk = func(text, file string, depth int) {
		fileKey := ""
		if paths.ResolveSymlinks(file) != paths.ResolveSymlinks(main) {
			fileKey = e.display(file)
		}
		section, condition := "", ""
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") {
				section, condition = "", ""
				switch m := includeIfHeader.FindStringSubmatch(trimmed); {
				case m != nil:
					section, condition = "includeif", unescapeSubsection(m[1])
					trimmed = strings.TrimSpace(trimmed[len(m[0]):])
				case includeHeader.MatchString(trimmed):
					section = "include"
					trimmed = strings.TrimSpace(trimmed[len(includeHeader.FindString(trimmed)):])
				case userHeader.MatchString(trimmed):
					section = "user"
					trimmed = strings.TrimSpace(trimmed[len(userHeader.FindString(trimmed)):])
				}
			}
			key, value, ok := strings.Cut(trimmed, "=")
			key = strings.ToLower(strings.TrimSpace(key))
			switch {
			case !ok:
			case section == "user" && key == "email": // the email decides who commits
				out = append(out, orderEvent{global: true, file: e.display(file)})
			case section == "includeif" && key == "path":
				k := ruleKey(fileKey, condition)
				if r, known := byRule[k]; known {
					out = append(out, orderEvent{rule: &r, key: k, file: e.display(file)})
				}
			case section == "include" && key == "path" && depth < maxIncludeDepth:
				if included := e.includedFile(gitValue(value), file); included != "" {
					if content, err := readOptional(included); err == nil {
						walk(content, included, depth+1)
					}
				}
			}
		}
	}
	if xdg := e.xdgGitConfig(); xdg != "" {
		if text, err := readOptional(xdg); err == nil {
			walk(text, xdg, 0)
		}
	}
	if text, err := readOptional(main); err == nil {
		walk(text, e.userGitConfig(), 0)
	}
	return out
}

// userHeader matches a plain [user] section header.
var userHeader = regexp.MustCompile(`(?i)^\s*\[\s*user\s*\]`)

// overriddenAt says why the identity rule at position i of order never
// applies in its folder, or "": a later identity set for everything, or a
// later rule for a folder containing it with another identity (Git applies
// what it reads last).
func (e Env) overriddenAt(order []orderEvent, i int) string {
	r := order[i].rule
	reason := ""
	for _, later := range order[i+1:] {
		covers := later.rule != nil && coversWith(e.comparisonKey, []string{later.rule.Dir}, r.Dir)
		same := later.rule != nil && strings.EqualFold(later.rule.Email, r.Email) && later.rule.Name == r.Name
		switch {
		case later.global:
			reason = "the identity set for everything in " + later.file + " comes after it"
		case covers && !same:
			reason = "your later rule for " + later.rule.Dir + " (" + author(later.rule.Name, later.rule.Email) + ") overrides it"
		case covers:
			reason = "" // a later rule with the same identity puts it back
		}
	}
	return reason
}

// neverApplied maps the identity rules (by ruleKey) that never apply in
// Git's read order to the reason; a rule read more than once counts by its
// last reading.
func (e Env) neverApplied(ctx context.Context) map[string]string {
	order := e.readOrder(ctx)
	out := map[string]string{}
	for i, ev := range order {
		if ev.rule == nil {
			continue
		}
		if why := e.overriddenAt(order, i); why != "" {
			out[ev.key] = why
		} else {
			delete(out, ev.key)
		}
	}
	return out
}
