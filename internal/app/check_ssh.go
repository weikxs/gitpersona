package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
)

const (
	sshCheckTimeout = 20 * time.Second
	// sshWaitDelay bounds the wait for output after ssh ends, should a
	// ProxyCommand it started keep the output open.
	sshWaitDelay = 2 * time.Second
)

// welcomes find the account in a host's SSH welcome message.
var welcomes = []*regexp.Regexp{
	regexp.MustCompile(`Hi ([A-Za-z0-9_-]+)! You've successfully authenticated`), // GitHub (EMU logins have _)
	regexp.MustCompile(`Welcome to GitLab, @([A-Za-z0-9_.-]+)!`),                 // GitLab
	regexp.MustCompile(`logged in as ([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*)`),     // Bitbucket ("as demo.")
}

// deployKey finds GitHub's welcome for a deploy key, which names a
// repository instead of an account.
var deployKey = regexp.MustCompile(`Hi ([A-Za-z0-9_-]+/[A-Za-z0-9._-]+)! You've successfully authenticated`)

// signedIn finds a greeting that says the key signed in, without naming
// the account.
var signedIn = regexp.MustCompile(`(?i)(successfully authenticated|authenticated via|welcome to gitlab)`)

// permissionDenied finds ssh's refusal, whatever banner came before it.
var permissionDenied = regexp.MustCompile(`Permission denied \(`)

// authenticatedTo finds ssh's own word that the key signed in.
var authenticatedTo = regexp.MustCompile(`(?m)^Authenticated to .* using "publickey"`)

// sshErrorStatus is the exit status of ssh itself failing.
const sshErrorStatus = 255

// deniedLine returns the line that says why ssh failed, or the first one.
func deniedLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if permissionDenied.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return firstLine(text)
}

// acceptedKey finds, in ssh -v output, the key the host accepted.
var acceptedKey = regexp.MustCompile(`(?m)^debug1: Server accepts key: (.+?) [A-Z0-9-]+ SHA256:`)

// CheckSSHOptions selects the profile to check; empty means every profile
// with an SSH key.
type CheckSSHOptions struct {
	Profile string
}

// CheckSSH connects, only when the user asks, to each profile's host with
// the profile's SSH key and compares the account the host greets with the
// profile's. ssh uses the key, as for a push; GitPersona never reads it.
func CheckSSH(ctx context.Context, e Env, opts CheckSSHOptions) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	if opts.Profile != "" {
		if err := knownProfile(cfg, opts.Profile, "check-ssh --profile"); err != nil {
			return err
		}
	}
	checked := 0
	results := map[sshResult]int{}
	for _, p := range cfg.Profiles {
		if (opts.Profile != "" && p.ID != opts.Profile) || p.SSHKey == "" {
			continue
		}
		checked++
		results[e.checkProfileSSH(ctx, p)]++
	}
	switch {
	case opts.Profile != "" && checked == 0:
		return fmt.Errorf("profile %s has no ssh_key; Git uses your own SSH settings there", opts.Profile)
	case checked == 0:
		fmt.Fprintln(e.Stdout, "No profile has an ssh_key; Git uses your own SSH settings.")
	}
	var problems []string
	if n := results[sshWrongAccount]; n > 0 {
		problems = append(problems, fmt.Sprintf("%d signed in as another account", n))
	}
	if n := results[sshFailed]; n > 0 {
		problems = append(problems, fmt.Sprintf("%d did not sign in", n))
	}
	if len(problems) > 0 {
		return fmt.Errorf("of %d SSH keys, %s", checked, strings.Join(problems, " and "))
	}
	return nil
}

// sshResult is the outcome of checking one profile's SSH key.
type sshResult int

const (
	sshOK           sshResult = iota
	sshWrongAccount           // signed in, as another account (or with another key)
	sshFailed                 // did not sign in
)

