package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/discover"
)

// Several rules of the user's can name the same GitHub account with
// another author, such as a team folder that commits under another name.
// A profile takes over one identity; the others would be overridden in its
// folders. Each such identity becomes a profile of its own for the same
// account, so every folder keeps the author it has today.

// siblingProfiles makes a profile for each identity no profile took that
// names the account of a proposed GitHub profile, and marks it used. An
// identity with the same author, SSH key and signing as that profile only
// adds its folders. Only the folders where the identity's rule applies
// today and a profile can replace it count; an identity without any stays
// the user's.
func (e Env) siblingProfiles(ctx context.Context, profiles []SetupProfile, existing []discover.Identity, used []bool, globalName string) []SetupProfile {
	rules, never := e.identityRules(ctx), e.neverApplied(ctx)
	out := append([]SetupProfile{}, profiles...)
	ids := map[string]bool{}
	for _, p := range out {
		ids[p.ID] = true
	}
	for i, id := range existing {
		if used[i] || id.Account == "" {
			continue
		}
		k := e.profileOfAccount(out, id.Account)
		if k < 0 {
			continue
		}
		id.Dirs = liveReplaceableDirs(id, rules, never)
		if len(id.Dirs) == 0 {
			continue
		}
		used[i] = true
		base := out[k]
		key, signing, _ := e.existingKeys(id)
		if sameAuthor(base, id, globalName) && key == base.SSHKey && signing == base.Signing {
			out[k].Dirs = append(append([]string{}, base.Dirs...), id.Dirs...)
			continue
		}
		sibling := SetupProfile{
			ID:      uniqueID(siblingID(base.ID, id.Dirs), "", ids),
			Account: base.Account, Dirs: []string{}, Source: "existing",
		}
		ids[sibling.ID] = true
		out = append(out, e.adopt(sibling, id, globalName))
	}
	return out
}

// liveIdentities returns the identities with only the folders where their
// rule applies today: a later rule or global identity of the user's
// overrides the others, and a profile there would change the author.
func (e Env) liveIdentities(ctx context.Context, ids []discover.Identity) []discover.Identity {
	rules, never := e.identityRules(ctx), e.neverApplied(ctx)
	out := make([]discover.Identity, len(ids))
	for i, id := range ids {
		var dirs []string
		for _, d := range id.Dirs {
			applies := true
			for _, r := range rules {
				if r.Dir == d && strings.EqualFold(r.Email, id.Email) && r.Name == id.Name && never[ruleKey(r.File, r.Condition)] != "" {
					applies = false
				}
			}
			if applies {
				dirs = append(dirs, d)
			}
		}
		id.Dirs = dirs
		out[i] = id
	}
	return out
}

// liveReplaceableDirs returns the folders of an identity whose rule a
// profile can replace (it sets nothing more) and that apply today (no later
// rule or global identity of the user's overrides them).
func liveReplaceableDirs(id discover.Identity, rules []discover.Rule, never map[string]string) []string {
	var dirs []string
	for _, d := range id.Dirs {
		live := false
		for _, r := range rules {
			if r.Dir != d || !strings.EqualFold(r.Email, id.Email) || r.Name != id.Name {
				continue
			}
			if r.Keep != "" || never[ruleKey(r.File, r.Condition)] != "" {
				live = false
				break
			}
			live = true
		}
		if live {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// profileOfAccount returns the index of the GitHub profile for account,
// or -1.
func (e Env) profileOfAccount(profiles []SetupProfile, account string) int {
	for k, p := range profiles {
		if p.onGitHub() && strings.EqualFold(p.Account, account) {
			return k
		}
	}
	return -1
}

// sameAuthor reports whether an identity commits as profile p does.
func sameAuthor(p SetupProfile, id discover.Identity, globalName string) bool {
	name := id.Name
	if name == "" {
		name = globalName
	}
	return strings.EqualFold(p.Email, id.Email) && p.Name == name
}

// siblingID names a second profile of an account after its first folder.
func siblingID(base string, dirs []string) string {
	if len(dirs) == 0 {
		return profileID(base + "-2")
	}
	id := profileID(base + "-" + filepath.Base(dirs[0]))
	if len(id) > maxIDLen-4 {
		id = strings.TrimRight(id[:maxIDLen-4], "-")
	}
	return id
}
