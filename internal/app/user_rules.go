package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/paths"
)

// userRules reads the includeIf rules of the user's global Git
// configuration (text, the file's content), so GitPersona can include them
// again among its own rules: they keep working in their folders, and only a
// more specific GitPersona rule or a pin replaces them. A rule for a
// directory that cfg gives to a profile with another identity yields to
// that profile. Rules that cannot be included again are returned in
// skipped, with the reason; GitPersona's rules override them.
func (e Env) userRules(ctx context.Context, cfg config.Config, text string) (rules []generate.UserRule, skipped []skippedRule) {
	identities := map[string]discover.Rule{} // condition -> the identity it sets
	for _, r := range discover.IdentityRules(ctx, e.Home, e.resolvedUserGitConfig(), e.Run) {
		identities[r.Condition] = r
	}
	owners := ownersOf(cfg)
	for _, found := range includeIfPaths(text) {
		rule, ok := e.userRule(found.condition, found.path)
		if !ok {
			continue
		}
		if why := e.cannotIncludeAgain(ctx, rule.Path); why != "" {
			skipped = append(skipped, skippedRule{Condition: found.condition, Why: why})
			continue
		}
		if id, known := identities[found.condition]; known && rule.Dir != "" {
			rule.Yield = e.givenToAnother(rule.Dir, id, owners)
		}
		rules = append(rules, rule)
	}
	return rules, skipped
}

// skippedRule is a rule of the user's that GitPersona does not include
// again, and why.
type skippedRule struct {
	Condition string
	Why       string
}

type includeIfPath struct{ condition, path string }

// includeIfPaths returns the path settings of the includeIf sections of a
// Git configuration's text, in order.
func includeIfPaths(text string) []includeIfPath {
	var out []includeIfPath
	condition := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			condition = ""
			m := includeIfHeader.FindStringSubmatch(trimmed)
			if m == nil {
				continue
			}
			condition = unescapeSubsection(m[1])
			trimmed = strings.TrimSpace(trimmed[len(m[0]):]) // a setting may follow the header
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if condition != "" && ok && strings.EqualFold(strings.TrimSpace(key), "path") {
			out = append(out, includeIfPath{condition, gitValue(value)})
		}
	}
	return out
}

// listKeys are settings Git collects as lists: included a second time, the
// same values would be added twice (two credential helpers, a header sent
// twice), so rules that set them are not included again.
var listKeys = []string{".helper", ".extraheader", ".fetch", ".push", ".pushurl"}

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
// path is relative to the global configuration as Git names it (not
// following a symlink), while a ./ pattern is relative to its real
// directory, as Git does.
func (e Env) userRule(condition, path string) (generate.UserRule, bool) {
	if path == "" {
		return generate.UserRule{}, false
	}
	if !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "%(prefix)/") && !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(e.userGitConfig()), path)
	}
	rule := generate.UserRule{Condition: condition, Path: path}
	for _, prefix := range []string{"gitdir:", "gitdir/i:"} {
		pattern, ok := strings.CutPrefix(condition, prefix)
		if !ok {
			continue
		}
		if rest, ok := strings.CutPrefix(pattern, "./"); ok {
			pattern = filepath.Dir(e.resolvedUserGitConfig()) + "/" + rest
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
