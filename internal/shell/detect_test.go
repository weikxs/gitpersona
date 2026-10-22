package shell

import (
	"context"
	"errors"
	"testing"
)

func runner(outputs map[string]string) Runner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if out, ok := outputs[name]; ok {
			return []byte(out), nil
		}
		return nil, errors.New("not found")
	}
}

func TestLoginShellPrefersTheSystemRecordOverSHELL(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		outputs    map[string]string
		envShell   string
		wantKind   Kind
		wantSource string
	}{
		{name: "macOS dscl says fish while $SHELL says zsh", goos: "darwin",
			outputs: map[string]string{"dscl": "UserShell: /opt/homebrew/bin/fish\n"}, envShell: "/bin/zsh",
			wantKind: Fish, wantSource: "dscl"},
		{name: "Linux getent", goos: "linux",
			outputs: map[string]string{"getent": "alex:x:1000:1000:Alex:/home/alex:/usr/bin/zsh\n"}, envShell: "/bin/bash",
			wantKind: Zsh, wantSource: "getent"},
		{name: "falls back to $SHELL", goos: "linux", outputs: nil, envShell: "/bin/bash",
			wantKind: Bash, wantSource: "$SHELL"},
		{name: "unknown shell", goos: "darwin", outputs: map[string]string{"dscl": "UserShell: /usr/local/bin/nu\n"},
			wantKind: Unknown, wantSource: "dscl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := LoginShell(context.Background(), tc.goos, "alex", runner(tc.outputs), func(string) string { return tc.envShell })

			if got.Kind != tc.wantKind || got.Source != tc.wantSource {
				t.Fatalf("LoginShell() = %+v, want kind %q from %s", got, tc.wantKind, tc.wantSource)
			}
		})
	}
}

func TestKindOf(t *testing.T) {
	for path, want := range map[string]Kind{
		"/opt/homebrew/bin/fish": Fish, "/bin/zsh": Zsh, "/usr/bin/bash": Bash, "-zsh": Zsh, "/bin/sh": Unknown, "": Unknown,
	} {
		if got := KindOf(path); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Kind{"fish": Fish, "zsh": Zsh, "bash": Bash} {
		if got, err := Parse(in); err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := Parse("powershell"); err == nil {
		t.Error("Parse() accepted an unsupported shell")
	}
}

// psRunner fakes `ps -o ppid=,comm= -p PID` for a process tree.
func psRunner(tree map[string][2]string) Runner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ps" {
			return nil, errors.New("unexpected")
		}
		entry, ok := tree[args[len(args)-1]]
		if !ok {
			return nil, errors.New("no such process")
		}
		return []byte(" " + entry[0] + " " + entry[1] + "\n"), nil
	}
}

func TestCurrentShellWalksUpTheProcessTree(t *testing.T) {
	// gitpersona (500) <- env (400) <- -zsh (300) <- fish (200) <- launchd (1)
	tree := map[string][2]string{
		"400": {"300", "/usr/bin/env"},
		"300": {"200", "-zsh"},
		"200": {"1", "/opt/homebrew/bin/fish"},
		"1":   {"0", "/sbin/launchd"},
	}

	got := CurrentShell(context.Background(), 400, psRunner(tree))

	if got.Kind != Zsh || got.Source != "process 300" {
		t.Fatalf("CurrentShell() = %+v, want the nearest shell ancestor (zsh)", got)
	}
}

func TestCurrentShellUnknownWhenNoShellAncestor(t *testing.T) {
	tree := map[string][2]string{"400": {"1", "/usr/bin/make"}, "1": {"0", "/sbin/launchd"}}

	if got := CurrentShell(context.Background(), 400, psRunner(tree)); got.Kind != Unknown {
		t.Fatalf("CurrentShell() = %+v, want unknown", got)
	}
}

func TestInstalledShells(t *testing.T) {
	etcShells := "# comment\n/bin/sh\n/bin/zsh\n/bin/bash\n/opt/homebrew/bin/fish\n/usr/local/bin/nu\n"
	exists := func(p string) bool { return p != "/bin/bash" }

	got := InstalledShells(etcShells, exists)

	want := []Installed{{Kind: Fish, Path: "/opt/homebrew/bin/fish"}, {Kind: Zsh, Path: "/bin/zsh"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("InstalledShells() = %+v, want %+v (only existing fish/zsh/bash, in that order)", got, want)
	}
}
