package app

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
)

// Verdicts of a repository check, from best to worst.
const (
	VerdictOK      = "ok"      // author and every push account are the profile's
	VerdictNote    = "note"    // as expected, with something worth knowing
	VerdictProblem = "problem" // commits or pushes would use another identity
	// VerdictUnconfirmed: the push account cannot be told from the settings
	// (an SSH key no profile names); `verify --ssh` asks the host.
	VerdictUnconfirmed = "unconfirmed"
	VerdictUnmanaged   = "unmanaged" // no profile applies; Git uses the user's own settings
)

// verifyWorkers bounds the repositories checked at once (each runs a few git commands).
const verifyWorkers = 8

// VerifyReport is the stable --json output of `gitpersona verify`.
type VerifyReport struct {
	Repositories []RepoCheck `json:"repositories"`
	// Scanned are the folders looked in, written with ~.
	Scanned []string `json:"scanned"`
	// Missing are the chosen folders that do not exist.
	Missing []string `json:"missing"`
	// Complete is false when the scan stopped before it looked everywhere.
	Complete bool         `json:"complete"`
	Counts   VerifyCounts `json:"counts"`
	// Total is the number of repositories found; Limited is set when only
	// some of them were checked (the page's automatic check).
	Total   int  `json:"total"`
	Limited bool `json:"limited,omitempty"`
}

// VerifyCounts counts repositories per verdict.
type VerifyCounts struct {
	OK       int `json:"ok"`
	Notes    int `json:"notes"`
	Problems int `json:"problems"`
	// Unconfirmed repositories push with an SSH key no profile names.
	Unconfirmed int `json:"unconfirmed"`
	Unmanaged   int `json:"unmanaged"`
}

// RepoCheck is what one repository really uses, compared with its profile.
type RepoCheck struct {
	Path    string `json:"path"` // written with ~
	Profile string `json:"profile,omitempty"`
	Account string `json:"account,omitempty"`
	Reason  string `json:"reason"`
	Match   string `json:"match,omitempty"`
	// Author is what commits made here use: "Name <email>".
	Author       string `json:"author"`
	AuthorSource string `json:"author_source,omitempty"`
	// ProfileAuthor is the profile's "Name <email>", when a profile applies.
	ProfileAuthor string         `json:"profile_author,omitempty"`
	Remotes       []RemoteCheck  `json:"remotes"`
	Verdict       string         `json:"verdict"`
	Findings      []CheckFinding `json:"findings"`
}

// RemoteCheck is the account one remote pushes as.
type RemoteCheck struct {
	Name     string `json:"name"`
	Address  string `json:"address"`  // host/owner/repository, without user names or passwords
	Protocol string `json:"protocol"` // https, ssh or other
	Host     string `json:"host"`     // the real host (an SSH alias resolved)
	Owner    string `json:"owner,omitempty"`
	// PushAs is the account the host sees on a push, or "" when GitPersona
	// cannot tell (your own credential helper or an unknown SSH key decides).
	PushAs string `json:"push_as,omitempty"`
	// Via says how, for people: "gh token", "your credential helper",
	// "SSH key ~/.ssh/…", in the language of the request. ViaKind is the same
	// as a code that never changes: one of the Via* constants.
	Via     string `json:"via"`
	ViaKind string `json:"via_kind"`
	// ChosenBy says where the repository itself chose the push account, or "",
	// for people; ChosenKind is the same as a code (the Chosen* constants).
	ChosenBy   string `json:"chosen_by,omitempty"`
	ChosenKind string `json:"chosen_kind,omitempty"`
	// Keys are the SSH keys ssh offers, in order, written with ~.
	Keys []string `json:"keys"`
	// Greeted is set when the SSH host was asked (verify --ssh); PushAs is
	// then the account it greeted.
	Greeted     bool     `json:"greeted,omitempty"`
	keys        []string // the same, as real paths
	secretInURL bool     // the URL carries a password or token
	sharedPath  string   // the ControlPath of a shared SSH connection, or ""
}

