package e2e

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// localPathMarks are pieces of absolute paths that would tell where the
// pictures were made; demo homes show paths with ~ instead.
var localPathMarks = []string{"/Users/", "/home/", "/var/folders/", "/private/", "/tmp/"}

// checkNoLocalPaths fails when text shows the sandbox's location or any
// other absolute home or temporary path.
func checkNoLocalPaths(t *testing.T, s *sandbox, where, text string) {
	t.Helper()
	marks := append([]string{filepath.Dir(s.home)}, localPathMarks...)
	for _, mark := range marks {
		if strings.Contains(text, mark) {
			t.Fatalf("%s shows a local path (%q):\n%s", where, mark, text)
		}
	}
}

// Stand-ins that make the pictures the same on every machine: zsh is the
// login shell (dscl on macOS, getent elsewhere) and the shell running now
// (ps).
const (
	fakeDscl   = "#!/bin/sh\necho 'UserShell: /bin/zsh'\n"
	fakeGetent = "#!/bin/sh\necho 'demo:x:1000:1000:Demo:/nonexistent:/bin/zsh'\n"
	fakePs     = "#!/bin/sh\necho '4242 /bin/zsh'\n"
)

// useDemoTools puts the fake gh and the stand-ins above in the demo home,
// first on PATH, so that GitPersona shows the gh path as ~/.local/bin/gh
// rather than as a temporary folder, and never the shells of this machine.
// GitPersona's own gh comes before them, as in a shell that loads it.
func (s *sandbox) useDemoTools() {
	s.t.Helper()
	s.write(".local/bin/gh", fakeGH, 0o700)
	s.write(".local/bin/dscl", fakeDscl, 0o700)
	s.write(".local/bin/getent", fakeGetent, 0o700)
	s.write(".local/bin/ps", fakePs, 0o700)
	sep := string(os.PathListSeparator)
	bin := filepath.Join(s.home, ".config/gitpersona/bin") + sep + filepath.Join(s.home, ".local/bin")
	env := make([]string, 0, len(s.env))
	for _, kv := range s.env {
		if strings.HasPrefix(kv, "PATH=") {
			parts := strings.Split(strings.TrimPrefix(kv, "PATH="), sep)
			kv = "PATH=" + bin + sep + strings.Join(parts[1:], sep)
		}
		env = append(env, kv)
	}
	s.env = env
}

// shootTerminal checks text for local paths and saves it, light and dark,
// as a terminal window sized to fit.
func shootTerminal(b *browser, s *sandbox, name, text string) {
	b.t.Helper()
	checkNoLocalPaths(b.t, s, name, text)
	for _, dark := range []bool{false, true} {
		b.viewport(1100, 400, dark)
		showTerminal(b, text, dark)
		var height int
		b.eval(`return Math.ceil(document.documentElement.scrollHeight)`, &height)
		b.viewport(1100, height, dark)
		b.screenshot(name + suffix(dark))
	}
}

// showTerminal opens a page that draws text as a terminal window.
func showTerminal(b *browser, text string, dark bool) {
	b.t.Helper()
	bg, fg, bar := "#ffffff", "#1f2328", "#eaeef2"
	if dark {
		bg, fg, bar = "#0d1117", "#e6edf3", "#161b22"
	}
	page := fmt.Sprintf(`<!doctype html><meta charset="utf-8"><body style="margin:0;background:transparent">
<div style="margin:24px;border-radius:10px;overflow:hidden;box-shadow:0 8px 28px rgb(0 0 0/25%%);background:%s">
<div style="height:30px;background:%s;display:flex;gap:8px;align-items:center;padding-left:14px">
<span style="width:12px;height:12px;border-radius:50%%;background:#ff5f57"></span>
<span style="width:12px;height:12px;border-radius:50%%;background:#febc2e"></span>
<span style="width:12px;height:12px;border-radius:50%%;background:#28c840"></span></div>
<pre style="margin:0;padding:18px 22px;color:%s;font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap">%s</pre>
</div>`, bg, bar, fg, html.EscapeString(strings.TrimRight(text, "\n")))
	b.call("Page.navigate", map[string]any{"url": "data:text/html;charset=utf-8," + url.PathEscape(page)})
	b.waitFor("the terminal", `document.querySelector("pre")`)
}
