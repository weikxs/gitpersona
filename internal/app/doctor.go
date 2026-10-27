package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
)

const statusCheckTimeout = 15 * time.Second

// check is the outcome of one doctor check. A warning is shown but does not
// make doctor fail.
type check struct {
	ok     bool
	warn   bool
	label  string
	advice string
	// fix, when set, repairs the problem; fixes with the same key run once.
	fix    func(context.Context, Env) error
	fixKey string
}

// DoctorOptions controls Doctor.
type DoctorOptions struct {
	Fix  bool // repair what can be repaired, then check again
	JSON bool // print a DoctorReport instead of text
}

// DoctorReport is the stable --json output of `gitpersona doctor`.
type DoctorReport struct {
	OK     bool          `json:"ok"`
	Checks []DoctorCheck `json:"checks"`
}

// DoctorCheck is one check in DoctorReport; Status is ok, warn or fail.
type DoctorCheck struct {
	Status string `json:"status"`
	Label  string `json:"label"`
	Advice string `json:"advice,omitempty"`
	// Fix names the repair the page can run for this check, if any.
	Fix string `json:"fix,omitempty"`
}

// Doctor runs every check, prints the results and reports whether all passed.
// It never fetches tokens: sign-in is checked with a token-free status query.
func Doctor(ctx context.Context, e Env) bool {
	return DoctorWith(ctx, e, DoctorOptions{})
}

// DoctorWith runs Doctor with options.
func DoctorWith(ctx context.Context, e Env, opts DoctorOptions) bool {
	checks := e.runChecks(ctx)
	if opts.Fix {
		fixer := e
		if opts.JSON {
			fixer.Stdout = e.Stderr // keep stdout for the JSON report
		}
		if fixer.fixAll(ctx, checks) {
			checks = e.runChecks(ctx)
		}
	}
	if opts.JSON {
		return e.printDoctorJSON(checks)
	}
	return e.printDoctor(checks)
}

// fixAll runs each distinct fix of a failing or warning check and reports
// whether it attempted any.
func (e Env) fixAll(ctx context.Context, checks []check) bool {
	done := map[string]bool{}
	for _, c := range checks {
		if (c.ok && !c.warn) || c.fix == nil || done[c.fixKey] {
			continue
		}
		done[c.fixKey] = true
		fmt.Fprintln(e.Stdout, e.tr("Fixing: %s", c.label))
		if err := c.fix(ctx, e); err != nil {
			fmt.Fprintln(e.Stdout, "    "+e.tr("could not fix it: %v", err))
		}
	}
	if len(done) > 0 {
		fmt.Fprintln(e.Stdout)
	}
	return len(done) > 0
}

func doctorReport(checks []check) DoctorReport {
	report := DoctorReport{OK: true, Checks: make([]DoctorCheck, len(checks))}
	for i, c := range checks {
		status := "ok"
		switch {
		case c.warn:
			status = "warn"
		case !c.ok:
			status, report.OK = "fail", false
		}
		report.Checks[i] = DoctorCheck{Status: status, Label: c.label, Advice: c.advice}
		if status != "ok" && c.fixKey == FixOldRules {
			report.Checks[i].Fix = c.fixKey
		}
	}
	return report
}