// CheckFinding is one thing a check found.
type CheckFinding struct {
	Level string `json:"level"` // problem, unconfirmed or note
	Text  string `json:"text"`
}

// verifyContext is what every repository check shares.
type verifyContext struct {
	cfg      config.Config
	signedIn map[string][]string // host -> accounts signed in to gh; nil when unknown
	keys     map[string]config.Profile
	ssh      *sshResolver
	// greet, when set, asks each SSH host which account a push signs in as.
	greet *greeter
}

// VerifyRepos checks every repository in the profiles' folders, the pinned
// repositories and the extra folders: who commits and who pushes there.
// It only reads Git and SSH settings; it never fetches a token, reads a key
// or connects anywhere.
//
// With askHosts, it also connects to each SSH host once per key setting, as a
// push would, and reads which account the host greets: ssh uses the key,
// GitPersona never reads it.
func (e Env) VerifyRepos(ctx context.Context, extra []string, askHosts bool) (VerifyReport, error) {
	t, err := e.verifyTargets(ctx, extra)
	if err != nil {
		return VerifyReport{}, err
	}
	return e.checkTargets(ctx, t, askHosts, 0), nil
}

// verifyTargets is what a check looks at: the configuration, the folders
// and the repositories found in them (a walk of the file system only).
type verifyTargets struct {
	cfg   config.Config
	roots ScanRoots
	found discover.Found
}

func (e Env) verifyTargets(ctx context.Context, extra []string) (verifyTargets, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		return verifyTargets{}, err
	}
	folders := slices.Clone(extra)
	for _, p := range cfg.Profiles {
		folders = append(folders, p.Dirs...)
	}
	for _, pin := range cfg.Pins {
		folders = append(folders, pin.Repo)
	}
	roots, err := e.ResolveScanRoots(folders)
	if err != nil {
		return verifyTargets{}, err
	}
	found := e.scannedRepos(ctx, roots)
	slices.SortFunc(found.Repos, func(a, b discover.Repo) int { return cmp.Compare(a.Dir, b.Dir) })
	return verifyTargets{cfg: cfg, roots: roots, found: found}, nil
}

// checkTargets checks the repositories of t, at most limit of them (0: all).
func (e Env) checkTargets(ctx context.Context, t verifyTargets, askHosts bool, limit int) VerifyReport {
	repos := t.found.Repos
	if limit > 0 && len(repos) > limit {
		repos = repos[:limit]
	}
	vc := verifyContext{cfg: t.cfg, signedIn: e.ghAccountsByHost(ctx, t.cfg), keys: e.profileKeys(t.cfg), ssh: newSSHResolver(e)}
	if askHosts {
		vc.greet = newGreeter(e)
	}
	checks := make([]RepoCheck, len(repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, verifyWorkers)
	for i, r := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			checks[i] = e.checkRepo(ctx, vc, r.Dir)
		}()
	}
	wg.Wait()
	e.flagSharedConnections(checks)
	slices.SortFunc(checks, func(a, b RepoCheck) int { return cmp.Compare(a.Path, b.Path) })
	report := VerifyReport{Repositories: checks, Scanned: t.roots.Shown, Missing: t.roots.Missing,
		Complete: t.found.Complete && !t.found.TooDeep, Total: len(t.found.Repos), Limited: len(repos) < len(t.found.Repos)}
	for _, c := range checks {
		switch c.Verdict {
		case VerdictOK:
			report.Counts.OK++
		case VerdictNote:
			report.Counts.Notes++
		case VerdictProblem:
			report.Counts.Problems++
		case VerdictUnconfirmed:
			report.Counts.Unconfirmed++
		default:
			report.Counts.Unmanaged++
		}
	}
	return report
}

