package app

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
)

// remote is one remote's push URL, as Git uses it (insteadOf applied).
type remote struct {
	name string
	url  string
}

// remotesOf lists a repository's remotes with the URL a push goes to.
func (e Env) remotesOf(ctx context.Context, dir string) []remote {
	names, err := e.git(ctx, dir, "remote")
	if err != nil || names == "" {
		return nil
	}
	var out []remote
	for _, name := range strings.Fields(names) {
		urls, err := e.git(ctx, dir, "remote", "get-url", "--push", name)
		if err != nil || urls == "" {
			continue
		}
		first, _, _ := strings.Cut(urls, "\n")
		out = append(out, remote{name: name, url: strings.TrimSpace(first)})
	}
	return out
}

// checkRemote works out which account a push to one remote authenticates as.
func (e Env) checkRemote(ctx context.Context, vc verifyContext, dir string, r remote) RemoteCheck {
	host, owner, user := discover.RemoteParts(r.url)
	rc := RemoteCheck{Name: r.name, Address: remoteAddress(r.url), Protocol: remoteProtocol(r.url), Host: host,
		Owner: owner, Keys: []string{}}
	switch rc.Protocol {
	case "https", "http":
		e.httpsPush(ctx, dir, r.url, user, &rc)
	case "ssh":
		e.sshPush(ctx, vc, dir, host, &rc)
	}
	return rc
}

// httpsPush: the user name Git sends is the URL's, else credential.username;
// GitPersona's helper answers for that account, or the profile's.
func (e Env) httpsPush(ctx context.Context, dir, url, user string, rc *RemoteCheck) {
	helper, _ := e.git(ctx, dir, "config", "--get-urlmatch", "credential.helper", url)
	named, _ := e.git(ctx, dir, "config", "--get-urlmatch", "credential.username", url)
	local, _ := e.git(ctx, dir, "config", "--local", "--get-urlmatch", "credential.username", url)
	account, provider, ours := gitPersonaHelper(helper)
	if urlHasPassword(url) {
		// Git sends the URL's password and asks no helper: the token decides.
		rc.Via, rc.ChosenBy, rc.secretInURL = "a token written in the remote URL", "the remote URL", true
		return
	}
	rc.PushAs = firstNonEmpty(user, named, account)
	switch {
	case user != "":
		rc.ChosenBy = "the remote URL names it"
	case local != "":
		rc.ChosenBy = "credential.username in the repository"
	}
	switch {
	case ours && provider == config.ProviderGitHub:
		rc.Via = "gh token"
	case ours:
		rc.Via = "GitPersona"
	default:
		rc.Via = "your credential helper"
	}
}

// sshPush: the account is the one whose key ssh offers first.
func (e Env) sshPush(ctx context.Context, vc verifyContext, dir, alias string, rc *RemoteCheck) {
	resolved := vc.ssh.resolve(ctx, alias)
	rc.Host = resolved.hostname
	command := os.Getenv("GIT_SSH_COMMAND")
	switch {
	case command != "":
		rc.ChosenBy = "GIT_SSH_COMMAND"
	default:
		command, _ = e.git(ctx, dir, "config", "core.sshCommand")
		if local, _ := e.git(ctx, dir, "config", "--local", "core.sshCommand"); local != "" {
			rc.ChosenBy = "core.sshCommand in the repository"
		}
	}
	keys := e.dedupeKeys(append(identityFlags(command), resolved.files...))
	rc.keys = keys
	rc.sharedPath = sharedConnection(command, resolved.controlPath)
	for _, k := range keys {
		rc.Keys = append(rc.Keys, e.display(k))
	}
	if len(keys) == 0 {
		rc.Via = "ssh-agent or ssh's default keys"
		return
	}
	rc.Via = "SSH key " + e.display(keys[0])
	if p, ok := vc.keys[keys[0]]; ok && strings.EqualFold(p.Host, rc.Host) {
		rc.PushAs = p.Account
	}
	if vc.greet != nil && profileHost(vc.cfg, rc.Host) {
		g := vc.greet.ask(ctx, alias, command)
		rc.Greeted, rc.PushAs = true, g.account
		switch {
		case g.account != "":
			rc.Via += ", " + rc.Host + " greets " + g.account
			switch {
			case strings.Contains(g.accepted, "/"):
				rc.Via = "SSH key " + e.display(g.accepted) + ", " + rc.Host + " greets " + g.account
			case g.accepted != "":
				// A key from ssh-agent, named by its comment (often a name or an
				// email), which is not shown.
				rc.Via = "a key from ssh-agent, " + rc.Host + " greets " + g.account
			}
		case g.deployKey != "":
			rc.Via += ", a deploy key for " + g.deployKey
		case g.denied != "":
			rc.Via += ", refused: " + g.denied
		default:
			rc.Via += ", the host did not say which account"
		}
	}
}

