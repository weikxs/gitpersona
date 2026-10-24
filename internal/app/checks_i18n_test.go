package app

import (
	"context"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/i18n"
)

func demoProfileWithMissingFiles() config.Config {
	return config.Config{Profiles: []config.Profile{{
		ID: "acme", Provider: config.ProviderGitHub, Host: "github.com", Account: "acme-alex",
		SSHKey: "~/.ssh/acme-missing", Dirs: []string{"~/Projects/not-yet"},
	}}}
}

func TestKeyFileAndDirectoryChecksAreShownInChinese(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	cfg := demoProfileWithMissingFiles()

	keys := inChinese(env).checkKeyFiles(cfg)
	dirs := inChinese(env).checkDirs(cfg)

	if len(keys) != 1 || keys[0].label != "配置文件 acme：SSH 密钥 ~/.ssh/acme-missing 不存在" ||
		!strings.Contains(keys[0].advice, "无法签名或推送") {
		t.Errorf("key checks = %+v", keys)
	}
	if len(dirs) != 1 || dirs[0].label != "配置文件 acme：目录 ~/Projects/not-yet 还不存在" || !dirs[0].warn {
		t.Errorf("directory checks = %+v", dirs)
	}
	english := env.checkKeyFiles(cfg)
	if len(english) != 1 || english[0].label != "profile acme: SSH key ~/.ssh/acme-missing does not exist" {
		t.Errorf("English key checks = %+v", english)
	}
}

func TestMissingKeyErrorIsTranslatedWhereItIsShown(t *testing.T) {
	w := newWorld(t)
	p := demoProfileWithMissingFiles().Profiles[0]

	err := requireKeyFiles(p, w.home)

	if err == nil || !strings.HasPrefix(err.Error(), "SSH key ~/.ssh/acme-missing does not exist; create it first") {
		t.Fatalf("English error = %v", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, "SSH 密钥 ~/.ssh/acme-missing 不存在；请先创建") {
		t.Errorf("Chinese error = %q", got)
	}
}

func TestGitVersionCheckIsShownInChinese(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env.Run = func(context.Context, string, ...string) ([]byte, error) { return []byte("git version 2.20.1\n"), nil }

	old := inChinese(env).checkGitVersion(context.Background())

	if old.ok || !strings.Contains(old.label, "Git 2.20.1 低于 2.31") {
		t.Errorf("old Git check = %+v", old)
	}
	if !strings.Contains(old.advice, "升级 Git") {
		t.Errorf("advice = %q", old.advice)
	}
}

func TestAuthAndEditErrorsAreTranslatedWhereTheyAreShown(t *testing.T) {
	w, env, _ := verifyWorld(t)

	err := Use(context.Background(), env, "nobody")

	if err == nil || !strings.Contains(err.Error(), `unknown profile "nobody"`) {
		t.Fatalf("English error = %v", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, `未知的配置文件 "nobody"`) {
		t.Errorf("Chinese error = %q", got)
	}
	err = AuthLogout(context.Background(), inChinese(env), AuthOptions{Profile: "acme"})
	if err == nil || !strings.Contains(i18n.Text(i18n.ZH, err), "GitHub") {
		t.Errorf("auth on a GitHub profile = %v", err)
	}
	_ = w
}

func TestApplyOutputIsShownInChinese(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if err := Apply(context.Background(), inChinese(env), ApplyOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if !strings.Contains(got, "将创建 ~/.config/gitpersona/") || !strings.Contains(got, "没有写入任何内容（试运行）。") {
		t.Errorf("dry run output = %q", got)
	}
	if strings.Contains(got, "would create") {
		t.Errorf("English text left in the Chinese output: %q", got)
	}
}
