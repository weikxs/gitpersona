package discover

import "testing"

func TestRemoteHostOwnerNamesTheHostAndOwnerOfAnyRemoteForm(t *testing.T) {
	tests := []struct{ url, host, owner string }{
		{"git@github-acme:acme-alex/api.git", "github-acme", "acme-alex"},
		{"git@github-acme:*/**", "github-acme", "*"},
		{"https://github.com/octo-alex/api", "github.com", "octo-alex"},
		{"/srv/git/api.git", "", ""},
	}
	for _, tc := range tests {
		host, owner := RemoteHostOwner(tc.url)
		if host != tc.host || owner != tc.owner {
			t.Errorf("RemoteHostOwner(%q) = %q, %q; want %q, %q", tc.url, host, owner, tc.host, tc.owner)
		}
	}
}

func TestParseRemoteGivesNothingForURLsWithoutAnOwner(t *testing.T) {
	for _, url := range []string{
		"https://github.com",  // no path at all
		"https://github.com/", // empty owner
		"git@github.com:",     // empty path
	} {
		if host, owner, user := parseRemote(url); host != "" || owner != "" || user != "" {
			t.Errorf("parseRemote(%q) = %q, %q, %q; want nothing", url, host, owner, user)
		}
	}
}
