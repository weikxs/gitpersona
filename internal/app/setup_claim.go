package app

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/discover"
)

// LeftOut is an identity of the user's existing includeIf setup that no
// profile takes over. OverriddenBy names the profile whose rules replace it
// in its folder (GitPersona's rules come after the user's: a profile's
// directory around it, or the default), or is "" when the rule keeps
// applying. Replaceable are the directories whose rules could go once a
// profile takes it over.
type LeftOut struct {
	discover.Identity
	OverriddenBy string   `json:"overridden_by,omitempty"`
	Replaceable  []string `json:"replaceable"`
	// ReplacedIn are profile directories inside its folder, where the
	// profile's identity applies instead.
	ReplacedIn []string `json:"replaced_in"`
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

// leftOut describes the identities no profile takes over: which profile
// replaces them, and which of their rules could go if one took them over.
// An identity without a name is shown with globalName, which it commits with.
func (e Env) leftOut(ctx context.Context, ids []discover.Identity, used []bool, owners []ruleOwner, defaultID, globalName string) []LeftOut {
	rules := e.identityRules(ctx)
	out := []LeftOut{}
	for i, id := range ids {
		if used[i] {
			continue
		}
		id.Source = e.Display(id.Source)
		key, signing, _ := e.existingKeys(id) // offered only with keys that exist
		id.SSHKey, id.Signing = key, discover.Signing{Method: signing.Method, Key: signing.Key}
		l := LeftOut{Identity: id, Replaceable: []string{}, ReplacedIn: []string{}}
		for _, d := range id.Dirs {
			if by, ok := e.overriddenBy(d, owners, defaultID); ok && !by.sameIdentity(id.Email, id.Name) && l.OverriddenBy == "" {
				l.OverriddenBy = by.id
			}
			l.ReplacedIn = append(l.ReplacedIn, e.profileDirsInside(d, owners)...)
		}
		for _, r := range rules {
			if strings.EqualFold(r.Email, id.Email) && r.Name == id.Name && r.Keep == "" {
				l.Replaceable = append(l.Replaceable, r.Dir)
			}
		}
		if l.Name == "" {
			l.Name = globalName
		}
		out = append(out, l)
	}
	return out
}

// sortedClaims returns the claimed identity indexes in order.
func sortedClaims(claims map[int]string) []int { return slices.Sorted(maps.Keys(claims)) }

// ownRules describes the user's own includeIf identity rules that stay:
// those of the identities left out, those kept because they do more than a
// profile, and rules that are not directory rules (onbranch, hasconfig).
func (e Env) ownRules(ctx context.Context, left []LeftOut, kept []KeptRule) []string {
	out := []string{}
	for _, l := range left {
		if l.OverriddenBy == "" { // a profile's folder around it decides there anyway
			out = append(out, author(l.Name, l.Email)+" for "+strings.Join(l.Dirs, ", "))
		}
	}
	for _, k := range kept {
		if k.OverriddenBy == "" {
			out = append(out, "your rule for "+k.Dir)
		}
	}
	for _, r := range e.otherIdentityRules(ctx) {
		out = append(out, author(r.Name, r.Email)+" for "+r.Condition)
	}
	return out
}

// withoutKeptDirs takes out of the profiles the directories of rules that do
// more than a profile can (KeptRule says why), and directories that contain
// one: in a profile's directory the profile wins, so those settings would be
// lost. The rules keep applying there.
func (e Env) withoutKeptDirs(ctx context.Context, profiles []SetupProfile) ([]SetupProfile, []KeptRule) {
	why := map[string]string{}
	var keptRules []string
	for _, r := range e.identityRules(ctx) {
		if r.Keep != "" {
			why[e.comparisonKey(r.Dir)] = r.Keep
			keptRules = append(keptRules, r.Dir)
		}
	}
	reason := func(d string) (string, bool) {
		if r, ok := why[e.comparisonKey(d)]; ok {
			return r, true
		}
		for _, k := range keptRules {
			if coversWith(e.comparisonKey, []string{d}, k) {
				return "it contains your rule for " + k + " (" + why[e.comparisonKey(k)] + ")", true
			}
		}
		return "", false
	}
	kept := []KeptRule{}
	out := make([]SetupProfile, len(profiles))
	for i, p := range profiles {
		p.Dirs = slices.DeleteFunc(slices.Clone(p.Dirs), func(d string) bool {
			if r, ok := reason(d); ok {
				kept = append(kept, KeptRule{Dir: d, Why: r})
				return true
			}
			return false
		})
		out[i] = p
	}
	return out, kept
}

// keptDirs returns the directories of kept rules.
func keptDirs(kept []KeptRule) []string {
	dirs := make([]string, len(kept))
	for i, k := range kept {
		dirs[i] = k.Dir
	}
	return dirs
}

// keptOverrides says, for each kept rule, which profile applies in its
// folder instead (a profile's folder around it, or the default), if any.
func (e Env) keptOverrides(kept []KeptRule, owners []ruleOwner, defaultID string) []KeptRule {
	out := make([]KeptRule, len(kept))
	for i, k := range kept {
		k.OverriddenBy = ""
		if by, ok := e.overriddenBy(k.Dir, owners, defaultID); ok {
			k.OverriddenBy = by.id
		}
		k.ReplacedIn = e.profileDirsInside(k.Dir, owners)
		out[i] = k
	}
	return out
}

// shadowedRules describes the user's directory rules that never applied in
// their folder, because a later rule of theirs for a folder around it sets
// another identity, but that a profile now applies there: after setup that
// folder's identity changes.
func (e Env) shadowedRules(ctx context.Context, owners []ruleOwner) []string {
	rules := e.identityRules(ctx)
	var out []string
	for i, inner := range rules {
		// Git applies the last rule that matches: the last later rule whose
		// folder contains this one decides there.
		last := -1
		for j := i + 1; j < len(rules); j++ {
			if coversWith(e.comparisonKey, []string{rules[j].Dir}, inner.Dir) {
				last = j
			}
		}
		if last < 0 {
			continue
		}
		outer := rules[last]
		if strings.EqualFold(inner.Email, outer.Email) && inner.Name == outer.Name {
			continue
		}
		if by, ok := e.overriddenBy(inner.Dir, owners, ""); ok && by.sameIdentity(inner.Email, inner.Name) {
			out = append(out, fmt.Sprintf("your rule for %s (%s) never applied, because your later rule for %s (%s) overrides it; "+
				"after setup %s uses %s", inner.Dir, author(inner.Name, inner.Email), outer.Dir, author(outer.Name, outer.Email),
				inner.Dir, author(inner.Name, inner.Email)))
		}
	}
	return out
}
