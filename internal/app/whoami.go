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
	path, inRepo := e.repoRoot(ctx, e.WorkDir)
	if !inRepo {
		path = e.WorkDir
	}
	r := resolve.Resolve(cfg, paths.ResolveSymlinks(path), resolve.Options{
		Home:            e.Home,
		CaseInsensitive: e.CaseInsensitive,
		Canonicalize:    paths.ResolveSymlinks,
	})

	report := WhoamiReport{Found: r.Found, Reason: string(r.Reason), Match: r.Match, Path: path, InRepository: inRepo}
	if r.Found {
		p := r.Profile
		report.Profile, report.Account, report.Provider, report.Host = p.ID, p.Account, p.Provider, p.Host
		report.Name, report.Email = p.Name, p.Email
	}
	if inRepo {
		report.EffectiveEmail, _ = e.git(ctx, path, "config", "user.email")
	}
	report.Consistent = !inRepo || !r.Found || report.EffectiveEmail == report.Email
	return report
}

func printWhoami(e Env, r WhoamiReport) {
	if !r.Found {
		fmt.Fprintln(e.Stdout, "No GitPersona profile applies here; Git uses its own settings.")
		return
	}
	fmt.Fprintf(e.Stdout, "%s  (profile %s · %s on %s)\n", r.Account, r.Profile, r.Provider, r.Host)
	fmt.Fprintf(e.Stdout, "  author  %s <%s>\n", r.Name, r.Email)
	reason := resolve.Reason(r.Reason).Describe()
	if r.Match != "" {
		reason += " " + r.Match
	}
	fmt.Fprintf(e.Stdout, "  reason  %s\n", reason)
	fmt.Fprintf(e.Stdout, "  path    %s\n", e.display(r.Path))
	if !r.Consistent {
		fmt.Fprintf(e.Stdout, "  warning Git currently uses %q. Run `gitpersona apply`, or remove a repository-local user.email.\n", r.EffectiveEmail)
	}
}
