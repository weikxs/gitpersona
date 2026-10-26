package app

import (
	"context"
	"time"

	"github.com/weikxs/gitpersona/internal/credential"
	"github.com/weikxs/gitpersona/internal/provider"
)

const credentialTimeout = 15 * time.Second

// Credential serves one Git credential helper request for target. When gh
// cannot be found it stays silent, so Git moves on to the next helper.
func Credential(ctx context.Context, e Env, op string, target credential.Target) error {
	ctx, cancel := context.WithTimeout(ctx, credentialTimeout)
	defer cancel()

	gh, err := e.GHPath()
	if err != nil {
		gh = "" // no token source; Serve still validates op and stays silent
	}
	return credential.Serve(ctx, op, e.Stdin, e.Stdout, target, ghSource{gh: gh, run: e.Run})
}

// ghSource reports no token when gh is unavailable.
type ghSource struct {
	gh  string
	run provider.Runner
}

func (s ghSource) Token(ctx context.Context, host, account string) (string, error) {
	if s.gh == "" {
		return "", nil
	}
	return provider.GitHub{GH: s.gh, Run: s.run}.Token(ctx, host, account)
}
