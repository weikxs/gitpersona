package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/shell"
)

// DirSuggestion is a directory the setup proposes for a profile, and why.
type DirSuggestion struct {
	Dir string `json:"dir"`
	Why string `json:"why"`
}

// PinSuggestion is a repository whose own settings say it belongs to another
// profile than the directory it sits in.
type PinSuggestion struct {
	Repo    string `json:"repo"`
	Profile string `json:"profile"`
	Why     string `json:"why"`
	// Outside is set when no directory rule covers the repository, so the
	// default applies there: the pin is needed only while Profile is not
	// the default.
	Outside bool `json:"outside,omitempty"`
}

// needed reports whether the pin changes anything with defaultID as the
// default profile.
func (p PinSuggestion) needed(defaultID string) bool { return !p.Outside || p.Profile != defaultID }

// UnassignedDir is a directory whose repositories belong to an owner that is
// none of the user's accounts, such as an organization; the user chooses.
type UnassignedDir struct {
	Dir   string `json:"dir"`
	Owner string `json:"owner"`
	Why   string `json:"why"`
}

// organize scans the usual code folders, the folders of the existing
// identities and the SSH configuration (never keys). It ties existing
// identities to accounts by what it finds, and fills the proposal with
// directory rules, pins, folders the user must choose for, SSH keys of SSH
// aliases, the old rules the profiles take over, and the login shell to set
// up.
func (e Env) organize(ctx context.Context, p SetupProposal, existing []discover.Identity, used []bool) (SetupProposal, error) {
	// The folders of existing rules come first: the scan has a time limit,
	// and they decide whose an existing identity is.
	roots, err := e.ResolveScanRoots(append(identityDirs(existing), DefaultScanRoots...))
	if err != nil {
		return SetupProposal{}, err
	}
	aliases := discover.SSHAliases(filepath.Join(e.Home, ".ssh", "config"), e.Home)
	found := e.scannedRepos(ctx, roots)
	repos := found.Repos

	profiles := slices.Clone(p.Profiles)
	claims := claimIdentities(newInference(profiles, repos, aliases, e.comparisonKey), existing, used, repos, aliases)
	for _, i := range sortedClaims(claims) {
		used[i] = true
		k := slices.IndexFunc(profiles, func(x SetupProfile) bool { return x.Account == claims[i] })
		profiles[k] = e.adopt(profiles[k], existing[i], p.Global.Name)
	}

	l := inferLayout(profiles, repos, aliases, e.comparisonKey, identityDirs(unused(existing, used)))
	for i, prof := range profiles {
		prof.Found = l.found[prof.Account]
		if prof.SSHKey == "" {
			prof.SSHKey = e.aliasKey(prof.Account, aliases, l.aliasAccount)
		}
		profiles[i] = prof
	}
	p.Profiles, p.Roots, p.ScanIncomplete, p.ScanTooDeep = profiles, roots.Shown, !found.Complete, found.TooDeep
	p.Pins = append([]PinSuggestion{}, l.pins...)
	p.Unassigned = append([]UnassignedDir{}, l.unassigned...)

	owners := setupOwners(profiles)
	replaced, kept := e.classifyOldRules(ctx, owners, nil)
	p.OldRules, p.KeptRules = []string{}, kept
	for _, r := range replaced {
		p.OldRules = append(p.OldRules, r.Dir)
	}
	p.Unmatched = e.leftOut(ctx, existing, used, owners, p.Default)

	p.Shells = []string{}
	if k := e.loginShell(ctx).Kind; k != shell.Unknown {
		if _, done := e.integrated(k); !done {
			p.Shells = []string{string(k)}
		}
	}
	return p, nil
}

// scannedRepos finds the repositories under roots once each, even when two
// roots reach the same folder, written as the user reaches them.
func (e Env) scannedRepos(ctx context.Context, roots ScanRoots) discover.Found {
	seen := map[string]bool{}
	var repos []discover.Repo
	found := discover.Repos(ctx, roots.Real, discover.DefaultScanLimits)
	for _, r := range found.Repos {
		realPath := paths.ResolveSymlinks(r.Dir)
		if seen[realPath] {
			continue
		}
		seen[realPath] = true
		r.Dir = roots.Written(e, r.Dir)
		repos = append(repos, r)
	}
	found.Repos = repos
	return found
}

// comparisonKey is the form two paths are compared in: absolute, and
// lowercase on a case-insensitive disk.
func (e Env) comparisonKey(path string) string {
	norm, err := paths.Normalize(path, e.Home)
	if err != nil {
		norm = path
	}
	if e.CaseInsensitive {
		return strings.ToLower(norm)
	}
	return norm
}

// aliasKey returns the IdentityFile of the account's SSH alias, when that
// file exists. It only checks that the file is there; it never opens it.
func (e Env) aliasKey(account string, aliases []discover.SSHAlias, aliasAccount map[string]string) string {
	for _, a := range aliases {
		if aliasAccount[strings.ToLower(a.Alias)] == account && a.IdentityFile != "" && e.keyExists(a.IdentityFile) {
			return a.IdentityFile
		}
	}
	return ""
}

// existingKeys returns the SSH key and signing of an existing identity whose
// key files exist, with a note for each one left out because its file is
// missing.
func (e Env) existingKeys(id discover.Identity) (string, config.Signing, []string) {
	key, notes := "", []string{}
	switch {
	case id.SSHKey != "" && e.keyExists(id.SSHKey):
		key = e.displayPath(id.SSHKey)
	case id.SSHKey != "":
		notes = append(notes, "the SSH key "+e.displayPath(id.SSHKey)+" of your includeIf setup does not exist, so it is left out")
	}
	signing := config.Signing{Method: id.Signing.Method, Key: id.Signing.Key}
	if signing.Method == config.SigningSSH && !e.keyExists(signing.Key) {
		notes = append(notes, "the signing key "+e.displayPath(signing.Key)+" of your includeIf setup does not exist, so signing is left out")
		signing = config.Signing{}
	}
	signing.Key = e.displayPath(signing.Key)
	return key, signing, notes
}

// displayPath writes a path under the home as ~/...; other values (such as
// a GPG key id) stay as they are.
func (e Env) displayPath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	return e.Display(path)
}

// keyExists reports whether a key file is there, without opening it.
func (e Env) keyExists(path string) bool { return keyFileExists(path, e.Home, false) }
