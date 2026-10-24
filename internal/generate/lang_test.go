package generate

import (
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/i18n"
)

func TestInvalidConfigurationIsChineseAllTheWayDown(t *testing.T) {
	cfg := demoConfig()
	cfg.Profiles[0].Email = "not-an-email"

	_, err := Generate(cfg, demoOptions())

	if err == nil {
		t.Fatal("Generate() = nil error, want one")
	}
	zh := i18n.Text(i18n.ZH, err)
	if !strings.HasPrefix(zh, "配置无效：") || !strings.Contains(zh, "邮箱 \"not-an-email\" 无效") {
		t.Errorf("Chinese text = %q", zh)
	}
	if !strings.HasPrefix(err.Error(), "invalid configuration: ") {
		t.Errorf("English text = %q", err.Error())
	}
}

func TestGeneratedFilesContainNoChineseText(t *testing.T) {
	a, err := Generate(demoConfig(), demoOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range a {
		if strings.ContainsAny(f.Content, "配置文件") {
			t.Errorf("%s contains Chinese text: %q", f.Name, f.Content)
		}
	}
}
