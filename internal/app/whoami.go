package app

import (
	"context"
	"encoding/json"
	"fmt"

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

// Whoami reports which profile applies to the current directory and whether
// Git currently agrees.
func Whoami(ctx context.Context, e Env, asJSON bool) error {
	cfg, err := config.Load(e.configFile())
	if err != nil {
		return err
	}
	report := e.whoami(ctx, cfg)
	if asJSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printWhoami(e, report)
	return nil
}

func (e Env) whoami(ctx context.Context, cfg config.Config) WhoamiReport {
	opts := resolve.Options{Home: e.Home, CaseInsensitive: e.CaseInsensitive, Canonicalize: paths.ResolveSymlinks}
	root, inRepo := e.repoRoot(ctx, e.WorkDir)
	gitDir, _ := e.git(ctx, e.WorkDir, "rev-parse", "--absolute-git-dir")

	var r resolve.Result
	report := WhoamiReport{InRepository: inRepo}
	if inRepo {
		report.Path = paths.ResolveSymlinks(root)
		r = resolve.ResolveGitDir(cfg, paths.ResolveSymlinks(gitDir), opts)
		report.EffectiveEmail, _ = e.git(ctx, root, "config", "user.email")
	} else {
		report.Path = paths.ResolveSymlinks(e.WorkDir)
		r = resolve.ResolveOutsideRepository(cfg)
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
	report.Consistent = !inRepo || !r.Found || report.EffectiveEmail == report.Email
	return report
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
		fmt.Fprintf(e.Stdout, "Repositories created or cloned here will use %s (profile %s, %s %s).\n",
			n.Account, n.Profile, resolve.Reason(n.Reason).Describe(), n.Match)
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
	if !r.Consistent {
		fmt.Fprintf(e.Stdout, "  warning Git currently uses %q. Run `gitpersona doctor`; a repository-local "+
			"user.email (check `git config --local user.email`) or an outdated setup can cause this.\n", r.EffectiveEmail)
	}
}
