package app

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/paths"
)

// sshHost is what ssh's configuration says about one host or alias.
type sshHost struct {
	hostname    string
	files       []string // identity files that exist, as real paths, in ssh's order
	controlPath string   // where ssh shares connections, or "none"
}

// sshResolver asks `ssh -G` about each host once. ssh -G only evaluates the
// configuration: it connects nowhere and opens no key.
type sshResolver struct {
	env   Env
	mu    sync.Mutex
	hosts map[string]sshHost
}

func newSSHResolver(e Env) *sshResolver {
	return &sshResolver{env: e, hosts: map[string]sshHost{}}
}

func (s *sshResolver) resolve(ctx context.Context, alias string) sshHost {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.hosts[alias]; ok {
		return h
	}
	h := sshHost{hostname: alias, controlPath: "none"}
	out, err := s.env.Run(ctx, "ssh", "-G", "--", alias)
	if err == nil {
		h = parseSSHConfig(string(out), alias)
		h.files = s.env.dedupeKeys(h.files)
	}
	s.hosts[alias] = h
	return h
}

// parseSSHConfig reads the hostname and identity files from `ssh -G` output.
func parseSSHConfig(out, alias string) sshHost {
	h := sshHost{hostname: alias, controlPath: "none"}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "hostname":
			h.hostname = strings.TrimSpace(value)
		case "identityfile":
			h.files = append(h.files, strings.TrimSpace(value))
		case "controlpath":
			h.controlPath = strings.TrimSpace(value)
		}
	}
	return h
}

// dedupeKeys turns key paths into real paths of files that exist, dropping
// repeats and keeping the order. It only looks whether a file is there.
func (e Env) dedupeKeys(keys []string) []string {
	var out []string
	for _, k := range keys {
		norm, err := paths.Normalize(k, e.Home)
		if err != nil {
			continue
		}
		if info, err := os.Stat(norm); err != nil || info.IsDir() {
			continue
		}
		resolved := paths.ResolveSymlinks(norm)
		if !slices.Contains(out, resolved) {
			out = append(out, resolved)
		}
	}
	return out
}

// greeter asks each SSH host once per alias and ssh command which account a
// push signs in as.
type greeter struct {
	env    Env
	mu     sync.Mutex
	asked  map[string]*sync.Once
	answer map[string]sshGreeting
}

func newGreeter(e Env) *greeter {
	return &greeter{env: e, asked: map[string]*sync.Once{}, answer: map[string]sshGreeting{}}
}

// ask connects like a push to alias with command (core.sshCommand or
// GIT_SSH_COMMAND): the same -i keys and options; ssh adds the host's own
// settings from ~/.ssh/config, as it does for Git.
func (g *greeter) ask(ctx context.Context, alias, command string) sshGreeting {
	key := alias + "\x00" + command
	g.mu.Lock()
	once, ok := g.asked[key]
	if !ok {
		once = &sync.Once{}
		g.asked[key] = once
	}
	g.mu.Unlock()
	once.Do(func() {
		// Never through a shared connection: it would answer for the key
		// that opened it, not the one this push names.
		args := append([]string{"-o", "ControlPath=none"}, sshCommandArgs(command)...)
		answer := g.env.sshGreet(ctx, args, alias)
		g.mu.Lock()
		g.answer[key] = answer
		g.mu.Unlock()
	})
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.answer[key]
}

// sshCommandArgs keeps the options of an ssh command line ("ssh -i key -o
// IdentitiesOnly=yes"): the program itself and anything after a host are
// not passed on.
func sshCommandArgs(command string) []string {
	words := discover.ShellWords(command)
	if len(words) == 0 {
		return nil
	}
	var args []string
	for i := 1; i < len(words); i++ {
		w := words[i]
		if !strings.HasPrefix(w, "-") {
			break
		}
		args = append(args, w)
		if (w == "-i" || w == "-o" || w == "-F" || w == "-p" || w == "-l") && i+1 < len(words) {
			i++
			args = append(args, words[i])
		}
	}
	return args
}

// flagSharedConnections marks the SSH remotes whose pushes can reuse a
// connection that another key opened: ssh ignores -i on a shared connection
// (ControlMaster), so the push goes out as that key's account.
func (e Env) flagSharedConnections(checks []RepoCheck) {
	firstKeys := map[string][]string{} // ControlPath -> the distinct keys pushes start with
	for _, c := range checks {
		for _, rc := range c.Remotes {
			if rc.sharedPath != "" && len(rc.keys) > 0 && !slices.Contains(firstKeys[rc.sharedPath], rc.keys[0]) {
				firstKeys[rc.sharedPath] = append(firstKeys[rc.sharedPath], rc.keys[0])
			}
		}
	}
	for i := range checks {
		c := &checks[i]
		for _, rc := range c.Remotes {
			keys := firstKeys[rc.sharedPath]
			if rc.sharedPath == "" || len(keys) < 2 || len(rc.keys) == 0 {
				continue
			}
			var others []string
			for _, k := range keys {
				if k != rc.keys[0] {
					others = append(others, e.display(k))
				}
			}
			c.problem(e.tr("remote %s: ssh shares one connection to %s (ControlMaster, ControlPath %s) with pushes that use %s; "+
				"a push here can go out over it, signed in with that key instead of %s. Set `ControlMaster no` for %s in "+
				"~/.ssh/config, or give the profile an ssh_key (GitPersona's SSH command never shares a connection)",
				rc.Name, rc.Host, rc.sharedPath, strings.Join(others, ", "), e.display(rc.keys[0]), rc.Host))
		}
		c.Verdict = c.verdict(c.Profile != "")
	}
}