func (e Env) printDoctorJSON(checks []check) bool {
	report := doctorReport(checks)
	enc := json.NewEncoder(e.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
	return report.OK
}

func (e Env) runChecks(ctx context.Context) []check {
	checks := []check{e.checkGitVersion(ctx)}
	cfg, cfgCheck := e.checkConfig()
	checks = append(checks, cfgCheck)
	if cfgCheck.ok {
		checks = append(checks, e.checkGenerated(ctx))
		checks = append(checks, e.checkBlockIsLast())
		checks = append(checks, e.checkCurrentRepository(ctx, cfg)...)
		checks = append(checks, e.checkPins(cfg)...)
		checks = append(checks, e.checkKeyFiles(cfg)...)
		checks = append(checks, e.checkSigning(ctx, cfg)...)
		checks = append(checks, e.checkDirs(cfg)...)
		checks = append(checks, e.checkOldRules(ctx, cfg)...)
		checks = append(checks, e.checkShellIntegration(ctx, cfg)...)
		checks = append(checks, e.checkGHRouting(ctx)...)
		checks = append(checks, e.checkGitHubAccounts(ctx, cfg)...)
		checks = append(checks, e.checkHelpers(ctx, cfg)...)
	}

	return checks
}

func (e Env) printDoctor(checks []check) bool {
	allOK := true
	for _, c := range checks {
		mark := "✓"
		switch {
		case c.warn:
			mark = "!"
		case !c.ok:
			mark, allOK = "✗", false
		}
		fmt.Fprintf(e.Stdout, "%s %s\n", mark, c.label)
		if (c.warn || !c.ok) && c.advice != "" {
			fmt.Fprintf(e.Stdout, "    → %s\n", c.advice)
		}
	}
	return allOK
}

func (e Env) checkConfig() (config.Config, check) {
	cfg, err := e.loadConfig()
	if err == nil {
		err = cfg.Validate(e.Home, e.CaseInsensitive)
	}
	if errors.Is(err, config.ErrNotFound) {
		return cfg, check{label: e.tr("no configuration yet (%s)", e.display(e.configFile())),
			advice: e.tr("run `gitpersona setup` to create it from what you already have")}
	}
	if err != nil {
		return cfg, check{label: e.tr("configuration: %v", err), advice: e.tr("fix %s", e.display(e.configFile()))}
	}
	profiles := e.tr("%d profiles", len(cfg.Profiles))
	if len(cfg.Profiles) == 1 {
		profiles = e.tr("1 profile")
	}
	pins := e.tr("%d pins", len(cfg.Pins))
	if len(cfg.Pins) == 1 {
		pins = e.tr("1 pin")
	}
	return cfg, check{ok: true, label: e.tr("configuration is valid (%s, %s)", profiles, pins)}
}

func (e Env) differ(n int) string {
	if n == 1 {
		return e.tr("1 file differs")
	}
	return e.tr("%d files differ", n)
}

func (e Env) checkGenerated(ctx context.Context) check {
	changes, err := planApply(ctx, e)
	switch {
	case err != nil:
		return check{label: e.tr("generated Git configuration: %v", err)}
	case len(changes) > 0:
		names := make([]string, len(changes))
		for i, c := range changes {
			names[i] = e.display(c.path)
		}
		return check{
			fix: applyFix, fixKey: "apply",
			label: e.tr("generated Git configuration is out of date (%s: %s)",
				e.differ(len(changes)), strings.Join(names, ", ")),
			advice: e.tr("run `gitpersona apply` (this also happens after gitpersona is moved or upgraded, " +
				"or the configuration or your global Git settings change)"),
		}
	default:
		return check{ok: true, label: e.tr("generated Git configuration is up to date")}
	}
}

// applyFix regenerates the files. It never removes files (that needs
// `gitpersona apply --yes`) and never prunes backups.
func applyFix(ctx context.Context, e Env) error {
	return Apply(ctx, e, ApplyOptions{SkipRemovals: true, NoPrune: true})
}

// checkPins warns about pins whose repository does not exist (yet): they are
// harmless, but often a typo or a repository that moved.
func (e Env) checkPins(cfg config.Config) []check {
	var checks []check
	for _, pin := range cfg.Pins {
		norm, err := paths.Normalize(pin.Repo, e.Home)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(norm, ".git")); err != nil {
			checks = append(checks, check{warn: true,
				label:  e.tr("pin %s: no Git repository there yet", pin.Repo),
				advice: e.tr("clone it there, or remove the pin with `gitpersona unpin` inside the repository")})
		}
	}
	return checks
}

// checkCurrentRepository explains, when doctor runs inside a repository, why
// Git there does not use the expected profile.
func (e Env) checkCurrentRepository(ctx context.Context, cfg config.Config) []check {
	r := e.whoami(ctx, cfg)
	if !r.InRepository || !r.Found {
		return nil
	}
	if !r.Consistent {
		return []check{{label: e.tr("this repository does not use profile %s (%s)", r.Profile, r.Account),
			advice: e.mismatchAdvice(r)}}
	}
	return []check{{ok: true, label: e.tr("this repository uses profile %s (%s)", r.Profile, r.Account)}}
}

// checkBlockIsLast flags settings after the GitPersona block in the global
// Git configuration: Git applies later settings last, so they override every
// profile (for example a user.email added with `git config --global`, or the
// helper `gh auth setup-git` adds).
func (e Env) checkBlockIsLast() check {
	content, err := readOptional(e.resolvedUserGitConfig())
	if err != nil {
		return check{label: e.tr("%v", err)}
	}
	if block.ContentAfter(content) {
		return check{
			fix: applyFix, fixKey: "apply",
			label:  e.tr("%s has settings after the GitPersona block, which override every profile", e.display(e.userGitConfig())),
			advice: e.tr("run `gitpersona apply` to move the block to the end (your settings are kept)"),
		}
	}
	return check{ok: true, label: e.tr("the GitPersona block is the last part of %s", e.display(e.userGitConfig()))}
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
	gh, err := e.GHPath()
	if err != nil {
		return []check{{label: e.tr("GitHub CLI (gh) not found"), advice: e.tr("install it from https://cli.github.com and sign in with `gh auth login`")}}
	}

	source := provider.GitHub{GH: gh, Run: e.Run}
	checks := []check{{ok: true, label: e.tr("GitHub CLI found at %s", e.display(gh))}}
	signedIn := map[string][]string{} // host -> accounts, queried once per host
	unknownHost := map[string]bool{}  // hosts gh could not be asked about
	for _, p := range profiles {
		accounts, queried := signedIn[p.Host]
		if !queried {
			sctx, cancel := context.WithTimeout(ctx, statusCheckTimeout)
			var err error
			accounts, err = source.SignedInAccounts(sctx, p.Host)
			cancel()
			if err != nil {
				checks = append(checks, check{label: e.tr("could not ask gh which accounts are signed in on %s", p.Host),
					advice: e.tr("run `gh auth status` to see what is wrong (GitPersona needs gh 2.40 or newer)")})
			}
			signedIn[p.Host], unknownHost[p.Host] = accounts, err != nil
		}
		if unknownHost[p.Host] {
			continue // already reported; do not guess whether the account is signed in
		}
		if !slices.Contains(accounts, p.Account) {
			checks = append(checks, check{
				label:  e.tr("profile %s: %s is not signed in to gh on %s", p.ID, p.Account, p.Host),
				advice: e.tr("run `gh auth login --hostname %s` and sign in as %s", p.Host, p.Account),
			})
			continue
		}
		checks = append(checks, check{ok: true, label: e.tr("profile %s: %s is signed in to gh on %s", p.ID, p.Account, p.Host)})
	}
	return checks
}
