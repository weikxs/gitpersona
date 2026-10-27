package app

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
)

// How a push signs in (RemoteCheck.ViaKind) and where the repository chose
// the push account (RemoteCheck.ChosenKind). These codes are stable; the
// matching texts are for people and depend on the language.
const (
	ViaGH         = "gh"         // gh's token, through GitPersona's helper
	ViaGitPersona = "gitpersona" // GitPersona's helper for another host
	ViaHelper     = "helper"     // the user's own credential helper
	ViaURLToken   = "url-token"  // a token written in the remote URL
	ViaSSHKey     = "ssh-key"    // an SSH key file
	ViaSSHAgent   = "ssh-agent"  // ssh-agent or ssh's default keys

	ChosenURLSecret      = "url-secret"       // the URL carries the token
	ChosenURLUser        = "url-user"         // the URL names the account
	ChosenRepoUsername   = "repo-username"    // credential.username in the repository
	ChosenEnvSSHCommand  = "env-ssh-command"  // GIT_SSH_COMMAND
	ChosenRepoSSHCommand = "repo-ssh-command" // core.sshCommand in the repository
)

// remote is one remote's push URL, as Git uses it (insteadOf applied).
type remote struct {
	name string
	url  string
}

// remotesOf lists a repository's remotes with the URL a push goes to, from
// one `git remote -v`: its push lines are what `git remote get-url --push`
// answers (pushurl and pushInsteadOf applied), the first one per remote.
func (e Env) remotesOf(ctx context.Context, dir string) []remote {
	lines, err := e.git(ctx, dir, "remote", "-v")
	if err != nil || lines == "" {
		return nil
	}
	var out []remote
	seen := map[string]bool{}
	for _, line := range strings.Split(lines, "\n") {
		name, rest, ok := strings.Cut(line, "\t")
		url, ok2 := strings.CutSuffix(strings.TrimSpace(rest), " (push)")
		if !ok || !ok2 || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, remote{name: name, url: strings.TrimSpace(url)})
	}
	return out
}

// checkRemote works out which account a push to one remote authenticates as.
func (e Env) checkRemote(ctx context.Context, vc verifyContext, dir string, repo repoState, r remote) RemoteCheck {
	host, owner, user := discover.RemoteParts(r.url)
	rc := RemoteCheck{Name: r.name, Address: remoteAddress(r.url), Protocol: remoteProtocol(r.url), Host: host,
		Owner: owner, Keys: []string{}}
	if rc.Address == localRepository {
		rc.Address = e.tr("local repository")
	}
	switch rc.Protocol {
	case "https", "http":
		e.httpsPush(ctx, dir, r.url, user, &rc)
	case "ssh":
		e.sshPush(ctx, vc, repo, host, &rc)
	}
	return rc
}

// httpsPush: the user name Git sends is the URL's, else credential.username;
// GitPersona's helper answers for that account, or the profile's.
func (e Env) httpsPush(ctx context.Context, dir, url, user string, rc *RemoteCheck) {
	matched := e.credentialSettings(ctx, dir, url)
	helper, named := matched["credential.helper"], matched["credential.username"]
	account, provider, ours := gitPersonaHelper(helper)
	if urlHasPassword(url) {
		// Git sends the URL's password and asks no helper: the token decides.
		rc.Via, rc.ViaKind = e.tr("a token written in the remote URL"), ViaURLToken
		rc.ChosenBy, rc.ChosenKind, rc.secretInURL = e.tr("the remote URL"), ChosenURLSecret, true
		return
	}
	rc.PushAs = firstNonEmpty(user, named, account)
	switch {
	case user != "":
		rc.ChosenBy, rc.ChosenKind = e.tr("the remote URL names it"), ChosenURLUser
	case named != "" && e.localCredentialUsername(ctx, dir, url) != "":
		rc.ChosenBy, rc.ChosenKind = e.tr("credential.username in the repository"), ChosenRepoUsername
	}
	switch {
	case ours && provider == config.ProviderGitHub:
		rc.Via, rc.ViaKind = e.tr("gh token"), ViaGH
	case ours:
		rc.Via, rc.ViaKind = e.tr("GitPersona"), ViaGitPersona
	default:
		rc.Via, rc.ViaKind = e.tr("your credential helper"), ViaHelper
	}
}

// credentialSettings returns the credential.* settings Git uses for url, from
// one `git config --get-urlmatch credential <url>` (key → value).
func (e Env) credentialSettings(ctx context.Context, dir, url string) map[string]string {
	out, _ := e.git(ctx, dir, "config", "--get-urlmatch", "credential", url)
	settings := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(line, " "); ok {
			settings[normalKey(key)] = value
		}
	}
	return settings
}

// localCredentialUsername is credential.username for url from the
// repository's own config file only.
func (e Env) localCredentialUsername(ctx context.Context, dir, url string) string {
	local, _ := e.git(ctx, dir, "config", "--local", "--get-urlmatch", "credential.username", url)
	return local
}

