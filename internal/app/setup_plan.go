package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/provider"
	"github.com/weikxs/gitpersona/internal/shell"
)

// SetupProfile is a profile the setup page proposes, or one the user confirmed.
type SetupProfile struct {
	ID string `json:"id"`
	// Provider and Host are empty for GitHub, and set for a profile of
	// another host, which comes from the existing setup.
	Provider string   `json:"provider,omitempty"`
	Host     string   `json:"host,omitempty"`
	Account  string   `json:"account"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Dirs     []string `json:"dirs"`
	// Source is "existing" when name, email and directories come from the
	// user's own includeIf setup, and "gh" when they come from GitHub.
	Source string `json:"source,omitempty"`
	// EmailSource is "public" for the email on the GitHub profile, "noreply"
	// when the account shows none, or "existing".
	EmailSource string `json:"email_source,omitempty"`
	// NameSource is "public" for the name on the GitHub profile, "login"
	// when the account shows none, "existing", or "global" for the global
	// Git name an existing identity without a name used.
	NameSource string `json:"name_source,omitempty"`
	// SSHKey and Signing come from the existing setup or an SSH alias.
	SSHKey  string         `json:"ssh_key,omitempty"`
	Signing config.Signing `json:"signing"`
	// Found are directories the repositories on disk suggest (proposal only).
	Found []DirSuggestion `json:"found,omitempty"`
	// Notes say what of the existing setup was left out, and why.
	Notes []string `json:"notes,omitempty"`
}

// GlobalIdentity is the user.name and user.email of the global Git
// configuration: what commits use outside every rule before setup.
type GlobalIdentity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SetupProposal is what the setup page starts from.
type SetupProposal struct {
	Default  string         `json:"default"`
	Profiles []SetupProfile `json:"profiles"`
	// Unmatched are identities of an existing setup that no signed-in
	// account claims; the page shows them so nothing is silently dropped.
	Unmatched []LeftOut `json:"unmatched"`
	// Global is the identity used outside every rule today, so the page can
	// say what the default profile replaces.
	Global GlobalIdentity `json:"global"`
	// OwnRules describe the user's own includeIf identity rules that stay; a
	// default profile would override them everywhere, so setup proposes none
	// while there are any. SuggestedDefault is the profile it would propose.
	OwnRules         []string `json:"own_rules"`
	SuggestedDefault string   `json:"suggested_default"`
	// Shadowed are changes setup makes to folders where a rule of the user's
	// never applied (a later rule of theirs overrode it).
	Shadowed []string `json:"shadowed"`
	// Pins are repositories whose own settings name another profile than
	// their directory; Unassigned are folders the user must choose for.
	Pins       []PinSuggestion `json:"pins"`
	Unassigned []UnassignedDir `json:"unassigned"`
	// Roots are the folders that were scanned; Shells the shells to set up.
	Roots  []string `json:"roots"`
	Shells []string `json:"shells"`
	// ScanIncomplete is set when the scan stopped at its time limit, so
	// some folders were not looked at.
	ScanIncomplete bool `json:"scan_incomplete"`
	// ScanUnfinished are the scanned folders it did not finish, with ~.
	ScanUnfinished []string `json:"scan_unfinished"`
	// GitHubUnavailable says why no GitHub account is proposed (gh missing,
	// or no account signed in), when other hosts' profiles are.
	GitHubUnavailable string `json:"github_unavailable,omitempty"`
	// ScanTooDeep is set when folders below the depth limit were not looked in.
	ScanTooDeep bool `json:"scan_too_deep"`
	// OldRules are the directories of hand-made includeIf rules whose
	// identities the profiles take over; setup removes those rules.
	OldRules []string `json:"old_rules"`
	// KeptRules are those rules that do more than a profile and stay.
	KeptRules []KeptRule `json:"kept_rules"`
	// NoShell is set when the user declined the shell integration.
	NoShell bool `json:"-"`
}

// Plan is the proposal accepted as it is: every found directory, pin and
// shell included, unassigned folders left out.
func (p SetupProposal) Plan() SetupPlan {
	plan := SetupPlan{Default: p.Default, Shells: p.Shells, Pins: []ListedPin{}, RemoveOldRules: p.OldRules, NoShell: p.NoShell}
	for _, prof := range p.Profiles {
		dirs := slices.Clone(prof.Dirs)
		for _, f := range prof.Found {
			dirs = append(dirs, f.Dir)
		}
		prof.Dirs, prof.Found = dirs, nil
		plan.Profiles = append(plan.Profiles, prof)
	}
	for _, pin := range p.Pins {
		if pin.needed(p.Default) {
			plan.Pins = append(plan.Pins, ListedPin{Repo: pin.Repo, Profile: pin.Profile})
		}
	}
	return plan
}

// SetupPlan is the configuration the user confirmed on the setup page.
type SetupPlan struct {
	Default  string         `json:"default"`
	Profiles []SetupProfile `json:"profiles"`
	Pins     []ListedPin    `json:"pins"`
	Shells   []string       `json:"shells"` // fish, zsh or bash: add the shell integration too
	// RemoveOldRules are directories whose hand-made includeIf rules go,
	// when a profile of the plan takes them over.
	RemoveOldRules []string `json:"remove_old_rules"`
	// NoShell records that the user chose not to set up any shell.
	NoShell bool `json:"no_shell"`
}

// ProposeSetup suggests one profile per account signed in to gh, and one per
// existing identity for another host (see hostProfiles), reusing
// the name, email, directories and keys of a matching existing includeIf
// setup, and organizes the repositories on disk into directory rules and
// pins (see organize).
// It reads no tokens: accounts come from a status query and names from the
// public user API.
func ProposeSetup(ctx context.Context, e Env) (SetupProposal, error) {
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	existing := e.existingIdentities(ctx)
	source, accounts, ghErr := e.signedInAccounts(ctx)
	if ghErr != nil && len(existing) == 0 {
		return SetupProposal{}, ghErr
	}
	used := make([]bool, len(existing))
	global := e.globalIdentity(ctx)

	proposal := SetupProposal{Profiles: []SetupProfile{}}
	for _, a := range accounts {
		if a.State != "success" {
			continue
		}
		p := SetupProfile{ID: profileID(a.Login), Account: a.Login, Dirs: []string{}, Source: "gh"}
		gh := ghIdentity(ctx, source, a.Login)
		p.Name, p.Email, p.EmailSource, p.NameSource = gh.name, gh.email, gh.emailSource, gh.nameSource
		if i := matchIdentity(existing, used, a.Login, gh.publicEmail); i >= 0 {
			used[i] = true
			p = e.adopt(p, existing[i], global.Name)
		}
		if a.Active {
			proposal.Default = p.ID
		}
		proposal.Profiles = append(proposal.Profiles, p)
	}
	proposal.Global = global
	proposal, err := e.organize(ctx, proposal, existing, used)
	if err != nil {
		return SetupProposal{}, err
	}
	if ghErr != nil && len(proposal.Profiles) == 0 {
		return SetupProposal{}, fmt.Errorf("%w\n(GitPersona also found %s in your Git settings, but none names one account "+
			"on one host other than GitHub; add credential.https://<host>.username to its includeIf file, or write the profile in "+
			"config.toml yourself)", ghErr, identities(len(existing)))
	}
	if ghErr != nil {
		proposal.GitHubUnavailable = ghErr.Error()
	}
	return proposal, nil
}

// globalIdentity reads user.name and user.email from the global Git
// configuration itself, not from files it includes.
func (e Env) globalIdentity(ctx context.Context) GlobalIdentity {
	get := func(key string) string {
		out, _ := e.git(ctx, e.Home, "config", "--file", e.GlobalGitConfigPath(), "--get", key)
		return out
	}
	return GlobalIdentity{Name: get("user.name"), Email: get("user.email")}
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
	nameSource  string // "public" or "login"
	publicEmail string // the profile's public email, or ""
}

// ghIdentity returns the account's public name (or its login) and its public
// email; an account that shows no email gets its GitHub noreply address.
func ghIdentity(ctx context.Context, source provider.GitHub, login string) accountIdentity {
	id := accountIdentity{name: login, nameSource: "login", email: login + "@users.noreply.github.com", emailSource: "noreply"}
	u, err := source.User(ctx, config.DefaultGitHubHost, login)
	if err != nil {
		return id
	}
	if u.Name != "" {
		id.name, id.nameSource = u.Name, "public"
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
func matchIdentity(identities []discover.Identity, used []bool, login, publicEmail string) int {
	noreply := "+" + strings.ToLower(login) + "@users.noreply.github.com"
	for i, id := range identities {
		if used[i] || forOtherHost(id) {
			continue
		}
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
	return e.applyChanges(changes, ApplyOptions{Yes: true, Command: "setup"})
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
	for _, p := range cfg.Profiles {
		if err := requireKeyFiles(p, e.Home); err != nil {
			return nil, fmt.Errorf("profile %s: %w", p.ID, err)
		}
	}
	generated, err := e.planWith(ctx, cfg, e.oldRulesEdit(ctx, plan))
	if err != nil {
		return nil, err
	}
	kinds, err := ParseShells(plan.Shells)
	if err != nil {
		return nil, err
	}
	shells, err := e.shellChanges(kinds)
	if err != nil {
		return nil, err
	}
	changes := append([]change{{path: e.configFile(), content: text, create: true}}, generated...)
	return append(changes, shells...), nil
}

// oldRulesEdit removes from the global Git configuration the hand-made
// includeIf rules the plan asks to remove and its profiles take over, in the
// same change (and backup) as the rest of the setup. The user ticked these
// rules for removal, perhaps after editing the name, so a profile with the
// email covering the folder is enough.
func (e Env) oldRulesEdit(ctx context.Context, plan SetupPlan) func(string) string {
	if len(plan.RemoveOldRules) == 0 {
		return nil
	}
	owners := setupOwners(plan.Profiles)
	for k := range owners {
		owners[k].anyName = true
	}
	return func(text string) string {
		updated, _, _ := e.withoutOldRules(ctx, text, owners, plan.RemoveOldRules)
		return updated
	}
}

// ParseShells parses shell names such as "zsh".
func ParseShells(names []string) ([]shell.Kind, error) {
	kinds := make([]shell.Kind, 0, len(names))
	for _, name := range names {
		k, err := shell.Parse(name)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, k)
	}
	return kinds, nil
}

// emailNote says where a proposed email came from, as a comment.
func emailNote(source string) string {
	switch source {
	case "public":
		return " # public email on GitHub"
	case "noreply":
		return " # GitHub noreply address; the account shows no public email"
	}
	return ""
}

// renderPlan writes plan as a commented configuration file.
func renderPlan(plan SetupPlan) string {
	var b strings.Builder
	b.WriteString("# GitPersona configuration, created by `gitpersona setup`.\n")
	b.WriteString("# Change it with `gitpersona ui`, `gitpersona use <profile>`, or by hand; see the README.\n")
	b.WriteString("version = 1\n")
	if plan.Default != "" {
		fmt.Fprintf(&b, "default = %q\n", plan.Default)
	}
	if plan.NoShell {
		b.WriteString("shell_integration = false # you chose not to load GitPersona in your shells\n")
	}
	for _, p := range plan.Profiles {
		fmt.Fprintf(&b, "\n[[profile]]\nid      = %q\n", p.ID)
		if !p.onGitHub() {
			fmt.Fprintf(&b, "provider = %q\nhost    = %q\n", p.Provider, p.Host)
		}
		fmt.Fprintf(&b, "account = %q\nname    = %q\nemail   = %q%s\n", p.Account, p.Name, p.Email, emailNote(p.EmailSource))
		if len(p.Dirs) > 0 {
			quoted := make([]string, len(p.Dirs))
			for i, d := range p.Dirs {
				quoted[i] = fmt.Sprintf("%q", d)
			}
			fmt.Fprintf(&b, "dirs    = [%s]\n", strings.Join(quoted, ", "))
		}
		if p.SSHKey != "" {
			fmt.Fprintf(&b, "ssh_key = %q\n", p.SSHKey)
		}
		switch p.Signing.Method {
		case config.SigningSSH, config.SigningGPG:
			fmt.Fprintf(&b, "signing = { method = %q, key = %q }\n", p.Signing.Method, p.Signing.Key)
		case config.SigningNone:
			b.WriteString("signing = { method = \"none\" }\n")
		}
	}
	for _, pin := range plan.Pins {
		fmt.Fprintf(&b, "\n[[pin]]\nrepo    = %q\nprofile = %q\n", pin.Repo, pin.Profile)
	}
	return b.String()
}

// identities says how many identities there are.
func identities(n int) string {
	if n == 1 {
		return "1 identity"
	}
	return fmt.Sprintf("%d identities", n)
}
