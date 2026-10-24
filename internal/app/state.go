package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/provider"
	"github.com/weikxs/gitpersona/internal/textdiff"
)

// StateReport is everything the configuration page shows. Building it never
// fetches a token: accounts come from a token-free gh status query.
type StateReport struct {
	ConfigPath    string             `json:"config_path"`
	ConfigExists  bool               `json:"config_exists"`
	ConfigError   string             `json:"config_error,omitempty"`
	Config        ConfigView         `json:"config"`
	Accounts      []provider.Account `json:"accounts"`
	AccountsError string             `json:"accounts_error,omitempty"`
	Shells        ShellsReport       `json:"shells"`
	Doctor        DoctorReport       `json:"doctor"`
}

// ConfigView is the configuration as the page edits it.
type ConfigView struct {
	Default  string        `json:"default"`
	Profiles []ProfileView `json:"profiles"`
	Pins     []ListedPin   `json:"pins"`
}

// ProfileView is one profile on the page.
type ProfileView struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider"`
	Host     string         `json:"host"`
	Account  string         `json:"account"`
	Name     string         `json:"name"`
	Email    string         `json:"email"`
	SSHKey   string         `json:"ssh_key"`
	Signing  config.Signing `json:"signing"`
	Dirs     []string       `json:"dirs"`
}

// State gathers the page's state.
func State(ctx context.Context, e Env) StateReport {
	report := StateReport{
		ConfigPath: e.display(e.configFile()),
		Config:     ConfigView{Profiles: []ProfileView{}, Pins: []ListedPin{}},
		Accounts:   []provider.Account{},
		Shells:     e.shellsReport(ctx),
		Doctor:     e.tildeReport(doctorReport(e.runChecks(ctx))),
	}
	cfg, err := config.Load(e.configFile())
	switch {
	case errors.Is(err, config.ErrNotFound):
	case err != nil:
		report.ConfigExists, report.ConfigError = true, e.withTilde(err.Error())
	default:
		report.ConfigExists = true
		report.Config = configView(cfg)
		if verr := cfg.Validate(e.Home, e.CaseInsensitive); verr != nil {
			report.ConfigError = e.withTilde(verr.Error())
		}
	}
	if gh, err := e.LookPath("gh"); err == nil {
		accounts, err := provider.GitHub{GH: gh, Run: e.Run}.Accounts(ctx, config.DefaultGitHubHost)
		if err != nil {
			report.AccountsError = err.Error()
		} else if accounts != nil {
			report.Accounts = accounts
		}
	} else {
		report.AccountsError = e.tr("the GitHub CLI (gh) is not installed")
	}
	return report
}

func configView(cfg config.Config) ConfigView {
	v := ConfigView{Default: cfg.Default, Profiles: []ProfileView{}, Pins: []ListedPin{}}
	for _, p := range cfg.Profiles {
		v.Profiles = append(v.Profiles, ProfileView{ID: p.ID, Provider: p.Provider, Host: p.Host, Account: p.Account,
			Name: p.Name, Email: p.Email, SSHKey: p.SSHKey, Signing: p.Signing, Dirs: append([]string{}, p.Dirs...)})
	}
	for _, p := range cfg.Pins {
		v.Pins = append(v.Pins, ListedPin{Repo: p.Repo, Profile: p.Profile})
	}
	return v
}

// FileChange is one file apply would change, with its diff.
type FileChange struct {
	Path   string `json:"path"`
	Action string `json:"action"` // create, update or remove
	Diff   string `json:"diff"`
}

// Preview returns the changes apply would make, without writing anything.
func Preview(ctx context.Context, e Env) ([]FileChange, error) {
	changes, err := planApply(ctx, e)
	if err != nil {
		return nil, err
	}
	return e.fileChanges(changes), nil
}

// fileChanges describes changes with their diffs.
func (e Env) fileChanges(changes []change) []FileChange {
	out := make([]FileChange, 0, len(changes))
	for _, c := range changes {
		name := e.display(c.path)
		fc := FileChange{Path: name, Action: c.verb()}
		switch {
		case c.linkTo != "":
			fc.Diff = e.tr("link to %s", e.display(c.linkTo))
		default:
			before := ""
			if info, err := os.Lstat(c.path); err == nil && info.Mode()&os.ModeSymlink == 0 {
				before, _ = readOptional(c.path) // never read through a link (it may point at a program)
			}
			after := c.content
			if c.remove {
				after = ""
			}
			fc.Diff = e.withTilde(textdiff.Unified(before, after, name))
		}
		out = append(out, fc)
	}
	return out
}

// WithTilde is withTilde for text the page shows, such as error messages.
func (e Env) WithTilde(text string) string { return e.withTilde(text) }

func (e Env) tildeReport(r DoctorReport) DoctorReport {
	checks := make([]DoctorCheck, len(r.Checks))
	for i, c := range r.Checks {
		checks[i] = DoctorCheck{Status: c.Status, Label: e.withTilde(c.Label), Advice: e.withTilde(c.Advice), Fix: c.Fix}
	}
	return DoctorReport{OK: r.OK, Checks: checks}
}

// withTilde writes the home directory as ~ in text shown on the page, so
// diffs never show the user's absolute home path (it often holds their name).
// The files themselves keep absolute paths, which Git needs.
func (e Env) withTilde(text string) string {
	sep := string(filepath.Separator)
	homes := []string{e.Home}
	if realHome, err := filepath.EvalSymlinks(e.Home); err == nil && realHome != e.Home {
		homes = append([]string{realHome}, homes...)
	}
	for _, home := range homes {
		text = strings.ReplaceAll(text, home+sep, "~"+sep)
	}
	return text
}