// checkProfileSSH checks one profile and prints the result.
func (e Env) checkProfileSSH(ctx context.Context, p config.Profile) sshResult {
	key, err := paths.Normalize(p.SSHKey, e.Home)
	if err != nil {
		fmt.Fprintf(e.Stdout, "✗ %s: ssh_key %s: %v\n", p.ID, p.SSHKey, err)
		return sshFailed
	}
	if !keyFileExists(key, e.Home, false) { // only checks that it exists
		fmt.Fprintf(e.Stdout, "✗ %s: ssh_key %s does not exist\n", p.ID, e.display(key))
		return sshFailed
	}
	// The profile's port is the HTTPS one; ssh takes its port from the
	// user's SSH settings.
	host, _ := splitHostPort(p.Host)
	fmt.Fprintf(e.Stdout, "Connecting to %s with %s for profile %s…\n", host, e.display(key), p.ID)
	g := e.sshGreet(ctx, []string{"-o", "IdentitiesOnly=yes", "-i", key}, host)
	switch {
	case g.missing:
		fmt.Fprintln(e.Stdout, "✗ ssh not found")
		return sshFailed
	case g.denied != "":
		fmt.Fprintf(e.Stdout, "✗ %s: the key did not sign in to %s: %s\n", p.ID, host, g.denied)
		return sshFailed
	}
	if g.accepted != "" && !sameFile(g.accepted, key) {
		fmt.Fprintf(e.Stdout, "✗ %s: %s did not accept %s; ssh signed in with %s from your SSH settings instead\n",
			p.ID, host, e.display(key), e.display(g.accepted))
		return sshWrongAccount
	}
	if g.deployKey != "" {
		fmt.Fprintf(e.Stdout, "✗ %s: the key is a deploy key for %s, not the key of account %s\n", p.ID, g.deployKey, p.Account)
		return sshWrongAccount
	}
	switch {
	case g.account == "" && g.authenticated:
		fmt.Fprintf(e.Stdout, "? %s: the key signs in to %s, but the host does not say as which account\n", p.ID, host)
		return sshOK
	case g.account == "":
		fmt.Fprintf(e.Stdout, "✗ %s: the key did not sign in to %s: %s\n", p.ID, host, g.first)
		return sshFailed
	case !strings.EqualFold(g.account, p.Account):
		fmt.Fprintf(e.Stdout, "✗ %s: the key signs in to %s as %s, not %s; Git pushes as %s with it\n", p.ID, host, g.account, p.Account, g.account)
		return sshWrongAccount
	}
	fmt.Fprintf(e.Stdout, "✓ %s: the key signs in to %s as %s\n", p.ID, host, g.account)
	return sshOK
}

// sshGreeting is what a host said when ssh signed in, as for a push.
type sshGreeting struct {
	account       string // the account the host greets
	deployKey     string // the repository a deploy key is for
	accepted      string // the key file the host accepted, from ssh -v
	authenticated bool   // the key signed in
	denied        string // why it did not sign in, or ""
	first         string // the first line ssh printed
	missing       bool   // ssh is not installed
}

// sshGreet connects to git@target with the extra ssh arguments and reads the
// host's greeting. BatchMode: never ask for a passphrase or to trust a new
// host here. -v tells which key the host accepted: ssh also offers the keys
// ~/.ssh/config names for the host, and -i does not replace them. ssh uses
// the key, as for a push; GitPersona never reads it.
func (e Env) sshGreet(ctx context.Context, args []string, target string) sshGreeting {
	ctx, cancel := context.WithTimeout(ctx, sshCheckTimeout)
	defer cancel()
	ssh, err := e.LookPath("ssh")
	if err != nil {
		return sshGreeting{missing: true}
	}
	argv := append([]string{"-T", "-v", "-o", "BatchMode=yes"}, args...)
	cmd := exec.CommandContext(ctx, ssh, append(argv, "git@"+target)...)
	cmd.WaitDelay = sshWaitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	// Hosts end the session with a non-zero status after greeting; 255 is
	// ssh's own failure (refused key, no connection).
	runErr := cmd.Run()
	var exit *exec.ExitError
	sshError := errors.As(runErr, &exit) && exit.ExitCode() == sshErrorStatus
	text := withoutDebug(out.String())
	g := sshGreeting{first: firstLine(text)}
	// A refusal decides; then signing in (a greeting, or ssh saying so)
	// does, even when the host closes without an exit status (255); only
	// then is 255 a failure, whose reason ssh prints last.
	g.authenticated = authenticatedTo.MatchString(text) || signedIn.MatchString(text) ||
		slices.ContainsFunc(welcomes, func(w *regexp.Regexp) bool { return w.MatchString(text) })
	switch {
	case permissionDenied.MatchString(text):
		g.denied = deniedLine(text)
		return g
	case sshError && !g.authenticated:
		g.denied = lastLine(text)
		return g
	}
	if m := acceptedKey.FindStringSubmatch(out.String()); m != nil {
		g.accepted = m[1]
	}
	if m := deployKey.FindStringSubmatch(text); m != nil {
		g.deployKey = m[1]
		return g
	}
	for _, w := range welcomes {
		if m := w.FindStringSubmatch(text); m != nil {
			g.account = m[1]
			break
		}
	}
	return g
}

// withoutDebug drops the debug lines of ssh -v, and its notice about a
// new host key, from its output.
func withoutDebug(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "debug") && !strings.HasPrefix(line, "OpenSSH_") && !strings.HasPrefix(line, "Warning: Permanently added") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// sameFile reports whether two key paths are the same file, by name or
// through symlinks; it only resolves paths.
func sameFile(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || paths.ResolveSymlinks(a) == paths.ResolveSymlinks(b)
}

// lastLine returns the last non-empty line of s, or a placeholder.
func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" && !authenticatedTo.MatchString(line) {
			return line
		}
	}
	return "no answer"
}

// firstLine returns the first non-empty line of s, or a placeholder.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "no answer"
}
