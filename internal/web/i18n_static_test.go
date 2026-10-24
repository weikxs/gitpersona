package web

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

var (
	dictEntry  = regexp.MustCompile(`(?m)^\s*"([^"]+)": "(.*)",$`)
	keyLiteral = regexp.MustCompile(`"([a-z]+\.[A-Za-z]+)"`)
	// dynamicKey is a key built from a value: t(`reason.${r.reason}`).
	dynamicKey  = regexp.MustCompile("`([a-z]+)\\.\\$\\{")
	placeholder = regexp.MustCompile(`\{(\w+)\}`)
	han         = regexp.MustCompile(`\p{Han}`)
)

// mayHoldChinese are the files that need Chinese in their code: the
// language switch names 中文 in Chinese whatever the page's language, and
// app.js recognizes "nothing to change" in the server's output in both
// languages.
var mayHoldChinese = map[string]bool{"i18n.js": true, "app.js": true}

// dynamicValues are the values the page fills into dynamic keys.
var dynamicValues = map[string][]string{
	"reason": {"pin", "rule", "default", "none"},
	"health": {"ok", "warn", "fail"},
}

func dictionary(t *testing.T, content string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range dictEntry.FindAllStringSubmatch(content, -1) {
		if _, dup := out[m[1]]; dup {
			t.Errorf("key %q is in a dictionary twice", m[1])
		}
		out[m[1]] = m[2]
	}
	return out
}

func placeholders(s string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// The page's text is in both languages: the same keys, the same
// placeholders, every key the views use, and no key nobody uses.
func TestThePageHasEveryTextInEnglishAndChinese(t *testing.T) {
	files := staticFiles(t)
	en := dictionary(t, files["i18n-en.js"])
	zh := dictionary(t, files["i18n-zh.js"])
	if len(en) < 100 {
		t.Fatalf("the English dictionary has only %d entries; is its format still one entry per line?", len(en))
	}
	for key, text := range en {
		other, ok := zh[key]
		if !ok {
			t.Errorf("no Chinese text for %q", key)
			continue
		}
		if !slices.Equal(placeholders(text), placeholders(other)) {
			t.Errorf("%q: placeholders differ: English %v, Chinese %v", key, placeholders(text), placeholders(other))
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("no English text for %q", key)
		}
	}

	used := map[string]bool{}
	for name, content := range files {
		if !strings.HasSuffix(name, ".js") || strings.HasPrefix(name, "i18n-") {
			continue
		}
		if m := han.FindString(content); m != "" && !mayHoldChinese[name] {
			t.Errorf("%s has Chinese text in the code; put it in the dictionaries", name)
		}
		for _, m := range keyLiteral.FindAllStringSubmatch(content, -1) {
			used[m[1]] = true
		}
		for _, m := range dynamicKey.FindAllStringSubmatch(content, -1) {
			values, ok := dynamicValues[m[1]]
			if !ok {
				t.Errorf("%s builds keys %s.…; list its values in dynamicValues", name, m[1])
			}
			for _, v := range values {
				used[m[1]+"."+v] = true
			}
		}
	}
	for key := range used {
		if _, ok := en[key]; !ok && looksLikeTextKey(key, en) {
			t.Errorf("the page uses %q, which no dictionary has", key)
		}
	}
	for key := range en {
		if !used[key] {
			t.Errorf("no page uses %q", key)
		}
	}
}

// looksLikeTextKey tells a text key from other dotted strings in the code
// (file names such as "app.js"): it shares its prefix with a dictionary key.
func looksLikeTextKey(key string, dict map[string]string) bool {
	prefix, _, _ := strings.Cut(key, ".")
	for k := range dict {
		if strings.HasPrefix(k, prefix+".") {
			return true
		}
	}
	return false
}
