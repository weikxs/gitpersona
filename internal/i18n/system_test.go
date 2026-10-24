package i18n

import (
	"errors"
	"testing"
)

func TestSystemFollowsMacOSLanguageOrderThenTheLocale(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(name string) string { return vars[name] }
	}
	apple := func(out string, err error) func(string, ...string) ([]byte, error) {
		return func(string, ...string) ([]byte, error) { return []byte(out), err }
	}
	for name, tc := range map[string]struct {
		goos string
		vars map[string]string
		run  func(string, ...string) ([]byte, error)
		want Lang
	}{
		"macOS in Chinese":              {"darwin", map[string]string{"LANG": "en_US.UTF-8"}, apple("(\n    \"zh-Hans-CN\",\n    \"en-CN\"\n)\n", nil), ZH},
		"macOS in English first":        {"darwin", nil, apple("(\n    \"en-AE\",\n    \"zh-Hans-AE\"\n)\n", nil), EN},
		"macOS in French":               {"darwin", map[string]string{"LANG": "zh_CN.UTF-8"}, apple("(\"fr-FR\")", nil), ""},
		"macOS unreadable, locale":      {"darwin", map[string]string{"LANG": "zh_CN.UTF-8"}, apple("", errors.New("no defaults")), ZH},
		"Linux LANG":                    {"linux", map[string]string{"LANG": "zh_CN.UTF-8"}, nil, ZH},
		"Linux LANGUAGE list first":     {"linux", map[string]string{"LANGUAGE": "en_GB:zh_CN", "LANG": "zh_CN.UTF-8"}, nil, EN},
		"Linux LC_ALL over LANG":        {"linux", map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "zh_CN.UTF-8"}, nil, EN},
		"Linux C locale is no language": {"linux", map[string]string{"LANG": "C.UTF-8"}, nil, ""},
		"Linux German":                  {"linux", map[string]string{"LANG": "de_DE.UTF-8"}, nil, ""},
		"nothing set":                   {"linux", nil, nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := System(tc.goos, env(tc.vars), tc.run); got != tc.want {
				t.Fatalf("System() = %q, want %q", got, tc.want)
			}
		})
	}
}
