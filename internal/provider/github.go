// Package provider talks to the command-line tools of Git hosting services,
// which own the user's credentials. GitPersona only borrows tokens from them.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Runner runs a program and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs a real program. Standard error is discarded so that tool
// messages never mix with tokens on stdout.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// GitHub reads tokens from the GitHub CLI's keyring.
type GitHub struct {
	GH  string // path to the gh executable
	Run Runner
}

// Token returns the token gh holds for account on host.
func (g GitHub) Token(ctx context.Context, host, account string) (string, error) {
	out, err := g.Run(ctx, g.GH, "auth", "token", "--hostname", host, "--user", account)
	if err != nil {
		// The output may contain part of a token, so it is never included.
		return "", fmt.Errorf("gh auth token for %s on %s: %w", account, host, err)
	}
	return string(out), nil
}

// authStatus is the part of `gh auth status --json hosts` GitPersona reads.
// It never contains tokens because --show-token is never passed.
type authStatus struct {
	Hosts map[string][]struct {
		Login string `json:"login"`
		State string `json:"state"`
	} `json:"hosts"`
}

// SignedInAccounts lists the accounts gh is signed in to on host, using a
// status query that does not reveal tokens.
func (g GitHub) SignedInAccounts(ctx context.Context, host string) ([]string, error) {
	out, err := g.Run(ctx, g.GH, "auth", "status", "--hostname", host, "--json", "hosts")
	if err != nil {
		return nil, fmt.Errorf("gh auth status on %s: %w", host, err)
	}
	var status authStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parse gh auth status: %w", err)
	}
	var accounts []string
	for _, a := range status.Hosts[host] {
		if a.State == "success" {
			accounts = append(accounts, a.Login)
		}
	}
	return accounts, nil
}
