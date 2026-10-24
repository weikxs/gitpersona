package i18n

import (
	"strings"
)

// System returns the language of the system's user interface when it is
// one the page has (Chinese or English), and "" otherwise: a system in
// another language, or one that cannot be asked, leaves the choice to the
// browser. On macOS the order of System Settings decides (AppleLanguages);
// elsewhere, and when that cannot be read, the locale variables do.
func System(goos string, getenv func(string) string, run func(name string, args ...string) ([]byte, error)) Lang {
	if goos == "darwin" && run != nil {
		if out, err := run("defaults", "read", "-g", "AppleLanguages"); err == nil {
			if first := firstAppleLanguage(string(out)); first != "" {
				return supported(first)
			}
		}
	}
	for _, name := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		value := getenv(name)
		if name == "LANGUAGE" {
			value, _, _ = strings.Cut(value, ":") // a list, most preferred first
		}
		if value == "" || value == "C" || value == "POSIX" || strings.HasPrefix(value, "C.") {
			continue
		}
		return supported(value)
	}
	return ""
}

// firstAppleLanguage reads the first entry of `defaults read -g
// AppleLanguages`, which prints a property list such as
// ( "en-AE", "zh-Hans-AE" ).
func firstAppleLanguage(out string) string {
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "(")
	first, _, _ := strings.Cut(out, ",")
	first = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(first), ")"))
	return strings.Trim(first, `"`)
}

// supported maps a locale or language tag to a language the page has.
func supported(tag string) Lang {
	tag = strings.ToLower(tag)
	switch {
	case Parse(tag) == ZH:
		return ZH
	case tag == "en" || strings.HasPrefix(tag, "en-") || strings.HasPrefix(tag, "en_"):
		return EN
	}
	return ""
}