// checkRepo compares what Git uses in one repository with its profile.
func (e Env) checkRepo(ctx context.Context, vc verifyContext, dir string) RepoCheck {
	norm, err := paths.Normalize(dir, e.Home)
	if err != nil {
		norm = dir
	}
	here := e
	here.WorkDir = norm
	who, repo := here.whoamiRepo(ctx, vc.cfg)
	check := RepoCheck{Path: e.display(norm), Profile: who.Profile, Account: who.Account, Reason: who.Reason,
		Match: who.Match, Author: authorOrEmpty(who.EffectiveName, who.EffectiveEmail), AuthorSource: who.EmailSource,
		Remotes: []RemoteCheck{}, Findings: []CheckFinding{}}
	var profile config.Profile
	if who.Found {
		profile, _ = vc.cfg.ProfileByID(who.Profile)
		check.ProfileAuthor = author(profile.Name, profile.Email)
		if !who.Consistent {
			check.problem(e.tr("commits use %s, not the profile's %s: %s", e.orNothing(check.Author), check.ProfileAuthor, e.mismatchAdvice(who)))
		}
	}
	for _, remote := range e.remotesOf(ctx, norm) {
		rc := e.checkRemote(ctx, vc, norm, repo, remote)
		check.Remotes = append(check.Remotes, rc)
		if who.Found {
			check.judgePush(e, vc, profile, rc)
		}
	}
	check.Verdict = check.verdict(who.Found)
	return check
}

// problem, note and unconfirmed add a finding; text is already in the
// language of the request (written with Env.tr).
func (c *RepoCheck) problem(text string) {
	c.Findings = append(c.Findings, CheckFinding{Level: VerdictProblem, Text: text})
}

func (c *RepoCheck) note(text string) {
	c.Findings = append(c.Findings, CheckFinding{Level: VerdictNote, Text: text})
}

// unconfirmed records that the push account cannot be told from the settings.
func (c *RepoCheck) unconfirmed(text string) {
	c.Findings = append(c.Findings, CheckFinding{Level: VerdictUnconfirmed, Text: text})
}

func (c *RepoCheck) verdict(managed bool) string {
	has := func(level string) bool {
		return slices.ContainsFunc(c.Findings, func(f CheckFinding) bool { return f.Level == level })
	}
	switch {
	case has(VerdictProblem):
		return VerdictProblem
	case managed && has(VerdictUnconfirmed):
		return VerdictUnconfirmed
	case !managed:
		return VerdictUnmanaged
	case len(c.Findings) > 0:
		return VerdictNote
	default:
		return VerdictOK
	}
}

// ghAccountsByHost asks gh, once per GitHub host of the profiles, which
// accounts it holds. A host gh cannot be asked about maps to nil.
func (e Env) ghAccountsByHost(ctx context.Context, cfg config.Config) map[string][]string {
	out := map[string][]string{}
	gh, err := e.GHPath()
	if err != nil {
		return out
	}
	source := provider.GitHub{GH: gh, Run: e.Run}
	for _, p := range cfg.Profiles {
		host := strings.ToLower(p.Host)
		if _, done := out[host]; done || p.Provider != config.ProviderGitHub {
			continue
		}
		sctx, cancel := context.WithTimeout(ctx, statusCheckTimeout)
		accounts, err := source.SignedInAccounts(sctx, p.Host)
		cancel()
		if err != nil {
			accounts = nil
		}
		out[host] = accounts
	}
	return out
}

// profileKeys maps each profile's SSH key (an absolute path) to its profile.
func (e Env) profileKeys(cfg config.Config) map[string]config.Profile {
	keys := map[string]config.Profile{}
	for _, p := range cfg.Profiles {
		if p.SSHKey == "" {
			continue
		}
		if norm, err := paths.Normalize(p.SSHKey, e.Home); err == nil {
			keys[paths.ResolveSymlinks(norm)] = p
		}
	}
	return keys
}

// authorOrEmpty is author, or "" without an email.
func authorOrEmpty(name, email string) string {
	if email == "" {
		return ""
	}
	return author(name, email)
}

func (e Env) orNothing(s string) string {
	if strings.TrimSpace(s) == "" {
		return e.tr("no author")
	}
	return s
}
