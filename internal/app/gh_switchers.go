package app

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/weikxs/gitpersona/internal/shell"
)

// ghFunction finds a gh function or alias defined in a shell startup file.
var ghFunction = regexp.MustCompile(`(?m)^\s*(function\s+gh(\s|;|\(|$)|gh\s*\(\s*\)|alias\s+gh[\s=])`)

// ghTokenExport finds GH_TOKEN exported to the shell, as a hook on changing
// directory does (fish `set -gx`, sh `export`).
var ghTokenExport = regexp.MustCompile(`(?m)(\bset\s+-[A-Za-z]*x[A-Za-z]*\s+GH_TOKEN\b|\bexport\s+GH_TOKEN=)`)

// ghSwitchers returns the shell startup files that already switch gh
// accounts: they define gh and hand it a token or an account, or export
// GH_TOKEN from `gh auth token`. GitPersona's integration does the same, so
// the user can keep one.
func (e Env) ghSwitchers() []string {
	var found []string
	// GitPersona's own lines only load it, so they never match.
	for _, k := range []shell.Kind{shell.Fish, shell.Zsh, shell.Bash} {
		files := e.startupFiles(k)
		if k == shell.Fish {
			files = append(files, filepath.Join(e.fishConfigDir(), "functions", "gh.fish")) // where funcsave writes
		}
		for _, path := range files {
			content, err := readOptional(path)
			if err != nil {
				continue
			}
			wraps := ghFunction.MatchString(content) && (strings.Contains(content, "GH_TOKEN") || strings.Contains(content, "gh auth"))
			exports := ghTokenExport.MatchString(content) && strings.Contains(content, "gh auth token")
			if wraps || exports {
				found = append(found, e.display(path))
			}
		}
	}
	return found
}

// ghSwitcherNote says that gh is already switched by the user's own setup.
func (e Env) ghSwitcherNote(files []string) string {
	return e.tr("you already switch gh accounts in %s, and GitPersona's integration switches gh by profile too. "+
		"GitPersona leaves gh alone wherever GH_TOKEN is set, so where your setup sets it, yours decides; "+
		"keeping one of the two avoids surprises", e.setupList(files))
}
