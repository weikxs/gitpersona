package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/provider"
)

const statusCheckTimeout = 15 * time.Second

// check is the outcome of one doctor check.
type check struct {
	ok     bool
	label  string
	advice string
}

// Doctor runs every check, prints the results and reports whether all passed.
// It never fetches tokens: sign-in is checked with a token-free status query.
func Doctor(ctx context.Context, e Env) bool {
	var checks []check
	cfg, cfgCheck := e.checkConfig()
	checks = append(checks, cfgCheck)
	if cfgCheck.ok {
		checks = append(checks, e.checkGenerated(ctx))
		checks = append(checks, e.checkGitHubAccounts(ctx, cfg)...)
	}

	allOK := true
	for _, c := range checks {
		mark := "✓"
		if !c.ok {
			mark, allOK = "✗", false
		}
		fmt.Fprintf(e.Stdout, "%s %s\n", mark, c.label)
		if !c.ok && c.advice != "" {
			fmt.Fprintf(e.Stdout, "    → %s\n", c.advice)
		}
	}
	return allOK
}

func (e Env) checkConfig() (config.Config, check) {
	cfg, err := config.Load(e.configFile())
	if err == nil {
		err = cfg.Validate(e.Home, e.CaseInsensitive)
	}
	if err != nil {
		return cfg, check{label: "configuration: " + err.Error(), advice: "fix " + e.display(e.configFile())}
	}
	return cfg, check{ok: true, label: fmt.Sprintf("configuration is valid (%s, %s)",
		plural(len(cfg.Profiles), "profile"), plural(len(cfg.Pins), "pin"))}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (e Env) checkGenerated(ctx context.Context) check {
	changes, err := planApply(ctx, e)
	switch {
	case err != nil:
		return check{label: "generated Git configuration: " + err.Error()}
	case len(changes) > 0:
		return check{label: fmt.Sprintf("generated Git configuration is out of date (%d files differ)", len(changes)),
			advice: "run `gitpersona apply`"}
	default:
		return check{ok: true, label: "generated Git configuration is up to date"}
	}
}

func (e Env) checkGitHubAccounts(ctx context.Context, cfg config.Config) []check {
	var profiles []config.Profile
	for _, p := range cfg.Profiles {
		if p.Provider == config.ProviderGitHub {
			profiles = append(profiles, p)
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	gh, err := e.LookPath("gh")
	if err != nil {
		return []check{{label: "GitHub CLI (gh) not found", advice: "install it from https://cli.github.com and sign in with `gh auth login`"}}
	}

	source := provider.GitHub{GH: gh, Run: e.Run}
	checks := []check{{ok: true, label: "GitHub CLI found at " + gh}}
	signedIn := map[string][]string{} // host -> accounts, queried once per host
	for _, p := range profiles {
		accounts, queried := signedIn[p.Host]
		if !queried {
			sctx, cancel := context.WithTimeout(ctx, statusCheckTimeout)
			var err error
			accounts, err = source.SignedInAccounts(sctx, p.Host)
			cancel()
			if err != nil {
				checks = append(checks, check{label: fmt.Sprintf("could not ask gh which accounts are signed in on %s", p.Host),
					advice: "run `gh auth status` to see what is wrong"})
				signedIn[p.Host] = nil
				continue
			}
			signedIn[p.Host] = accounts
		}
		if !slices.Contains(accounts, p.Account) {
			checks = append(checks, check{
				label:  fmt.Sprintf("profile %s: %s is not signed in to gh on %s", p.ID, p.Account, p.Host),
				advice: fmt.Sprintf("run `gh auth login --hostname %s` and sign in as %s", p.Host, p.Account),
			})
			continue
		}
		checks = append(checks, check{ok: true, label: fmt.Sprintf("profile %s: %s is signed in to gh on %s", p.ID, p.Account, p.Host)})
	}
	return checks
}
