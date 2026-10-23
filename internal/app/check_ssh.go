package app

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
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
	regexp.MustCompile(`Hi ([A-Za-z0-9-]+)! You've successfully authenticated`), // GitHub
	regexp.MustCompile(`Welcome to GitLab, @([A-Za-z0-9_.-]+)!`),                // GitLab
	regexp.MustCompile(`logged in as ([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*)`),    // Bitbucket ("as demo.")
}

// deployKey finds GitHub's welcome for a deploy key, which names a
// repository instead of an account.
var deployKey = regexp.MustCompile(`Hi ([A-Za-z0-9-]+/[A-Za-z0-9._-]+)! You've successfully authenticated`)

// signedIn finds a greeting that says the key signed in, without naming
// the account.
var signedIn = regexp.MustCompile(`(?i)(successfully authenticated|authenticated via|welcome to gitlab)`)

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
		return fmt.Errorf("profile %q has no ssh_key, or does not exist", opts.Profile)
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
	host, _, _ := strings.Cut(p.Host, ":")
	fmt.Fprintf(e.Stdout, "Connecting to %s with %s for profile %s…\n", host, e.display(key), p.ID)
	ctx, cancel := context.WithTimeout(ctx, sshCheckTimeout)
	defer cancel()
	ssh, err := e.LookPath("ssh")
	if err != nil {
		fmt.Fprintln(e.Stdout, "✗ ssh not found")
		return sshFailed
	}
	// BatchMode: never ask for a passphrase or to trust a new host here.
	// -v tells which key the host accepted: ssh also offers the keys
	// ~/.ssh/config names for the host, and -i does not replace them.
	cmd := exec.CommandContext(ctx, ssh, "-T", "-v", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-i", key, "git@"+host)
	cmd.WaitDelay = sshWaitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // hosts end the session with a non-zero status after greeting
	text := withoutDebug(out.String())
	if m := acceptedKey.FindStringSubmatch(out.String()); m != nil && !sameFile(m[1], key) {
		fmt.Fprintf(e.Stdout, "✗ %s: %s did not accept %s; ssh signed in with %s from your SSH settings instead\n",
			p.ID, host, e.display(key), e.display(m[1]))
		return sshWrongAccount
	}
	if m := deployKey.FindStringSubmatch(text); m != nil {
		fmt.Fprintf(e.Stdout, "✗ %s: the key is a deploy key for %s, not the key of account %s\n", p.ID, m[1], p.Account)
		return sshWrongAccount
	}
	account := ""
	for _, w := range welcomes {
		if m := w.FindStringSubmatch(text); m != nil {
			account = m[1]
			break
		}
	}
	switch {
	case account == "" && signedIn.MatchString(text):
		fmt.Fprintf(e.Stdout, "? %s: the key signs in to %s, but the host does not say as which account\n", p.ID, host)
		return sshOK
	case account == "":
		fmt.Fprintf(e.Stdout, "✗ %s: the key did not sign in to %s: %s\n", p.ID, host, firstLine(text))
		return sshFailed
	case !strings.EqualFold(account, p.Account):
		fmt.Fprintf(e.Stdout, "✗ %s: the key signs in to %s as %s, not %s; Git pushes as %s with it\n", p.ID, host, account, p.Account, account)
		return sshWrongAccount
	}
	fmt.Fprintf(e.Stdout, "✓ %s: the key signs in to %s as %s\n", p.ID, host, account)
	return sshOK
}

// withoutDebug drops the debug lines of ssh -v from its output.
func withoutDebug(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "debug") && !strings.HasPrefix(line, "OpenSSH_") {
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

// firstLine returns the first non-empty line of s, or a placeholder.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "no answer"
}
