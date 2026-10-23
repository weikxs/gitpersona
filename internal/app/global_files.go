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

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
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

// resets returns the profiles that must set their signing and their SSH
// command because a rule of the user's sets them where the profile applies:
// around one of its directories or pins, anywhere (onbranch, hasconfig, the
// whole home), or anywhere at all for the default. text is the global
// file's content as it will be; rules in it that are gone do not count.
func (e Env) resets(ctx context.Context, cfg config.Config, text string) (signing, ssh map[string]bool) {
	signing, ssh = map[string]bool{}, map[string]bool{}
	present := map[string]bool{}
	for _, s := range includeIfSections(text) {
		present[s.condition] = true
	}
	main := e.resolvedUserGitConfig()
	for _, file := range e.globalFiles() {
		for _, r := range discover.RulesSettings(ctx, e.Home, file, e.Run) {
			if file == main && !present[r.Condition] {
				continue
			}
			setsSigning := slices.ContainsFunc(signingKeys, func(k string) bool { return r.Keys[k] })
			if !setsSigning && !r.Keys["core.sshcommand"] {
				continue
			}
			for _, p := range cfg.Profiles {
				if e.ruleReaches(r.Dir, p, cfg) {
					signing[p.ID] = signing[p.ID] || setsSigning
					ssh[p.ID] = ssh[p.ID] || r.Keys["core.sshcommand"]
				}
			}
		}
	}
	return signing, ssh
}

// ruleReaches reports whether a rule of the user's for dir ("" or "~": any
// repository) can apply in a repository where profile p applies too.
func (e Env) ruleReaches(dir string, p config.Profile, cfg config.Config) bool {
	if dir == "" || dir == "~" || p.ID == cfg.Default {
		return true
	}
	places := slices.Clone(p.Dirs)
	for _, pin := range cfg.Pins {
		if pin.Profile == p.ID {
			places = append(places, pin.Repo)
		}
	}
	// Around the profile's folder, or inside it: either way the profile comes
	// after the rule there.
	return slices.ContainsFunc(places, func(place string) bool {
		return coversWith(e.comparisonKey, []string{dir}, place) || coversWith(e.comparisonKey, []string{place}, dir)
	})
}
