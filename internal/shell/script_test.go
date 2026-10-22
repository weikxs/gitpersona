package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const binDir = "/tmp/home/.config/gitpersona/bin"

// shellTestTimeout stops a script bug (such as an endless loop) from hanging the suite.
const shellTestTimeout = 10 * time.Second

func TestScriptContent(t *testing.T) {
	for _, k := range All {
		script := Script(k, binDir)

		for _, want := range []string{binDir, "gitpersona_prompt", "command gitpersona prompt"} {
			if !strings.Contains(script, want) {
				t.Errorf("%s script is missing %q:\n%s", k, want, script)
			}
		}
	}
	if !strings.Contains(Script(Fish, binDir), "_tide_item_gitpersona") {
		t.Error("fish script should define the tide item")
	}
	if !strings.Contains(Script(Zsh, binDir), "prompt_gitpersona") {
		t.Error("zsh script should define the Powerlevel10k segment")
	}
}

func TestScriptQuotesPathsWithSpaces(t *testing.T) {
	script := Script(Bash, "/tmp/my home/bin")

	if !strings.Contains(script, `'/tmp/my home/bin'`) {
		t.Fatalf("path with a space is not quoted:\n%s", script)
	}
}

// TestScriptsRunInRealShells checks syntax with each installed shell and
// that loading the script adds the directory to PATH exactly once and
// defines the prompt function.
func TestScriptsRunInRealShells(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "fake")
	if err := os.MkdirAll(fake, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake, "gitpersona"), []byte("#!/bin/sh\necho demo-account\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	checks := map[Kind][]string{
		Fish: {"fish", "--no-config", "-c"},
		Zsh:  {"zsh", "-f", "-c"},
		Bash: {"bash", "--norc", "-c"},
	}
	for kind, argv := range checks {
		if _, err := exec.LookPath(argv[0]); err != nil {
			t.Logf("%s not installed; skipped", kind)
			continue
		}
		script := filepath.Join(dir, "init."+string(kind))
		if err := os.WriteFile(script, []byte(Script(kind, bin)), 0o600); err != nil {
			t.Fatal(err)
		}
		// bin starts at the END of PATH, as macOS path_helper leaves it. After
		// loading the script twice it must be first and present exactly once.
		probe := "source " + script + "; source " + script + "; echo $PATH | tr ':' '\\n' | head -1; echo $PATH | tr ':' '\\n' | grep -c '^" + bin + "$'; gitpersona_prompt"
		if kind == Fish {
			probe = "source " + script + "; source " + script + "; echo $PATH[1]; string join \\n $PATH | grep -c '^" + bin + "$'; gitpersona_prompt"
		}
		ctx, cancel := context.WithTimeout(context.Background(), shellTestTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, argv[0], append(argv[1:], probe)...)
		cmd.Env = append(os.Environ(), "PATH="+fake+":/usr/bin:/bin:"+bin)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed to load the script: %v\n%s", kind, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != bin+"\n1\ndemo-account" {
			t.Errorf("%s: got %q, want bin first on PATH, once, and the prompt output", kind, got)
		}
	}
}
