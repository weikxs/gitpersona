package discover

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// SSHAlias is an SSH host alias that connects to github.com, such as
// "github-work" in a remote like git@github-work:owner/repo.git.
type SSHAlias struct {
	Alias        string `json:"alias"`
	IdentityFile string `json:"identity_file,omitempty"` // as written, often ~/.ssh/...
}

const (
	// maxIncludeDepth bounds nested Include directives, as ssh does.
	maxIncludeDepth = 16
	// maxLine bounds a configuration line.
	maxLine = 1 << 20
)

// SSHAliases reads an SSH client configuration (only that file and the
// configuration files it includes; never keys) and returns, in order, the
// hosts without wildcards that connect to github.com, with their first
// IdentityFile. Like ssh, the first value of each setting wins.
func SSHAliases(path, home string) []SSHAlias {
	p := sshParser{home: home, sshDir: filepath.Dir(path), visited: map[string]bool{}, hosts: map[string]*sshHost{}}
	p.read(path, 0)
	var aliases []SSHAlias
	for _, name := range p.order {
		h := p.hosts[name]
		target := h.hostName
		if target == "" {
			target = name // without HostName, ssh connects to the alias itself
		}
		if strings.EqualFold(target, "github.com") {
			aliases = append(aliases, SSHAlias{Alias: name, IdentityFile: h.keyFile})
		}
	}
	return aliases
}

// sshHost is what the configuration says about one host name.
type sshHost struct {
	hostName    string
	keyFile     string
	hostNameSet bool
	keyFileSet  bool
}

type sshParser struct {
	home, sshDir string
	visited      map[string]bool
	hosts        map[string]*sshHost
	order        []string
	// The block being read: the host names it applies to, or a Match block.
	current []string
	inMatch bool
}

func (p *sshParser) read(path string, depth int) {
	if depth > maxIncludeDepth || p.visited[path] || looksLikeKey(filepath.Base(path)) {
		return
	}
	p.visited[path] = true
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	lines := bufio.NewScanner(f)
	lines.Buffer(make([]byte, 0, 64<<10), maxLine)
	for lines.Scan() {
		p.line(lines.Text(), depth)
	}
}

func (p *sshParser) line(text string, depth int) {
	key, value := sshKeyword(text)
	switch key {
	case "host":
		p.current, p.inMatch = p.names(value), false
	case "match":
		p.current, p.inMatch = nil, true
	case "include":
		if p.inMatch {
			return // only applies when the Match condition holds; not evaluated
		}
		// Blocks inside an included file end with it, as in ssh.
		current := p.current
		p.include(value, depth)
		p.current, p.inMatch = current, false
	case "hostname":
		for _, h := range p.current {
			if host := p.hosts[h]; !host.hostNameSet {
				host.hostName, host.hostNameSet = value, true
			}
		}
	case "identityfile":
		for _, h := range p.current {
			if host := p.hosts[h]; !host.keyFileSet {
				host.keyFileSet = true
				if !strings.Contains(value, "%") { // %-tokens depend on the connection
					host.keyFile = value
				}
			}
		}
	}
}

// names returns the plain host names of a Host line, registering new ones;
// patterns with wildcards or negations are not aliases.
func (p *sshParser) names(value string) []string {
	var names []string
	for _, h := range strings.Fields(value) {
		if strings.ContainsAny(h, "*?!") {
			continue
		}
		if p.hosts[h] == nil {
			p.hosts[h] = &sshHost{}
			p.order = append(p.order, h)
		}
		names = append(names, h)
	}
	return names
}

// include reads the files an Include directive names; relative paths are
// under ~/.ssh, as ssh resolves them. A wildcard that matches files directly
// in ~/.ssh, where keys live, only opens files named like configuration.
func (p *sshParser) include(value string, depth int) {
	for _, pattern := range strings.Fields(value) {
		if rest, ok := strings.CutPrefix(pattern, "~/"); ok {
			pattern = filepath.Join(p.home, rest)
		} else if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(p.sshDir, pattern)
		}
		matches, _ := filepath.Glob(pattern)
		wildcard := strings.ContainsAny(pattern, "*?[")
		for _, m := range matches {
			if wildcard && filepath.Dir(m) == p.sshDir && !strings.Contains(filepath.Base(m), "config") {
				continue
			}
			if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() {
				p.read(m, depth+1)
			}
		}
	}
}

// looksLikeKey reports whether a file name is one ssh uses for keys or host
// lists, which GitPersona never opens.
func looksLikeKey(name string) bool {
	return strings.HasPrefix(name, "id_") || strings.HasSuffix(name, ".pub") ||
		strings.HasPrefix(name, "known_hosts") || strings.HasPrefix(name, "authorized_keys")
}

// sshKeyword splits a configuration line into its lowercase keyword and its
// value, which may follow a space or "=" and may be quoted.
func sshKeyword(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", ""
	}
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return strings.ToLower(line), ""
	}
	value := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line[i:]), "="))
	return strings.ToLower(line[:i]), strings.Trim(value, `"`)
}
