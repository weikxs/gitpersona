package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPromptIsCachedUntilTheAnswerCanChange runs the prompt function in each
// installed shell: the same directory asks gitpersona once, and a new
// directory, a repository appearing (git init) or GitPersona saving a change
// (the generation file) asks again.
func TestPromptIsCachedUntilTheAnswerCanChange(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	bin := filepath.Join(configDir, "bin")
	fake := filepath.Join(root, "fake")
	calls := filepath.Join(root, "calls")
	for _, dir := range []string{bin, fake, filepath.Join(root, "a"), filepath.Join(root, "b")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	gitpersona := "#!/bin/sh\necho x >> '" + calls + "'\necho demo-account\n"
	if err := os.WriteFile(filepath.Join(fake, "gitpersona"), []byte(gitpersona), 0o700); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(configDir, GenerationFile)
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")

	shells := map[Kind][]string{
		Fish: {"fish", "--no-config", "-c"},
		Zsh:  {"zsh", "-f", "-c"},
		Bash: {"bash", "--norc", "-c"},
	}
	for kind, argv := range shells {
		if _, err := exec.LookPath(argv[0]); err != nil {
			t.Logf("%s not installed; skipped", kind)
			continue
		}
		_ = os.Remove(calls)
		_ = os.RemoveAll(filepath.Join(a, ".git"))
		_ = os.Remove(generation)
		script := filepath.Join(root, "init."+string(kind))
		if err := os.WriteFile(script, []byte(Script(kind, bin)), 0o600); err != nil {
			t.Fatal(err)
		}
		count := "wc -l < '" + calls + "' | tr -d ' '"
		steps := []string{
			"source " + script,
			"cd '" + a + "'",
			"gitpersona_prompt", "gitpersona_prompt", "gitpersona_prompt", count, // 1 call
			"mkdir .git", "gitpersona_prompt", "gitpersona_prompt", count, // a repository appeared: 2
			"echo 7 > '" + generation + "'", "gitpersona_prompt", count, // GitPersona saved a change: 3
			"cd '" + b + "'", "gitpersona_prompt", count, // another directory: 4
			"cd '" + a + "'", "gitpersona_prompt", count, // back again: 5
		}
		if kind == Zsh {
			steps = append(steps, "p10k() { print -r -- \"$*\" }", "prompt_gitpersona", count) // cached: 5
		}
		if kind == Fish {
			steps = append(steps, "function _tide_print_item; echo $argv; end", "_tide_item_gitpersona", count) // cached: 5
		}
		ctx, cancel := context.WithTimeout(context.Background(), shellTestTimeout)
		cmd := exec.CommandContext(ctx, argv[0], append(argv[1:], strings.Join(steps, "\n"))...)
		cmd.Env = append(os.Environ(), "PATH="+fake+":/usr/bin:/bin")
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s: %v\n%s", kind, err, out)
		}
		want := []string{"demo-account", "demo-account", "demo-account", "1",
			"demo-account", "demo-account", "2", "demo-account", "3", "demo-account", "4", "demo-account", "5"}
		switch kind {
		case Zsh:
			want = append(want, "segment -t 👤 demo-account", "5")
		case Fish:
			want = append(want, "gitpersona 👤 demo-account", "5")
		}
		if got := strings.Split(strings.TrimSpace(string(out)), "\n"); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s:\n got %q\nwant %q", kind, got, want)
		}
	}
}
