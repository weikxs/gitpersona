package config

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/i18n"
)

func TestValidationProblemsAreShownInChineseOneToALine(t *testing.T) {
	cfg := Config{Version: CurrentVersion, Profiles: []Profile{
		{ID: "work", Provider: ProviderGitHub, Host: "github.com", Account: "octo-alex", Name: "Alex", Email: "not-an-email"},
		{ID: "work", Provider: ProviderGitHub, Host: "", Account: "octo-alex", Name: ""},
	}}

	err := cfg.Validate("/tmp/home", false)

	if err == nil {
		t.Fatal("Validate() = nil, want problems")
	}
	zh := i18n.Text(i18n.ZH, err)
	for _, want := range []string{"配置文件 id \"work\" 重复", "配置文件 \"work\"：必须填写主机", "配置文件 \"work\"：必须填写名称", "邮箱 \"not-an-email\" 无效"} {
		if !strings.Contains(zh, want) {
			t.Errorf("Chinese text %q lacks %q", zh, want)
		}
	}
	if lines := strings.Split(zh, "\n"); len(lines) != 5 {
		t.Errorf("Chinese text has %d lines, want one per problem (5): %q", len(lines), zh)
	}
	en := err.Error()
	for _, want := range []string{`duplicate profile id "work"`, `profile "work": host is required`} {
		if !strings.Contains(en, want) {
			t.Errorf("English text %q lacks %q", en, want)
		}
	}
	if strings.Contains(en, "配置") || strings.Contains(i18n.Text(i18n.EN, err), "配置") {
		t.Errorf("English text contains Chinese: %q", en)
	}
}

func TestValidationOfAGoodConfigurationIsNil(t *testing.T) {
	cfg := Config{Version: CurrentVersion, Profiles: []Profile{
		{ID: "work", Provider: ProviderGitHub, Host: "github.com", Account: "octo-alex", Name: "Alex", Email: "alex@example.com"},
	}}

	if err := cfg.Validate("/tmp/home", false); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestMissingConfigurationIsChineseAndStillErrNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "config.toml"))

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() = %v, want ErrNotFound", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, "找不到配置文件：") {
		t.Errorf("Chinese text = %q", got)
	}
}

func TestReadFailureKeepsItsCauseThroughTheChinese(t *testing.T) {
	_, err := Load(t.TempDir()) // a directory cannot be read as a file

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("Load(dir) = %v, want a *fs.PathError in the chain", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, "读取 ") || !strings.Contains(got, " 失败：") {
		t.Errorf("Chinese text = %q", got)
	}
}

func TestEditingErrorsAreChinese(t *testing.T) {
	text := "version = 1\n\n[[profile]]\nid = \"work\"\n"

	_, err := SetProfileField(text, "nope", "email", "alex@example.com")

	if err == nil {
		t.Fatal("SetProfileField() = nil error, want one")
	}
	if got := i18n.Text(i18n.ZH, err); got != `配置文件 "nope" 不存在` {
		t.Errorf("Chinese text = %q", got)
	}
	if err.Error() != `profile "nope" does not exist` {
		t.Errorf("English text = %q", err.Error())
	}
}
