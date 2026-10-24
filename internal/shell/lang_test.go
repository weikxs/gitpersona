package shell

import (
	"testing"

	"github.com/weikxs/gitpersona/internal/i18n"
)

func TestUnsupportedShellIsChineseInTheirLanguageOnly(t *testing.T) {
	_, err := Parse("tcsh")

	if err == nil {
		t.Fatal("Parse(tcsh) = nil error, want one")
	}
	if got, want := i18n.Text(i18n.ZH, err), `不支持的 shell "tcsh"（支持：fish、zsh、bash）`; got != want {
		t.Errorf("Chinese text = %q, want %q", got, want)
	}
	if got, want := err.Error(), `unsupported shell "tcsh" (supported: fish, zsh, bash)`; got != want {
		t.Errorf("English text = %q, want %q", got, want)
	}
}