// judgePush compares a remote's push account with the profile's.
func (c *RepoCheck) judgePush(e Env, vc verifyContext, profile config.Profile, rc RemoteCheck) {
	if rc.Protocol != "https" && rc.Protocol != "http" && rc.Protocol != "ssh" {
		return
	}
	if !strings.EqualFold(rc.Host, profile.Host) {
		c.note("remote %s is on %s, where profile %s does not decide; your own settings apply", rc.Name, rc.Host, profile.ID)
		return
	}
	switch {
	case rc.secretInURL:
		c.problem("remote %s has a token written in its URL (kept in plain text in .git/config); Git pushes with it, "+
			"whichever account it belongs to: `git remote set-url %s https://%s` removes it", rc.Name, rc.Name, rc.Address)
	case rc.PushAs == "" && rc.Greeted:
		c.problem("pushes to remote %s do not sign in as an account (%s); the profile's account is %s", rc.Name, rc.Via, profile.Account)
	case rc.PushAs == "":
		c.unconfirmed("which account remote %s pushes as is not in your settings (%s); `gitpersona verify --ssh` "+
			"asks %s, or give profile %s an ssh_key", rc.Name, rc.Via, rc.Host, profile.ID)
	case strings.EqualFold(rc.PushAs, profile.Account):
	case rc.ChosenBy != "":
		c.note("remote %s pushes as %s, which the repository chooses itself (%s)", rc.Name, rc.PushAs, rc.ChosenBy)
	default:
		c.problem("remote %s pushes as %s, not as the profile's %s (%s)", rc.Name, rc.PushAs, profile.Account, rc.Via)
	}
	if rc.Via == "gh token" && rc.PushAs != "" && !ghHolds(vc, rc.Host, rc.PushAs) {
		c.problem("%s is not signed in to gh on %s, so pushes to remote %s fail; run `gh auth login --hostname %s`",
			rc.PushAs, rc.Host, rc.Name, rc.Host)
	}
	for _, key := range rc.keys[min(1, len(rc.keys)):] {
		if p, ok := vc.keys[key]; ok && strings.EqualFold(p.Host, rc.Host) && !strings.EqualFold(p.Account, rc.PushAs) {
			c.note("if %s refuses the first key, ssh also offers %s, the key of %s", rc.Host, e.display(key), p.Account)
		}
	}
	if owner, ok := otherAccountOwns(vc.cfg, rc); ok {
		c.note("remote %s belongs to your account %s, but pushes go as %s", rc.Name, owner, rc.PushAs)
	}
}

// ghHolds reports whether gh holds account on host; true when unknown, so
// a gh that cannot be asked never raises a problem of its own.
func ghHolds(vc verifyContext, host, account string) bool {
	accounts, known := vc.signedIn[strings.ToLower(host)]
	if !known || accounts == nil {
		return true
	}
	return slices.ContainsFunc(accounts, func(a string) bool { return strings.EqualFold(a, account) })
}