// sshPush: the account is the one whose key ssh offers first.
func (e Env) sshPush(ctx context.Context, vc verifyContext, repo repoState, alias string, rc *RemoteCheck) {
	resolved := vc.ssh.resolve(ctx, alias)
	rc.Host = resolved.hostname
	command := os.Getenv("GIT_SSH_COMMAND")
	switch {
	case command != "":
		rc.ChosenBy, rc.ChosenKind = "GIT_SSH_COMMAND", ChosenEnvSSHCommand
	default:
		command = repo.config.get("core.sshCommand")
		if local := repo.config.getLocal("core.sshCommand", repo.root, repo.gitDir); local != "" {
			rc.ChosenBy, rc.ChosenKind = e.tr("core.sshCommand in the repository"), ChosenRepoSSHCommand
		}
	}
	keys := e.dedupeKeys(append(identityFlags(command), resolved.files...))
	rc.keys = keys
	rc.sharedPath = sharedConnection(command, resolved.controlPath)
	for _, k := range keys {
		rc.Keys = append(rc.Keys, e.display(k))
	}
	if len(keys) == 0 {
		rc.Via, rc.ViaKind = e.tr("ssh-agent or ssh's default keys"), ViaSSHAgent
		return
	}
	rc.ViaKind = ViaSSHKey
	rc.Via = e.tr("SSH key %s", e.display(keys[0]))
	if p, ok := vc.keys[keys[0]]; ok && strings.EqualFold(p.Host, rc.Host) {
		rc.PushAs = p.Account
	}
	if vc.greet != nil && profileHost(vc.cfg, rc.Host) {
		g := vc.greet.ask(ctx, alias, command)
		rc.Greeted, rc.PushAs = true, g.account
		switch {
		case g.account != "":
			rc.Via = e.tr("SSH key %s, %s greets %s", e.display(keys[0]), rc.Host, g.account)
			switch {
			case strings.Contains(g.accepted, "/"):
				rc.Via = e.tr("SSH key %s, %s greets %s", e.display(g.accepted), rc.Host, g.account)
			case g.accepted != "":
				// A key from ssh-agent, named by its comment (often a name or an
				// email), which is not shown.
				rc.Via, rc.ViaKind = e.tr("a key from ssh-agent, %s greets %s", rc.Host, g.account), ViaSSHAgent
			}
		case g.deployKey != "":
			rc.Via = e.tr("SSH key %s, a deploy key for %s", e.display(keys[0]), g.deployKey)
		case g.denied != "":
			rc.Via = e.tr("SSH key %s, refused: %s", e.display(keys[0]), e.shownLine(g.denied))
		default:
			rc.Via = e.tr("SSH key %s, the host did not say which account", e.display(keys[0]))
		}
	}
}

// judgePush compares a remote's push account with the profile's.
func (c *RepoCheck) judgePush(e Env, vc verifyContext, profile config.Profile, rc RemoteCheck) {
	if rc.Protocol != "https" && rc.Protocol != "http" && rc.Protocol != "ssh" {
		return
	}
	if !strings.EqualFold(rc.Host, profile.Host) {
		c.note(e.tr("remote %s is on %s, where profile %s does not decide; your own settings apply", rc.Name, rc.Host, profile.ID))
		return
	}
	switch {
	case rc.secretInURL:
		c.problem(e.tr("remote %s has a token written in its URL (kept in plain text in .git/config); Git pushes with it, "+
			"whichever account it belongs to: `git remote set-url %s https://%s` removes it", rc.Name, rc.Name, rc.Address))
	case rc.PushAs == "" && rc.Greeted:
		c.problem(e.tr("pushes to remote %s do not sign in as an account (%s); the profile's account is %s", rc.Name, rc.Via, profile.Account))
	case rc.PushAs == "":
		c.unconfirmed(e.tr("which account remote %s pushes as is not in your settings (%s); `gitpersona verify --ssh` "+
			"asks %s, or give profile %s an ssh_key", rc.Name, rc.Via, rc.Host, profile.ID))
	case strings.EqualFold(rc.PushAs, profile.Account):
	case rc.ChosenKind != "":
		c.note(e.tr("remote %s pushes as %s, which the repository chooses itself (%s)", rc.Name, rc.PushAs, rc.ChosenBy))
	default:
		c.problem(e.tr("remote %s pushes as %s, not as the profile's %s (%s)", rc.Name, rc.PushAs, profile.Account, rc.Via))
	}
	if rc.ViaKind == ViaGH && rc.PushAs != "" && !ghHolds(vc, rc.Host, rc.PushAs) {
		c.problem(e.tr("%s is not signed in to gh on %s, so pushes to remote %s fail; run `gh auth login --hostname %s`",
			rc.PushAs, rc.Host, rc.Name, rc.Host))
	}
	for _, key := range rc.keys[min(1, len(rc.keys)):] {
		if p, ok := vc.keys[key]; ok && strings.EqualFold(p.Host, rc.Host) && !strings.EqualFold(p.Account, rc.PushAs) {
			c.note(e.tr("if %s refuses the first key, ssh also offers %s, the key of %s", rc.Host, e.display(key), p.Account))
		}
	}
	if owner, ok := otherAccountOwns(vc.cfg, rc); ok {
		c.note(e.tr("remote %s belongs to your account %s, but pushes go as %s", rc.Name, owner, rc.PushAs))
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

// localRepository is the address of a remote that is a path on this computer;
// checkRemote shows it in the language of the request.
const localRepository = "local repository"

// remoteAddress is host/path of a remote URL without user names, passwords
// or a trailing .git, safe to show.
func remoteAddress(url string) string {
	var rest string
	if _, after, ok := strings.Cut(url, "://"); ok {
		rest = after
	} else if remoteProtocol(url) == "ssh" {
		rest = strings.Replace(url, ":", "/", 1)
	} else {
		return localRepository
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
