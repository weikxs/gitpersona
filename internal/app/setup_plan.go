package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/provider"
)

// SetupProfile is a profile the setup page proposes, or one the user confirmed.
type SetupProfile struct {
	ID      string   `json:"id"`
	Account string   `json:"account"`
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Dirs    []string `json:"dirs"`
	// Source is "existing" when name, email and directories come from the
	// user's own includeIf setup, and "gh" when they come from GitHub.
	Source string `json:"source,omitempty"`
	// EmailSource is "public" for the email on the GitHub profile, "noreply"
	// when the account shows none, or "existing".
	EmailSource string `json:"email_source,omitempty"`
}

// SetupProposal is what the setup page starts from.
type SetupProposal struct {
	Default  string         `json:"default"`
	Profiles []SetupProfile `json:"profiles"`
	// Unmatched are identities of an existing setup that no signed-in
	// account claims; the page shows them so nothing is silently dropped.
	Unmatched []discover.Identity `json:"unmatched"`
}

// SetupPlan is the configuration the user confirmed on the setup page.
type SetupPlan struct {
	Default  string         `json:"default"`
	Profiles []SetupProfile `json:"profiles"`
}

// ProposeSetup suggests one profile per account signed in to gh, reusing
// the name, email and directories of a matching existing includeIf setup.
// It reads no tokens: accounts come from a status query and names from the
// public user API.
func ProposeSetup(ctx context.Context, e Env) (SetupProposal, error) {
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	source, accounts, err := e.signedInAccounts(ctx)
	if err != nil {
		return SetupProposal{}, err
	}
	existing, _ := discover.Existing(ctx, e.Home, e.GlobalGitConfigPath(), e.Run)
	used := make([]bool, len(existing))

	proposal := SetupProposal{Profiles: []SetupProfile{}, Unmatched: []discover.Identity{}}
	for _, a := range accounts {
		if a.State != "success" {
			continue
		}
		p := SetupProfile{ID: profileID(a.Login), Account: a.Login, Dirs: []string{}, Source: "gh"}
		gh := ghIdentity(ctx, source, a.Login)
		p.Name, p.Email, p.EmailSource = gh.name, gh.email, gh.emailSource
		if i := matchIdentity(existing, a.Login, gh.publicEmail); i >= 0 {
			used[i] = true
			p.Name, p.Email, p.Source, p.EmailSource = existing[i].Name, existing[i].Email, "existing", "existing"
			p.Dirs = append(p.Dirs, existing[i].Dirs...)
		}
		if a.Active {
			proposal.Default = p.ID
		}
		proposal.Profiles = append(proposal.Profiles, p)
	}
	for i, id := range existing {
		if !used[i] {
			id.Source = e.Display(id.Source)
			proposal.Unmatched = append(proposal.Unmatched, id)
		}
	}
	return proposal, nil
}

func (e Env) signedInAccounts(ctx context.Context) (provider.GitHub, []provider.Account, error) {
	gh, err := e.LookPath("gh")
	if err != nil {
		return provider.GitHub{}, nil, errors.New("setup needs the GitHub CLI: install it from https://cli.github.com and run `gh auth login` for each account")
	}
	source := provider.GitHub{GH: gh, Run: e.Run}
	accounts, err := source.Accounts(ctx, config.DefaultGitHubHost)
	if err != nil || len(accounts) == 0 {
		return source, nil, errors.New("no GitHub account is signed in to gh; run `gh auth login` for each account, then try again")
	}
	return source, accounts, nil
}

// accountIdentity is the identity GitHub shows for an account.
type accountIdentity struct {
	name, email string
	emailSource string // "public" or "noreply"
	publicEmail string // the profile's public email, or ""
}

// ghIdentity returns the account's public name (or its login) and its public
// email; an account that shows no email gets its GitHub noreply address.
func ghIdentity(ctx context.Context, source provider.GitHub, login string) accountIdentity {
	id := accountIdentity{name: login, email: login + "@users.noreply.github.com", emailSource: "noreply"}
	u, err := source.User(ctx, config.DefaultGitHubHost, login)
	if err != nil {
		return id
	}
	if u.Name != "" {
		id.name = u.Name
	}
	if u.ID > 0 {
		id.email = fmt.Sprintf("%d+%s@users.noreply.github.com", u.ID, login)
	}
	if u.Email != "" {
		id.email, id.emailSource, id.publicEmail = u.Email, "public", u.Email
	}
	return id
}

// matchIdentity finds the existing identity that belongs to login: by its
// credential username, the account's public email, or its noreply email.
func matchIdentity(identities []discover.Identity, login, publicEmail string) int {
	noreply := "+" + strings.ToLower(login) + "@users.noreply.github.com"
	for i, id := range identities {
		email := strings.ToLower(id.Email)
		if strings.EqualFold(id.Account, login) || strings.HasSuffix(email, noreply) ||
			(publicEmail != "" && strings.EqualFold(id.Email, publicEmail)) ||
			email == strings.ToLower(login)+"@users.noreply.github.com" {
			return i
		}
	}
	return -1
}

// PreviewSetup returns every file setting up plan would write, with diffs,
// without writing anything.
func PreviewSetup(ctx context.Context, e Env, plan SetupPlan) ([]FileChange, error) {
	changes, err := e.setupChanges(ctx, plan)
	if err != nil {
		return nil, err
	}
	return e.fileChanges(changes), nil
}

// ApplySetup writes the configuration of plan and the Git configuration
// generated from it, in one backup.
func ApplySetup(ctx context.Context, e Env, plan SetupPlan) error {
	changes, err := e.setupChanges(ctx, plan)
	if err != nil {
		return err
	}
	return e.applyChanges(changes, ApplyOptions{Yes: true, Command: "setup (web)"})
}

func (e Env) setupChanges(ctx context.Context, plan SetupPlan) ([]change, error) {
	if _, err := os.Stat(e.configFile()); err == nil {
		return nil, fmt.Errorf("%s already exists; change it on the Profiles page", e.display(e.configFile()))
	}
	if len(plan.Profiles) == 0 {
		return nil, errors.New("choose at least one profile")
	}
	text := renderPlan(plan)
	cfg, err := config.Parse([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("the profiles cannot be written as a configuration: %w", err)
	}
	if err := cfg.Validate(e.Home, e.CaseInsensitive); err != nil {
		return nil, err
	}
	generated, err := e.planWith(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return append([]change{{path: e.configFile(), content: text, create: true}}, generated...), nil
}

// renderPlan writes plan as a commented configuration file.
func renderPlan(plan SetupPlan) string {
	var b strings.Builder
	b.WriteString("# GitPersona configuration, created on the setup page.\n")
	b.WriteString("# Change it with `gitpersona ui`, `gitpersona use <profile>`, or by hand; see the README.\n")
	b.WriteString("version = 1\n")
	if plan.Default != "" {
		fmt.Fprintf(&b, "default = %q\n", plan.Default)
	}
	for _, p := range plan.Profiles {
		fmt.Fprintf(&b, "\n[[profile]]\nid      = %q\naccount = %q\nname    = %q\nemail   = %q\n", p.ID, p.Account, p.Name, p.Email)
		if len(p.Dirs) > 0 {
			quoted := make([]string, len(p.Dirs))
			for i, d := range p.Dirs {
				quoted[i] = fmt.Sprintf("%q", d)
			}
			fmt.Fprintf(&b, "dirs    = [%s]\n", strings.Join(quoted, ", "))
		}
	}
	return b.String()
}