// otherAccountOwns returns the profile account on the remote's host that owns
// the repository, when pushes go as a different account.
func otherAccountOwns(cfg config.Config, rc RemoteCheck) (string, bool) {
	if rc.Owner == "" || rc.PushAs == "" || strings.EqualFold(rc.Owner, rc.PushAs) {
		return "", false
	}
	for _, p := range cfg.Profiles {
		if strings.EqualFold(p.Host, rc.Host) && strings.EqualFold(p.Account, rc.Owner) {
			return p.Account, true
		}
	}
	return "", false
}

// gitPersonaHelper reads the account and provider of a GitPersona credential
// helper line ("!… credential --provider github --host … --account …").
func gitPersonaHelper(helper string) (account, provider string, ours bool) {
	words := discover.ShellWords(strings.TrimPrefix(helper, "!"))
	i := slices.Index(words, "credential")
	if i < 0 {
		return "", "", false
	}
	for j := i + 1; j+1 < len(words); j++ {
		switch words[j] {
		case "--account":
			account = words[j+1]
		case "--provider":
			provider = words[j+1]
		}
	}
	return account, provider, account != "" && provider != ""
}

// identityFlags returns the files an ssh command line names with -i or
// -o IdentityFile=.
func identityFlags(command string) []string {
	var keys []string
	words := discover.ShellWords(command)
	for i, w := range words {
		switch {
		case w == "-i" && i+1 < len(words):
			keys = append(keys, words[i+1])
		case strings.HasPrefix(w, "-i") && len(w) > 2 && i > 0:
			keys = append(keys, w[2:])
		case w == "-o" && i+1 < len(words) && strings.HasPrefix(words[i+1], "IdentityFile="):
			keys = append(keys, strings.TrimPrefix(words[i+1], "IdentityFile="))
		}
	}
	return keys
}

// sharedConnection returns the ControlPath through which ssh would reuse an
// open connection for a push, or "" when the command turns sharing off.
func sharedConnection(command, controlPath string) string {
	words := discover.ShellWords(command)
	for i, w := range words {
		next := ""
		if i+1 < len(words) {
			next = words[i+1]
		}
		switch {
		case w == "-o" && strings.EqualFold(next, "ControlPath=none"),
			w == "-oControlPath=none",
			w == "-S" && next == "none":
			return ""
		}
	}
	if strings.EqualFold(controlPath, "none") {
		return ""
	}
	return controlPath
}

// remoteProtocol is https, http, ssh or other (a local path or file://).
func remoteProtocol(url string) string {
	scheme, _, hasScheme := strings.Cut(url, "://")
	switch {
	case hasScheme && (scheme == "https" || scheme == "http"):
		return scheme
	case hasScheme && (scheme == "ssh" || scheme == "git+ssh" || scheme == "ssh+git"):
		return "ssh"
	case !hasScheme && strings.Contains(url, ":") && !strings.HasPrefix(url, "/"):
		return "ssh"
	default:
		return "other"
	}
}

// remoteAddress is host/path of a remote URL without user names, passwords
// or a trailing .git, safe to show.
func remoteAddress(url string) string {
	var rest string
	if _, after, ok := strings.Cut(url, "://"); ok {
		rest = after
	} else if remoteProtocol(url) == "ssh" {
		rest = strings.Replace(url, ":", "/", 1)
	} else {
		return "local repository"
	}
	host, path, _ := strings.Cut(rest, "/")
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	return host + "/" + strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
}

// profileHost reports whether a profile is for host; --ssh connects only there.
func profileHost(cfg config.Config, host string) bool {
	return slices.ContainsFunc(cfg.Profiles, func(p config.Profile) bool { return strings.EqualFold(p.Host, host) })
}

// urlHasPassword reports whether an http(s) URL carries a password or token
// (https://user:secret@host/...). The secret itself is never kept.
func urlHasPassword(url string) bool {
	_, rest, ok := strings.Cut(url, "://")
	if !ok {
		return false
	}
	hostPart, _, _ := strings.Cut(rest, "/")
	at := strings.LastIndex(hostPart, "@")
	return at >= 0 && strings.Contains(hostPart[:at], ":")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
