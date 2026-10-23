package discover

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Repo is what a repository's own .git/config says about who it belongs to.
type Repo struct {
	Dir   string `json:"dir"`
	Host  string `json:"host"`  // the remote's host as written: github.com or an SSH alias
	Owner string `json:"owner"` // the remote's owner (user or organization)
	Email string `json:"email"` // user.email set in the repository itself
}

// Repos finds the repositories under roots and reads, from each .git/config,
// only the origin remote (or the first remote) and user.email.
func Repos(ctx context.Context, roots []string, limits ScanLimits) []Repo {
	ctx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	var repos []Repo
	for _, root := range roots {
		walkRepos(ctx, root, limits.MaxDepth, func(dir string) {
			data, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
			if err != nil {
				return
			}
			url, email := repoConfig(string(data))
			host, owner := parseRemote(url)
			repos = append(repos, Repo{Dir: dir, Host: host, Owner: owner, Email: email})
		})
	}
	return repos
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

// parseRemote returns the host (possibly an SSH alias) and the owner of a
// remote URL, or empty strings for local paths.
func parseRemote(url string) (host, owner string) {
	var path string
	switch {
	case strings.HasPrefix(url, "file://"):
		return "", ""
	case strings.Contains(url, "://"):
		rest := url[strings.Index(url, "://")+3:]
		h, p, ok := strings.Cut(rest, "/")
		if !ok {
			return "", ""
		}
		if at := strings.LastIndex(h, "@"); at >= 0 {
			h = h[at+1:]
		}
		host, path = h, p
	case strings.Contains(url, ":") && !strings.HasPrefix(url, "/"):
		h, p, _ := strings.Cut(url, ":")
		if at := strings.LastIndex(h, "@"); at >= 0 {
			h = h[at+1:]
		}
		host, path = h, p
	default:
		return "", ""
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h // a port
	}
	owner, _, _ = strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if host == "" || owner == "" {
		return "", ""
	}
	return host, owner
}
