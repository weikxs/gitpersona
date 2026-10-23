package app

import (
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/paths"
)

// userRules reads the includeIf rules of the user's global Git
// configuration as Git reads them: ~/.config/git/config first, then the
// global file (text, its content), following its plain includes such as
// dotfiles. GitPersona includes them again among its own rules, so they keep
// working in their folders; only a more specific GitPersona rule or a pin
// replaces them. A rule for a directory that cfg gives to a profile with
// another identity yields to that profile. Rules that cannot be included
// again are returned in skipped, with the reason; GitPersona's rules
// override them.
func (e Env) userRules(ctx context.Context, cfg config.Config, text string) (rules []generate.UserRule, skipped []skippedRule) {
	identities := map[string]discover.Rule{} // condition -> the identity it sets
	for _, r := range discover.IdentityRules(ctx, e.Home, e.resolvedUserGitConfig(), e.Run) {
		identities[r.Condition] = r
	}
	owners := ownersOf(cfg)
	found := e.collectGlobalRules(text)
	for i, f := range found {
		rule, ok := e.userRule(f)
		if !ok {
			continue
		}
		if why := e.cannotIncludeAgain(ctx, rule.Path); why != "" {
			skipped = append(skipped, skippedRule{Condition: f.condition, Why: why})
			continue
		}
		if id, known := identities[f.condition]; known && rule.Dir != "" {
			rule.Yield = e.givenToAnother(rule.Dir, id, owners)
		}
		rule.SameSection = i > 0 && found[i-1].file == f.file && found[i-1].section == f.section
		rules = append(rules, rule)
	}
	return rules, skipped
}

// collectGlobalRules returns the includeIf path settings of the global
// configuration as Git reads it: ~/.config/git/config, then the global file
// (text, its content), with the files they include.
func (e Env) collectGlobalRules(text string) []ruleSource {
	var found []ruleSource
	seen := map[string]bool{e.userGitConfig(): true}
	if xdg := e.xdgGitConfig(); xdg != "" {
		seen[xdg] = true
		if xdgText, err := readOptional(xdg); err == nil {
			e.collectRules(xdgText, xdg, 0, seen, &found)
		}
	}
	e.collectRules(text, e.userGitConfig(), 0, seen, &found)
	return found
}

// xdgGitConfig is ~/.config/git/config (under XDG_CONFIG_HOME), which Git
// reads before the global file unless GIT_CONFIG_GLOBAL is set; "" then.
func (e Env) xdgGitConfig() string {
	if e.GlobalGitConfig != "" {
		return ""
	}
	return filepath.Join(filepath.Dir(e.ConfigDir), "git", "config")
}

// ruleSource is an includeIf path setting and the file it is written in.
type ruleSource struct {
	condition, path string
	file            string // as Git names it (relative paths are relative to its directory)
	section         int    // which section of the file
}

// maxIncludeDepth is how deep Git follows includes.
const maxIncludeDepth = 10

// collectRules adds the includeIf path settings of a configuration's text
// (the content of file) to out, in order, following its plain includes
// except GitPersona's own.
func (e Env) collectRules(text, file string, depth int, seen map[string]bool, out *[]ruleSource) {
	if depth > maxIncludeDepth {
		return
	}
	section, condition, number := "", "", 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			section, condition = "", ""
			number++
			if m := includeIfHeader.FindStringSubmatch(trimmed); m != nil {
				section, condition = "includeif", unescapeSubsection(m[1])
				trimmed = strings.TrimSpace(trimmed[len(m[0]):]) // a setting may follow the header
			} else if m := includeHeader.FindString(trimmed); m != "" {
				section = "include"
				trimmed = strings.TrimSpace(trimmed[len(m):])
			}
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if section == "" || !ok || !strings.EqualFold(strings.TrimSpace(key), "path") {
			continue
		}
		if section == "includeif" {
			*out = append(*out, ruleSource{condition: condition, path: gitValue(value), file: file, section: number})
			continue
		}
		included := e.includedFile(gitValue(value), file)
		if included == "" || seen[included] {
			continue
		}
		seen[included] = true
		if content, err := readOptional(included); err == nil {
			e.collectRules(content, included, depth+1, seen, out)
		}
	}
}

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

// skippedRule is a rule of the user's that GitPersona does not include
// again, and why.
type skippedRule struct {
	Condition string
	Why       string
}

// listKeys are settings Git collects as lists: included a second time, the
// same values would be added twice (two credential helpers, a header sent
// twice), so rules that set them are not included again.
var listKeys = []string{".helper", ".extraheader", ".fetch", ".push", ".pushurl", ".insteadof", ".pushinsteadof"}

// cannotIncludeAgain says why the file a rule includes cannot be included a
// second time, or "": it is GitPersona's own or the global configuration
// (a loop), or it sets settings Git collects as lists.
func (e Env) cannotIncludeAgain(ctx context.Context, path string) string {
	norm, err := paths.Normalize(path, e.Home)
	if err != nil {
		return ""
	}
	resolved := paths.ResolveSymlinks(norm)
	if strings.HasPrefix(resolved, paths.ResolveSymlinks(e.ConfigDir)+string(filepath.Separator)) ||
		resolved == paths.ResolveSymlinks(e.resolvedUserGitConfig()) {
		return "it includes GitPersona's own configuration"
	}
	out, err := e.Run(ctx, "git", "-C", e.Home, "config", "--file", norm, "--includes", "--name-only", "--list")
	if err != nil {
		return "" // a missing file includes nothing, here as there
	}
	for _, key := range strings.Fields(string(out)) {
		if slices.ContainsFunc(listKeys, func(suffix string) bool { return strings.HasSuffix(key, suffix) }) ||
			(strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, ".url")) {
			return "it sets " + key + ", which would be added twice"
		}
	}
	return ""
}

// givenToAnother reports whether a profile with another identity than the
// rule's has exactly the rule's directory.
func (e Env) givenToAnother(dir string, id discover.Rule, owners []ruleOwner) bool {
	for _, o := range owners {
		for _, d := range o.dirs {
			if norm, err := paths.Normalize(d, e.Home); err == nil && e.comparisonKey(norm) == e.comparisonKey(dir) {
				return !o.sameIdentity(id.Email, id.Name)
			}
		}
	}
	return false
}

// userRule makes a rule includable from GitPersona's files. A relative
// path is relative to the file the rule is in as Git names it (not
// following a symlink), while a ./ pattern is relative to its real
// directory, as Git does.
func (e Env) userRule(f ruleSource) (generate.UserRule, bool) {
	condition, path := f.condition, f.path
	if path == "" {
		return generate.UserRule{}, false
	}
	if !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "%(prefix)/") && !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(f.file), path)
	}
	rule := generate.UserRule{Condition: condition, Path: path}
	for _, prefix := range []string{"gitdir:", "gitdir/i:"} {
		pattern, ok := strings.CutPrefix(condition, prefix)
		if !ok {
			continue
		}
		if rest, ok := strings.CutPrefix(pattern, "./"); ok {
			pattern = filepath.Dir(paths.ResolveSymlinks(f.file)) + "/" + rest
			rule.Condition = prefix + pattern
		}
		if !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "~/") {
			rule.Dir = "/" // Git matches it anywhere: the least specific rule
			continue
		}
		// "dir/", "dir/**" and "dir/**/" cover dir; other patterns are no folder.
		folder := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(pattern, "/"), "**"), "/")
		if folder != pattern && !strings.ContainsAny(folder, "*?[") {
			if norm, err := paths.Normalize(folder, e.Home); err == nil {
				rule.Dir = norm
			}
		}
	}
	return rule, true
}

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
