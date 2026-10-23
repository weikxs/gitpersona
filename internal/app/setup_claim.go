package app

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/discover"
)

// LeftOut is an identity of the user's existing includeIf setup that no
// profile takes over. OverriddenBy names the profile that applies in its
// directory instead (GitPersona's rules come last), or is "". Replaceable
// are the directories whose rules could go once a profile takes it over.
type LeftOut struct {
	discover.Identity
	OverriddenBy string   `json:"overridden_by,omitempty"`
	Replaceable  []string `json:"replaceable"`
}

// Evidence weights for tying an identity to an account. A single repository
// owned by the account is not enough on its own: it may be a clone.
const (
	keyWeight      = 3 // the identity's SSH key is the account's SSH alias key
	settingsWeight = 2 // a repository's own email, SSH alias or remote user
	ownerWeight    = 1 // a repository the account owns
	claimThreshold = 2
)

// claimIdentities ties existing identities that no account claimed by its
// name or email to the account the disk points to: the SSH key the identity
// uses is the key of that account's SSH alias, or the repositories in its
// directories, or that set its email, belong to that account. The strongest
// ties are made first, and each account takes at most one identity. It
// returns identity index -> account.
func claimIdentities(in inference, ids []discover.Identity, used []bool, repos []discover.Repo, aliases []discover.SSHAlias) map[int]string {
	type candidate struct {
		identity int
		account  string
		score    int
	}
	var candidates []candidate
	for i, id := range ids {
		if used[i] {
			continue
		}
		scores := in.claimScores(id, repos, aliases)
		if account, n := majority(scores); n >= claimThreshold && !tied(scores, n) {
			candidates = append(candidates, candidate{i, account, n})
		}
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int { return b.score - a.score })
	taken := map[string]bool{}
	for _, p := range in.profiles {
		taken[p.Account] = p.Source == "existing"
	}
	out := map[int]string{}
	for _, c := range candidates {
		if !taken[c.account] {
			out[c.identity], taken[c.account] = c.account, true
		}
	}
	return out
}

// claimScores weighs the evidence that identity id belongs to each account.
func (in inference) claimScores(id discover.Identity, repos []discover.Repo, aliases []discover.SSHAlias) map[string]int {
	scores := map[string]int{}
	for _, a := range aliases {
		account := in.aliasAccount[strings.ToLower(a.Alias)]
		if account != "" && id.SSHKey != "" && a.IdentityFile != "" && in.key(a.IdentityFile) == in.key(id.SSHKey) {
			scores[account] += keyWeight
		}
	}
	for _, r := range repos {
		if !in.covers(id.Dirs, r.Dir) && !strings.EqualFold(strings.TrimSpace(r.Email), id.Email) {
			continue
		}
		a, ok := in.attribute(r)
		switch {
		case !ok || a.account == "":
		case a.how == byOwner:
			scores[a.account] += ownerWeight
		default:
			scores[a.account] += settingsWeight
		}
	}
	return scores
}

// tied reports whether more than one key has the highest count n.
func tied(counts map[string]int, n int) bool {
	top := 0
	for _, c := range counts {
		if c == n {
			top++
		}
	}
	return top > 1
}

// adopt gives profile p the name, email, directories and keys of an
// existing identity. An identity without a name used the global name, which
// it keeps. Keys whose files are missing are left out with a note.
func (e Env) adopt(p SetupProfile, id discover.Identity, globalName string) SetupProfile {
	switch {
	case id.Name != "":
		p.Name, p.NameSource = id.Name, "existing"
	case globalName != "":
		p.Name, p.NameSource = globalName, "global"
	}
	p.Email, p.EmailSource, p.Source = id.Email, "existing", "existing"
	p.Dirs = append(slices.Clone(p.Dirs), id.Dirs...)
	p.SSHKey, p.Signing, p.Notes = e.existingKeys(id)
	return p
}

// unused returns the identities no profile took over.
func unused(ids []discover.Identity, used []bool) []discover.Identity {
	var out []discover.Identity
	for i, id := range ids {
		if !used[i] {
			out = append(out, id)
		}
	}
	return out
}

// identityDirs returns the directories of the existing identities, to scan.
func identityDirs(ids []discover.Identity) []string {
	var dirs []string
	for _, id := range ids {
		dirs = append(dirs, id.Dirs...)
	}
	return dirs
}

// setupOwners are the proposed profiles as old rules see them, with the
// directories found for them.
func setupOwners(profiles []SetupProfile) []ruleOwner {
	owners := make([]ruleOwner, len(profiles))
	for i, p := range profiles {
		dirs := slices.Clone(p.Dirs)
		for _, f := range p.Found {
			dirs = append(dirs, f.Dir)
		}
		owners[i] = ruleOwner{id: p.ID, name: p.Name, email: p.Email, dirs: dirs}
	}
	return owners
}

// leftOut describes the identities no profile takes over: what overrides
// them, and which of their rules could go if one did.
func (e Env) leftOut(ctx context.Context, ids []discover.Identity, used []bool, owners []ruleOwner, defaultID string) []LeftOut {
	rules := discover.IdentityRules(ctx, e.Home, e.resolvedUserGitConfig(), e.Run)
	out := []LeftOut{}
	for i, id := range ids {
		if used[i] {
			continue
		}
		id.Source = e.Display(id.Source)
		key, signing, _ := e.existingKeys(id) // offered only with keys that exist
		id.SSHKey, id.Signing = key, discover.Signing{Method: signing.Method, Key: signing.Key}
		l := LeftOut{Identity: id, Replaceable: []string{}}
		for _, d := range id.Dirs {
			if by, ok := e.overriddenBy(d, owners, defaultID); ok && l.OverriddenBy == "" && !by.sameIdentity(id.Email, id.Name) {
				l.OverriddenBy = by.id
			}
		}
		for _, r := range rules {
			if strings.EqualFold(r.Email, id.Email) && r.Name == id.Name && r.Keep == "" {
				l.Replaceable = append(l.Replaceable, r.Dir)
			}
		}
		out = append(out, l)
	}
	return out
}

// sortedClaims returns the claimed identity indexes in order.
func sortedClaims(claims map[int]string) []int { return slices.Sorted(maps.Keys(claims)) }
