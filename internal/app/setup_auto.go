package app

import (
	"context"
	"path/filepath"
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
}

// UnassignedDir is a directory whose repositories belong to an owner that is
// none of the user's accounts, such as an organization; the user chooses.
type UnassignedDir struct {
	Dir   string `json:"dir"`
	Owner string `json:"owner"`
	Why   string `json:"why"`
}

// organize scans the usual code folders and the SSH configuration (never
// keys), and fills the proposal with directory rules, pins, folders the user
// must choose for, SSH keys of SSH aliases, and the login shell to set up.
func (e Env) organize(ctx context.Context, p SetupProposal) (SetupProposal, error) {
	roots, err := e.ResolveScanRoots(DefaultScanRoots)
	if err != nil {
		return SetupProposal{}, err
	}
	aliases := discover.SSHAliases(filepath.Join(e.Home, ".ssh", "config"), e.Home)
	l := inferLayout(p.Profiles, e.scannedRepos(ctx, roots), aliases, e.comparisonKey)

	profiles := make([]SetupProfile, len(p.Profiles))
	for i, prof := range p.Profiles {
		prof.Found = l.found[prof.Account]
		if prof.SSHKey == "" {
			prof.SSHKey = e.aliasKey(prof.Account, aliases, l.aliasAccount)
		}
		profiles[i] = prof
	}
	p.Profiles, p.Roots = profiles, roots.Shown
	p.Pins = append([]PinSuggestion{}, l.pins...)
	p.Unassigned = append([]UnassignedDir{}, l.unassigned...)
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
func (e Env) scannedRepos(ctx context.Context, roots ScanRoots) []discover.Repo {
	seen := map[string]bool{}
	var repos []discover.Repo
	for _, r := range discover.Repos(ctx, roots.Real, discover.DefaultScanLimits) {
		real := paths.ResolveSymlinks(r.Dir)
		if seen[real] {
			continue
		}
		seen[real] = true
		r.Dir = roots.Written(e, r.Dir)
		repos = append(repos, r)
	}
	return repos
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
// key files exist.
func (e Env) existingKeys(id discover.Identity) (string, config.Signing) {
	key := ""
	if id.SSHKey != "" && e.keyExists(id.SSHKey) {
		key = id.SSHKey
	}
	signing := config.Signing{Method: id.Signing.Method, Key: id.Signing.Key}
	if signing.Method == config.SigningSSH && !e.keyExists(signing.Key) {
		signing = config.Signing{}
	}
	return key, signing
}

// keyExists reports whether a key file is there, without opening it.
func (e Env) keyExists(path string) bool { return keyFileExists(path, e.Home, false) }
