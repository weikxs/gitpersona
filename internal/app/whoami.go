package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/resolve"
)

// WhoamiReport is the stable --json output of `gitpersona whoami`.
type WhoamiReport struct {
	Found          bool   `json:"found"`
	Profile        string `json:"profile,omitempty"`
	Account        string `json:"account,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Host           string `json:"host,omitempty"`
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	Reason         string `json:"reason"`
	Match          string `json:"match,omitempty"`
	Path           string `json:"path"`
	InRepository   bool   `json:"in_repository"`
	EffectiveEmail string `json:"effective_email,omitempty"`
	Consistent     bool   `json:"consistent"`
	// EnvOverride names an environment variable that sets the author email
	// for commits made here, overriding every configuration file.
	EnvOverride string `json:"env_override,omitempty"`
	// LocalEmail is a repository-local user.email, which also overrides profiles.
	LocalEmail string `json:"local_email,omitempty"`
	// OwnRule is the file of the user's own includeIf rule that sets the
	// author here; GitPersona keeps such rules working in their folders.
	OwnRule string `json:"own_rule,omitempty"`
	// NewRepositories, outside a repository, is the profile that repositories
	// created or cloned here will use.
	NewRepositories *NewRepositories `json:"new_repositories,omitempty"`
}

// NewRepositories describes the profile future repositories in a directory get.
type NewRepositories struct {
	Profile string `json:"profile"`
	Account string `json:"account"`
	Reason  string `json:"reason"`
	Match   string `json:"match,omitempty"`
}

// WhoamiOptions controls Whoami.
type WhoamiOptions struct {
	JSON  bool // print a WhoamiReport
	Check bool // fail when Git does not use the profile
}

// Whoami reports which profile applies to the current directory and whether
// Git currently agrees.
func Whoami(ctx context.Context, e Env, asJSON bool) error {
	return WhoamiWith(ctx, e, WhoamiOptions{JSON: asJSON})
}

// WhoamiWith runs Whoami with options.
func WhoamiWith(ctx context.Context, e Env, opts WhoamiOptions) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	report := e.whoami(ctx, cfg)
	if opts.JSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printWhoami(e, report)
	}
	if opts.Check && !report.Consistent {
		return fmt.Errorf("this repository does not use profile %s: %s", report.Profile, mismatchAdvice(report))
	}
	return nil
}

func (e Env) whoami(ctx context.Context, cfg config.Config) WhoamiReport {
	opts := resolve.Options{Home: e.Home, CaseInsensitive: e.CaseInsensitive, Canonicalize: paths.ResolveSymlinks}
	// Bare repositories and the inside of .git have a Git directory but no
	// work tree, so the Git directory decides whether we are in a repository.
	gitDir, err := e.git(ctx, e.WorkDir, "rev-parse", "--absolute-git-dir")
	inRepo := err == nil && gitDir != ""
	root, hasWorkTree := e.repoRoot(ctx, e.WorkDir)
	if !hasWorkTree {
		root = gitDir
	}

	var r resolve.Result
	report := WhoamiReport{InRepository: inRepo}
	if inRepo {
		report.Path = paths.ResolveSymlinks(root)
		r = resolve.GitDir(cfg, paths.ResolveSymlinks(gitDir), opts)
		report.EffectiveEmail, _ = e.git(ctx, root, "config", "user.email")
	} else {
		report.Path = paths.ResolveSymlinks(e.WorkDir)
		r = resolve.OutsideRepository(cfg)
		if future := resolve.Resolve(cfg, report.Path, opts); future.Found {
			report.NewRepositories = &NewRepositories{Profile: future.Profile.ID, Account: future.Profile.Account,
				Reason: string(future.Reason), Match: future.Match}
		}
	}

	report.Found, report.Reason, report.Match = r.Found, string(r.Reason), r.Match
	if r.Found {
		p := r.Profile
		report.Profile, report.Account, report.Provider, report.Host = p.ID, p.Account, p.Provider, p.Host
		report.Name, report.Email = p.Name, p.Email
	}
	if inRepo {
		report.LocalEmail, _ = e.git(ctx, root, "config", "--local", "user.email")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		if v := os.Getenv(name); v != "" && v != report.Email {
			report.EnvOverride = name
			break
		}
	}
	if inRepo && r.Found && report.EffectiveEmail != report.Email && report.LocalEmail == "" {
		report.OwnRule = e.ownRule(ctx, cfg, root)
	}
	report.Consistent = report.EnvOverride == "" &&
		(!inRepo || !r.Found || report.EffectiveEmail == report.Email || report.OwnRule != "")
	return report
}

// ownRule returns the file of the user's own includeIf rule that sets
// user.email in the repository at root, or "" when the email comes from
// anywhere else (GitPersona's files, the repository, the global settings).
func (e Env) ownRule(ctx context.Context, cfg config.Config, root string) string {
	out, err := e.git(ctx, root, "config", "--show-origin", "--get", "user.email")
	origin, _, ok := strings.Cut(out, "\t")
	file, isFile := strings.CutPrefix(origin, "file:")
	if err != nil || !ok || !isFile {
		return ""
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, file)
	}
	text, err := readOptional(e.resolvedUserGitConfig())
	if err != nil {
		return ""
	}
	rules, _ := e.userRules(ctx, cfg, text)
	for _, r := range rules {
		if norm, err := paths.Normalize(r.Path, e.Home); err == nil && paths.ResolveSymlinks(norm) == paths.ResolveSymlinks(file) {
			return e.display(file)
		}
	}
	return ""
}

// mismatchAdvice explains why Git does not use the profile and how to fix it.
func mismatchAdvice(r WhoamiReport) string {
	switch {
	case r.EnvOverride != "":
		return fmt.Sprintf("the environment variable %s overrides the profile; unset it (for example in your direnv or shell setup)", r.EnvOverride)
	case r.LocalEmail != "":
		return fmt.Sprintf("this repository sets user.email %q itself; remove it with `git config --local --unset user.email`", r.LocalEmail)
	default:
		return fmt.Sprintf("Git currently uses %q; run `gitpersona doctor`", r.EffectiveEmail)
	}
}

func printWhoami(e Env, r WhoamiReport) {
	if !r.InRepository {
		fmt.Fprintf(e.Stdout, "Not inside a Git repository (%s).\n", e.display(r.Path))
	}
	if !r.Found {
		fmt.Fprintln(e.Stdout, "No GitPersona profile applies here; Git uses its own settings.")
	} else {
		printProfile(e, r)
	}
	if n := r.NewRepositories; n != nil {
		reason := resolve.Reason(n.Reason).Describe()
		if n.Match != "" {
			reason += " " + n.Match
		}
		fmt.Fprintf(e.Stdout, "Repositories created or cloned here will use %s (profile %s, %s).\n", n.Account, n.Profile, reason)
	}
}

func printProfile(e Env, r WhoamiReport) {
	fmt.Fprintf(e.Stdout, "%s  (profile %s · %s on %s)\n", r.Account, r.Profile, r.Provider, r.Host)
	fmt.Fprintf(e.Stdout, "  author  %s <%s>\n", r.Name, r.Email)
	reason := resolve.Reason(r.Reason).Describe()
	if r.Match != "" {
		reason += " " + r.Match
	}
	fmt.Fprintf(e.Stdout, "  reason  %s\n", reason)
	fmt.Fprintf(e.Stdout, "  path    %s\n", e.display(r.Path))
	if r.OwnRule != "" {
		fmt.Fprintf(e.Stdout, "  note    Git uses %s here, from your own includeIf rule (%s), which GitPersona keeps working\n",
			r.EffectiveEmail, r.OwnRule)
	}
	if !r.Consistent {
		fmt.Fprintf(e.Stdout, "  warning %s\n", mismatchAdvice(r))
	}
}

// WhoamiReportFor returns the whoami report for e.WorkDir.
func WhoamiReportFor(ctx context.Context, e Env) (WhoamiReport, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		return WhoamiReport{}, err
	}
	return e.whoami(ctx, cfg), nil
}
