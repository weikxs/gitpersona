package web

import (
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// staticFiles returns every embedded page file by its path under static/.
func staticFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := static.ReadFile(p)
		files[strings.TrimPrefix(p, "static/")] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPageMarkupWorksUnderTheStrictCSP(t *testing.T) {
	page := staticFiles(t)["index.html"]
	for name, pattern := range map[string]string{
		"inline script":   `<script(?:\s[^>]*)?>\s*[^<\s]`,
		"style element":   `<style`,
		"style attribute": `\sstyle=`,
		"event attribute": `\son[a-z]+=`,
		"javascript: URL": `javascript:`,
	} {
		if regexp.MustCompile(`(?i)` + pattern).MatchString(page) {
			t.Errorf("index.html has an %s, which the CSP blocks", name)
		}
	}
	for _, tag := range regexp.MustCompile(`(?i)<script[^>]*>`).FindAllString(page, -1) {
		if !strings.Contains(tag, `src="/static/`) {
			t.Errorf("script must load a page file: %s", tag)
		}
	}
	if !strings.Contains(page, csrfPlaceholder) {
		t.Error("index.html must carry the CSRF placeholder")
	}
}

func TestPageLoadsNothingFromOutside(t *testing.T) {
	external := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}|@import|url\(\s*['"]?(https?:|//)`)
	for name, content := range staticFiles(t) {
		// The SVG namespace names a vocabulary; nothing is loaded from it.
		content = strings.ReplaceAll(content, `"http://www.w3.org/2000/svg"`, "")
		if m := external.FindString(content); m != "" {
			t.Errorf("%s refers to an outside resource: %q", name, m)
		}
	}
}

func TestScriptsNeverParseTextAsHTML(t *testing.T) {
	sinks := regexp.MustCompile(`\.(innerHTML|outerHTML|insertAdjacentHTML)\b|document\.write|\beval\(|new Function\(|setTimeout\(\s*["']`)
	for name, content := range staticFiles(t) {
		if !strings.HasSuffix(name, ".js") {
			continue
		}
		if m := sinks.FindString(content); m != "" {
			t.Errorf("%s uses %q; build elements with textContent instead", name, m)
		}
		if strings.Contains(content, "console.log") {
			t.Errorf("%s has a console.log", name)
		}
	}
}

func TestEveryModuleImportResolves(t *testing.T) {
	files := staticFiles(t)
	imports := regexp.MustCompile(`(?m)^\s*(?:import|export)\b[^"']*from\s+["']([^"']+)["']`)
	for name, content := range files {
		if !strings.HasSuffix(name, ".js") {
			continue
		}
		for _, m := range imports.FindAllStringSubmatch(content, -1) {
			target := path.Join(path.Dir(name), m[1])
			if !strings.HasPrefix(m[1], ".") {
				t.Errorf("%s imports %q; only relative page modules are allowed", name, m[1])
			} else if _, ok := files[target]; !ok {
				t.Errorf("%s imports %q, which does not exist", name, m[1])
			}
		}
	}
}

func TestModulesAreServedAsJavaScript(t *testing.T) {
	s := newTestServer(t)
	cookie := login(t, s)
	for name := range staticFiles(t) {
		if name == "index.html" {
			continue // served at / with the CSRF token
		}
		r := request("GET", "/static/"+name, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d", name, w.Code)
			continue
		}
		ctype := w.Header().Get("Content-Type")
		if strings.HasSuffix(name, ".js") && !strings.HasPrefix(ctype, "text/javascript") {
			t.Errorf("%s served as %q; module scripts need text/javascript", name, ctype)
		}
	}
}

func TestStaticFoldersAreNotListed(t *testing.T) {
	s := newTestServer(t)
	cookie := login(t, s)
	for _, dir := range []string{"/static/", "/static/views/"} {
		r := request("GET", dir, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404 (no directory listing)", dir, w.Code)
		}
	}
}

// tokenPairs are the foreground/background token pairs the page uses for text.
var tokenPairs = [][2]string{
	{"text", "bg"}, {"text", "surface"}, {"text", "surface-2"}, {"muted", "bg"}, {"muted", "surface"},
	{"accent", "surface"}, {"accent", "bg"}, {"accent-text", "accent"}, {"text", "accent-soft"},
	{"ok", "ok-soft"}, {"warn", "warn-soft"}, {"fail", "fail-soft"}, {"fail", "surface"},
	{"add-text", "add-bg"}, {"del-text", "del-bg"}, {"hunk", "surface"}, {"muted", "surface-2"},
}

func TestColorTokensMeetWCAGContrastInBothThemes(t *testing.T) {
	css := staticFiles(t)["app.css"]
	dark := regexp.MustCompile(`(?s)@media \(prefers-color-scheme: dark\)\s*\{\s*:root\s*\{(.*?)\}`).FindStringSubmatch(css)
	if dark == nil {
		t.Fatal("app.css has no dark theme")
	}
	light := regexp.MustCompile(`(?s):root\s*\{(.*?)\}`).FindStringSubmatch(css)[1]
	for theme, block := range map[string]string{"light": light, "dark": dark[1]} {
		tokens := colorTokens(block)
		for _, pair := range tokenPairs {
			fg, bg := tokens[pair[0]], tokens[pair[1]]
			if fg == "" || bg == "" {
				t.Errorf("%s theme: missing --%s or --%s", theme, pair[0], pair[1])
				continue
			}
			if ratio := contrast(fg, bg); ratio < 4.5 {
				t.Errorf("%s theme: --%s on --%s is %.2f:1, below 4.5:1", theme, pair[0], pair[1], ratio)
			}
		}
		// Focus rings and borders of inputs need 3:1 against the surface.
		if ratio := contrast(tokens["focus"], tokens["surface"]); ratio < 3 {
			t.Errorf("%s theme: focus ring is %.2f:1 against the surface, below 3:1", theme, ratio)
		}
	}
	if !strings.Contains(css, "prefers-reduced-motion") {
		t.Error("app.css must respect prefers-reduced-motion")
	}
}

func colorTokens(block string) map[string]string {
	tokens := map[string]string{}
	for _, m := range regexp.MustCompile(`--([a-z0-9-]+):\s*(#[0-9a-fA-F]{6})\s*;`).FindAllStringSubmatch(block, -1) {
		tokens[m[1]] = m[2]
	}
	return tokens
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func luminance(hex string) float64 {
	var sum float64
	for i, weight := range []float64{0.2126, 0.7152, 0.0722} {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		sum += weight * c
	}
	return sum
}

// Browsers compile pattern attributes with the RegExp v flag, where these
// characters must be escaped inside a character class; an invalid pattern is
// silently ignored, so the field would not be checked at all.
func TestInputPatternsAreValidInVMode(t *testing.T) {
	patterns := regexp.MustCompile(`pattern: "((?:[^"\\]|\\.)*)"`)
	classes := regexp.MustCompile(`\[((?:[^\]\\]|\\.)*)\]`)
	reserved := regexp.MustCompile(`(^|[^\\])[()\[\]{}/|-]`)
	for name, content := range staticFiles(t) {
		for _, m := range patterns.FindAllStringSubmatch(content, -1) {
			source := strings.ReplaceAll(m[1], `\\`, `\`) // the JavaScript string's value
			for _, class := range classes.FindAllStringSubmatch(source, -1) {
				// Ranges such as a-z are allowed; remove them before looking.
				body := regexp.MustCompile(`[A-Za-z0-9]-[A-Za-z0-9]`).ReplaceAllString(class[1], "")
				if reserved.MatchString(body) {
					t.Errorf("%s: pattern %q has an unescaped special character in [%s]", name, source, class[1])
				}
			}
		}
	}
}

func TestApplySendsThePlanThePageShowed(t *testing.T) {
	files := staticFiles(t)
	if !strings.Contains(files["api.js"], `request("POST", "/api/apply", { yes, plan })`) {
		t.Error("api.apply must send the plan's hash with the request")
	}
	if !strings.Contains(files["views/apply.js"], "api.apply(true, plan)") {
		t.Error("the apply view must send the hash of the preview it shows")
	}
}

func TestThePageRefreshesWhenItsTabIsShownAgain(t *testing.T) {
	app := staticFiles(t)["app.js"]
	if !strings.Contains(app, `document.addEventListener("visibilitychange", refreshWhenVisible)`) {
		t.Fatal("app.js must refresh the state when the tab becomes visible")
	}
	_, body, found := strings.Cut(app, "async function refreshWhenVisible")
	if !found {
		t.Fatal("app.js has no refreshWhenVisible function")
	}
	body, _, _ = strings.Cut(body, "\n}\n")
	// No fresh=true: showing the tab again must not query gh's accounts.
	if !strings.Contains(body, "await loadState();") || strings.Contains(body, "loadState(true)") {
		t.Errorf("refreshWhenVisible must reuse gh's recent answer:\n%s", body)
	}
}

func TestTheOverviewAsksForTheCappedReusableCheck(t *testing.T) {
	overview := staticFiles(t)["views/overview.js"]
	for _, want := range []string{"api.verify([], false, auto)", "if (!verified) check(true);", "check(false)"} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview.js should contain %q", want)
		}
	}
}

func TestThePageSaysGoodbyeWhenItIsClosed(t *testing.T) {
	files := staticFiles(t)
	if !strings.Contains(files["app.js"], `window.addEventListener("pagehide"`) {
		t.Error("app.js must tell the server when the page goes away")
	}
	if !strings.Contains(files["api.js"], `fetch("/api/bye"`) || !strings.Contains(files["api.js"], "keepalive: true") {
		t.Error("bye must be a keepalive request to /api/bye")
	}
}
