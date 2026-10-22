package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "version flag prints build info", args: []string{"--version"}, wantCode: 0, wantStdout: "gitpersona dev"},
		{name: "version command prints build info", args: []string{"version"}, wantCode: 0, wantStdout: "gitpersona dev"},
		{name: "help flag prints usage", args: []string{"--help"}, wantCode: 0, wantStdout: "Usage:"},
		{name: "no arguments prints usage", args: nil, wantCode: 0, wantStdout: "Usage:"},
		{name: "unknown command fails with usage hint", args: []string{"bogus"}, wantCode: 2, wantStderr: `unknown command "bogus"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tc.args, &stdout, &stderr)

			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}
