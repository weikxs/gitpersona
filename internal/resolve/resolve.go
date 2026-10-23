// Package resolve decides which profile applies to a repository.
//
// The order is: a pin for the exact repository, then the most specific
// directory rule containing it, then the default profile. When none applies,
// GitPersona leaves Git's own behavior untouched.
//
// The generated Git configuration encodes the same order, so this package is
// what `whoami` and `doctor` use to explain what Git will do.
package resolve

import (
	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
)

// Reason explains why a profile was chosen.
type Reason string

// Reasons, from highest to lowest precedence.
const (
	ReasonPin     Reason = "pin"
	ReasonRule    Reason = "rule"
	ReasonDefault Reason = "default"
	ReasonNone    Reason = "none"
)

// Describe returns a short human-readable form of the reason.
func (r Reason) Describe() string {
	switch r {
	case ReasonPin:
		return "pinned repository"
	case ReasonRule:
		return "directory rule"
	case ReasonDefault:
		return "default profile"
	default:
		return "no rule matches"
	}
}

// Options controls path handling.
type Options struct {
	// Home expands "~" in configured paths.
	Home string
	// CaseInsensitive compares paths without regard to case.
	CaseInsensitive bool
	// Canonicalize, if set, is applied to every path before comparison,
	// typically to resolve symbolic links.
	Canonicalize func(string) string
}

// Result is the outcome of resolving one repository.
type Result struct {
	Found   bool
	Profile config.Profile
	Reason  Reason
	// Match is the pin or directory as written in the configuration.
	Match string
}

// Resolve returns the profile for the repository whose root directory is
// repoRoot, an absolute path. It never fails: invalid configuration entries
// are skipped, because Validate reports them.
func Resolve(cfg config.Config, repoRoot string, opts Options) Result {
	repo := opts.canonical(repoRoot)

	if r, ok := matchPin(cfg, repo, opts); ok {
		return r
	}
	if r, ok := matchRule(cfg, repo, opts); ok {
		return r
	}
	if p, ok := cfg.ProfileByID(cfg.Default); ok {
		return Result{Found: true, Profile: p, Reason: ReasonDefault}
	}
	return Result{Reason: ReasonNone}
}

func matchPin(cfg config.Config, repo string, opts Options) (Result, bool) {
	for _, pin := range cfg.Pins {
		norm, ok := opts.normalize(pin.Repo)
		if !ok || !paths.Same(repo, norm, opts.CaseInsensitive) {
			continue
		}
		if p, found := cfg.ProfileByID(pin.Profile); found {
			return Result{Found: true, Profile: p, Reason: ReasonPin, Match: pin.Repo}, true
		}
	}
	return Result{}, false
}

func matchRule(cfg config.Config, repo string, opts Options) (Result, bool) {
	best, bestLen := Result{}, -1
	for _, p := range cfg.Profiles {
		for _, d := range p.Dirs {
			norm, ok := opts.normalize(d)
			if !ok || !paths.Within(repo, norm, opts.CaseInsensitive) || len(norm) <= bestLen {
				continue
			}
			best, bestLen = Result{Found: true, Profile: p, Reason: ReasonRule, Match: d}, len(norm)
		}
	}
	return best, bestLen >= 0
}

func (o Options) normalize(p string) (string, bool) {
	norm, err := paths.Normalize(p, o.Home)
	if err != nil {
		return "", false
	}
	return o.canonical(norm), true
}

func (o Options) canonical(p string) string {
	if o.Canonicalize == nil {
		return p
	}
	return o.Canonicalize(p)
}

// Git directory layout used by includeIf "gitdir:" matching.
const (
	dotGit       = "/.git"
	worktreesDir = "/.git/worktrees"
)

// GitDir returns the profile Git itself applies to the repository
// whose Git directory is gitDir, mirroring the generated includeIf rules:
// directory rules match when the Git directory lies inside the rule
// directory, so a worktree follows its main repository; pins match the
// pinned repository's .git directory and its worktrees.
func GitDir(cfg config.Config, gitDir string, opts Options) Result {
	dir := opts.canonical(gitDir)

	for _, pin := range cfg.Pins {
		norm, ok := opts.normalize(pin.Repo)
		if !ok {
			continue
		}
		if paths.Same(dir, norm+dotGit, opts.CaseInsensitive) || paths.Within(dir, norm+worktreesDir, opts.CaseInsensitive) {
			if p, found := cfg.ProfileByID(pin.Profile); found {
				return Result{Found: true, Profile: p, Reason: ReasonPin, Match: pin.Repo}
			}
		}
	}
	if r, ok := matchRule(cfg, dir, opts); ok {
		return r
	}
	return OutsideRepository(cfg)
}

// OutsideRepository returns what Git uses outside any repository,
// where includeIf "gitdir:" never applies: only the default profile.
func OutsideRepository(cfg config.Config) Result {
	if p, ok := cfg.ProfileByID(cfg.Default); ok {
		return Result{Found: true, Profile: p, Reason: ReasonDefault}
	}
	return Result{Reason: ReasonNone}
}
