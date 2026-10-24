package generate

import (
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

// Generate validates first, so these guard the snippet writer on its own.
func TestProfileSnippetRefusesKeyPathsItCannotResolve(t *testing.T) {
	base := config.Profile{ID: "acme", Provider: config.ProviderGitHub, Host: "github.com", Account: "acme-alex",
		Name: "Alex", Email: "acme-alex@demo.example"}
	tests := []struct {
		name    string
		mutate  func(p config.Profile) config.Profile
		wantErr string
	}{
		{"relative ssh signing key", func(p config.Profile) config.Profile {
			p.Signing = config.Signing{Method: config.SigningSSH, Key: "keys/acme.pub"}
			return p
		}, "signing key"},
		{"relative ssh_key", func(p config.Profile) config.Profile {
			p.SSHKey = "keys/acme"
			return p
		}, "ssh_key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := profileSnippet(tc.mutate(base), demoOptions())

			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("profileSnippet() error = %v, want one about the %s", err, tc.wantErr)
			}
		})
	}
}
