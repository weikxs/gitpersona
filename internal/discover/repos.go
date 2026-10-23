package discover

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Repo is what a repository's own .git/config says about who it belongs to.
type Repo struct {
	Dir   string `json:"dir"`
	Host  string `json:"host"`  // the remote's host as written: github.com or an SSH alias
	Owner string `json:"owner"` // the remote's owner (user or organization)
	Email string `json:"email"` // user.email set in the repository itself
	User  string `json:"user"`  // the user name an https remote names, as in https://user@host/...
	// Lone is set for a repository whose folder says nothing about other
	// repositories: it is a scanned folder itself, or sits inside another
	// repository.
	Lone bool `json:"lone"`
	// Partial is set when the scan stopped at its time limit while in the
	// repository's folder, so the folder may hold more repositories.
	Partial bool `json:"partial,omitempty"`
}

// Found is the outcome of Repos.
type Found struct {
	Repos []Repo
	// Complete is false when the scan stopped at its time limit.
	Complete bool
	// TooDeep is set when folders below the depth limit were not looked in.
	TooDeep bool
	// Unfinished are the roots the scan did not finish (it stopped in the
	// first and never reached the others).
	Unfinished []string
}

// Repos finds the repositories under roots and reads, from each .git/config,
// only the origin remote (or the first remote) and user.email.
func Repos(ctx context.Context, roots []string, limits ScanLimits) Found {
	ctx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	var repos []Repo
	tooDeep, stoppedAt := false, ""
	var unfinished []string
	for _, root := range roots {
		if ctx.Err() != nil {
			unfinished = append(unfinished, root)
			continue
		}
		deep, stopped := walkRepos(ctx, root, limits.MaxDepth, func(dir string, lone bool) {
			data, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
			if err != nil {
				return
			}
			url, email := repoConfig(string(data))
			host, owner, user := parseRemote(url)
			repos = append(repos, Repo{Dir: dir, Host: host, Owner: owner, Email: email, User: user, Lone: lone})
		})
		tooDeep = tooDeep || deep
		if stopped != "" {
			stoppedAt = cmp.Or(stoppedAt, stopped)
			unfinished = append(unfinished, root)
		}
	}
	if ctx.Err() == nil && len(unfinished) == 0 {
		return Found{Repos: repos, Complete: true, TooDeep: tooDeep}
	}
	return Found{Repos: markPartial(repos, stoppedAt), TooDeep: tooDeep, Unfinished: unfinished}
}

// markPartial marks the repositories in the folders the scan was still
// going through when it stopped at stoppedAt: those may hold more.
func markPartial(repos []Repo, stoppedAt string) []Repo {
	out := slices.Clone(repos)
	for i, r := range out {
		parent := filepath.Dir(r.Dir)
		out[i].Partial = stoppedAt != "" && (stoppedAt == parent || strings.HasPrefix(stoppedAt, parent+string(filepath.Separator)))
	}
	return out
}

// repoConfig returns the origin remote's URL (or the first remote's) and
// user.email from the text of a .git/config.
func repoConfig(text string) (url, email string) {
	section, first, origin := "", "", ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.Trim(strings.TrimSpace(value), `"`)
		switch {
		case key == "url" && strings.HasPrefix(section, "remote "):
			if first == "" {
				first = value
			}
			if section == `remote "origin"` && origin == "" {
				origin = value
			}
		case key == "email" && section == "user":
			email = value
		}
	}
	if origin != "" {
		return origin, email
	}
	return first, email
}

// hostPort returns the port of host:port, or "".
func hostPort(h string) string {
	_, port, _ := strings.Cut(h, ":")
	return port
}

// parseRemote returns the host (possibly an SSH alias) and the owner of a
// remote URL, and the user name of an http(s) URL, or empty strings for
// local paths. The "git" of git@host is the SSH login, not an account. An
// http(s) host keeps a port other than the default, which Git's credential
// settings need.
func parseRemote(url string) (host, owner, user string) {
	var path string
	keepPort := false
	switch {
	case strings.HasPrefix(url, "file://"):
		return "", "", ""
	case strings.Contains(url, "://"):
		scheme, rest, _ := strings.Cut(url, "://")
		h, p, ok := strings.Cut(rest, "/")
		if !ok {
			return "", "", ""
		}
		if at := strings.LastIndex(h, "@"); at >= 0 {
			if strings.HasPrefix(scheme, "http") {
				user, _, _ = strings.Cut(h[:at], ":") // never keep a password
			}
			h = h[at+1:]
		}
		host, path = h, p
		if port := hostPort(h); strings.HasPrefix(scheme, "http") && port != "443" && port != "80" {
			keepPort = port != ""
		}
	case strings.Contains(url, ":") && !strings.HasPrefix(url, "/"):
		h, p, _ := strings.Cut(url, ":")
		if at := strings.LastIndex(h, "@"); at >= 0 {
			h = h[at+1:]
		}
		host, path = h, p
	default:
		return "", "", ""
	}
	if h, _, ok := strings.Cut(host, ":"); ok && !keepPort {
		host = h // a port
	}
	owner, _, _ = strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if host == "" || owner == "" {
		return "", "", ""
	}
	return host, owner, user
}
