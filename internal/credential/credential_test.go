package credential

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSource struct {
	token       string
	err         error
	gotHost     string
	gotAccount  string
	calledTimes int
}

func (f *fakeSource) Token(_ context.Context, host, account string) (string, error) {
	f.calledTimes++
	f.gotHost, f.gotAccount = host, account
	return f.token, f.err
}

var target = Target{Provider: "github", Host: "github.com", Account: "acme-alex"}

func TestServeGetReturnsAccountAndToken(t *testing.T) {
	src := &fakeSource{token: "secret-token\n"}
	var out bytes.Buffer

	err := Serve(context.Background(), "get", strings.NewReader("protocol=https\nhost=github.com\n\n"), &out, target, src)

	if err != nil {
		t.Fatalf("Serve() error: %v", err)
	}
	if got, want := out.String(), "username=acme-alex\npassword=secret-token\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if src.gotHost != "github.com" || src.gotAccount != "acme-alex" {
		t.Fatalf("token requested for %s/%s, want github.com/acme-alex", src.gotHost, src.gotAccount)
	}
}

func TestServeGetStaysSilent(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		source    *fakeSource
		wantCalls int
	}{
		{name: "different host", input: "protocol=https\nhost=gitlab.com\n", source: &fakeSource{token: "t"}},
		{name: "plain http", input: "protocol=http\nhost=github.com\n", source: &fakeSource{token: "t"}},
		{name: "explicit other username", input: "protocol=https\nhost=github.com\nusername=octo-alex\n", source: &fakeSource{token: "t"}},
		{name: "token lookup fails", input: "protocol=https\nhost=github.com\n", source: &fakeSource{err: errors.New("not logged in")}, wantCalls: 1},
		{name: "empty token", input: "protocol=https\nhost=github.com\n", source: &fakeSource{token: "  \n"}, wantCalls: 1},
		{name: "token with embedded newline", input: "protocol=https\nhost=github.com\n", source: &fakeSource{token: "a\nb"}, wantCalls: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			err := Serve(context.Background(), "get", strings.NewReader(tc.input), &out, target, tc.source)

			if err != nil || out.Len() != 0 {
				t.Fatalf("Serve() = %v with output %q, want silence so Git falls through", err, out.String())
			}
			if tc.source.calledTimes != tc.wantCalls {
				t.Fatalf("token source called %d times, want %d", tc.source.calledTimes, tc.wantCalls)
			}
		})
	}
}

func TestServeMatchesSameUsername(t *testing.T) {
	var out bytes.Buffer

	_ = Serve(context.Background(), "get", strings.NewReader("protocol=https\nhost=github.com\nusername=acme-alex\n"), &out, target, &fakeSource{token: "t"})

	if !strings.Contains(out.String(), "password=t") {
		t.Fatalf("output = %q, want a password when the username matches", out.String())
	}
}

func TestServeIgnoresStoreAndErase(t *testing.T) {
	for _, op := range []string{"store", "erase"} {
		src := &fakeSource{token: "t"}
		var out bytes.Buffer

		err := Serve(context.Background(), op, strings.NewReader("protocol=https\nhost=github.com\npassword=x\n"), &out, target, src)

		if err != nil || out.Len() != 0 || src.calledTimes != 0 {
			t.Errorf("%s: err=%v output=%q calls=%d, want a silent no-op", op, err, out.String(), src.calledTimes)
		}
	}
}

func TestServeRejectsUnknownOperation(t *testing.T) {
	err := Serve(context.Background(), "approve", strings.NewReader(""), &bytes.Buffer{}, target, &fakeSource{})

	if err == nil {
		t.Fatal("Serve() with an unknown operation succeeded, want an error")
	}
}

func TestServeOnlyGitHubIsSupportedInM1(t *testing.T) {
	src := &fakeSource{token: "t"}
	var out bytes.Buffer
	gitlab := Target{Provider: "gitlab", Host: "gitlab.globex.example", Account: "globex-alex"}

	_ = Serve(context.Background(), "get", strings.NewReader("protocol=https\nhost=gitlab.globex.example\n"), &out, gitlab, src)

	if out.Len() != 0 || src.calledTimes != 0 {
		t.Fatalf("gitlab target answered (%q), want silence until GitLab support lands", out.String())
	}
}

func TestParseRequest(t *testing.T) {
	got, err := ParseRequest(strings.NewReader("protocol=https\nhost=github.com\nwwwauth[]=Basic\nnot-a-pair\n\nignored=after-blank\n"))

	if err != nil {
		t.Fatal(err)
	}
	if got["protocol"] != "https" || got["host"] != "github.com" || got["ignored"] != "" {
		t.Fatalf("ParseRequest() = %v", got)
	}
}
