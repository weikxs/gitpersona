// Package credential implements the Git credential helper protocol for one
// profile. GitPersona answers only requests it is sure about; for everything
// else it stays silent so Git falls through to the user's next helper.
//
// Tokens are never stored, logged or written anywhere except to Git on stdout.
package credential

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// Target is the account a profile's helper answers for. Generated
// configuration passes it on the command line, so the helper needs neither
// the configuration file nor the current directory.
type Target struct {
	Provider string
	Host     string
	Account  string
}

// TokenSource returns an access token for an account on a host.
type TokenSource interface {
	Token(ctx context.Context, host, account string) (string, error)
}

// Serve handles one helper invocation: op is "get", "store" or "erase",
// in is Git's request and out receives the answer.
func Serve(ctx context.Context, op string, in io.Reader, out io.Writer, target Target, src TokenSource) error {
	req, err := ParseRequest(in)
	if err != nil {
		return err
	}
	switch op {
	case "get":
		return get(ctx, req, out, target, src)
	case "store", "erase":
		return nil // GitPersona never stores credentials.
	default:
		return fmt.Errorf("unknown credential operation %q (expected get, store or erase)", op)
	}
}

func get(ctx context.Context, req map[string]string, out io.Writer, target Target, src TokenSource) error {
	if !answers(req, target) {
		return nil
	}
	account := accountFor(req, target)
	token, err := src.Token(ctx, target.Host, account)
	token = strings.TrimSpace(token)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n") {
		return nil // fall through; never surface the reason, it may contain secrets
	}
	_, err = fmt.Fprintf(out, "username=%s\npassword=%s\n", account, token)
	return err
}

// accountFor returns the account to answer for: the profile's, or the one
// the request names. A repository or remote URL that names another account
// explicitly (credential.username in the repository, user@ in the URL)
// pushes as that account, with the token gh keeps for it; when gh has none,
// the helper stays silent.
func accountFor(req map[string]string, target Target) string {
	if user := req["username"]; user != "" {
		return user
	}
	return target.Account
}

// answers reports whether this helper is responsible for the request.
func answers(req map[string]string, target Target) bool {
	if target.Provider != "github" { // other providers arrive in a later milestone
		return false
	}
	return req["protocol"] == "https" && strings.EqualFold(req["host"], target.Host)
}

// ParseRequest reads "key=value" lines up to the first blank line or EOF.
// Later values of a repeated key replace earlier ones; malformed lines are
// ignored, as Git itself does for unknown attributes.
func ParseRequest(in io.Reader) (map[string]string, error) {
	req := map[string]string{}
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			req[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read credential request: %w", err)
	}
	return req, nil
}
