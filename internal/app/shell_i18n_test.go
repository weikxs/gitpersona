package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/i18n"
	"github.com/weikxs/gitpersona/internal/shell"
)

func TestInstallShellSaysItInChinese(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	env.Lang = i18n.ZH

	if err := InstallShell(context.Background(), env, nil, false); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"正在配置 fish（你的登录 shell，来自 dscl）。",
		"还安装了：zsh、bash。要一并配置它们：gitpersona install-shell --shell fish,zsh,bash",
		"打开新的终端（或 source 该文件）即可开始使用。",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("install-shell should say %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Setting up") {
		t.Errorf("install-shell should have no English sentence left:\n%s", out.String())
	}
}

func TestInstallShellLeavesAShellSetUpElsewhereAloneInChinese(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	env.Lang = i18n.ZH
	mustWrite(t, fishFile(w), "gitpersona init fish | source\n", 0o600)

	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}

	if want := "fish 已在 ~/.config/fish/conf.d/gitpersona.fish 中配置过；保持原样。"; !strings.Contains(out.String(), want) {
		t.Errorf("install-shell should say %q:\n%s", want, out.String())
	}
}

func TestInstallShellWithoutAKnownShellFailsInEnglishAndChinese(t *testing.T) {
	w := newWorld(t)
	env, _ := shellEnv(t, w)
	env.LoginShell = func(context.Context) shell.Detected { return shell.Detected{Path: "/bin/ksh"} }

	err := InstallShell(context.Background(), env, nil, false)

	if err == nil || !strings.HasPrefix(err.Error(), `could not tell which shell you use ("/bin/ksh")`) {
		t.Fatalf("InstallShell() = %v", err)
	}
	if got := i18n.Text(i18n.ZH, err); got != `无法判断你使用的 shell（"/bin/ksh"）；请用 --shell fish、zsh 或 bash 指定` {
		t.Errorf("Chinese error = %q", got)
	}
}

func TestShellsReportIsShownInChinese(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	env.Lang = i18n.ZH

	if err := Shells(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"默认  fish  /opt/homebrew/bin/fish（你的登录 shell，来自 dscl）",
		"当前  zsh  -zsh（运行此命令的 shell，来自 process 300）",
		"已安装的 shell：",
		"fish  /opt/homebrew/bin/fish  集成已安装",
		"zsh   /bin/zsh  集成未安装",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("shells output is missing %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorShellChecksAreShownInChinese(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	env.Lang = i18n.ZH

	Doctor(context.Background(), env)

	for _, want := range []string{
		"fish 集成未安装（提示符显示和 gh/glab 切换需要它）",
		"运行 `gitpersona install-shell`",
		"你现在使用的是 zsh，但默认 shell 是 fish；zsh 没有集成",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor should say %q:\n%s", want, out.String())
		}
	}
}

func TestShellFilesAreTheSameInEitherLanguage(t *testing.T) {
	written := map[i18n.Lang]string{}
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ZH} {
		w := newWorld(t)
		env, _ := shellEnv(t, w)
		env.Lang = lang

		if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish, shell.Zsh}, false); err != nil {
			t.Fatal(err)
		}

		var all strings.Builder
		for _, path := range []string{fishFile(w), filepath.Join(w.home, ".zshenv"), filepath.Join(w.home, ".zshrc")} {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			all.Write(data)
		}
		written[lang] = strings.ReplaceAll(all.String(), w.home, "<home>")
	}

	if written[i18n.EN] == "" || written[i18n.EN] != written[i18n.ZH] {
		t.Errorf("startup files differ between languages:\nEN:\n%s\nZH:\n%s", written[i18n.EN], written[i18n.ZH])
	}
}

func TestGlabRefusalIsShownInChinese(t *testing.T) {
	w, env, stderr, _ := glabWorld(t)
	if err := os.Remove(filepath.Join(w.home, "keychain", "gitlab.globex.example@globex-alex")); err != nil {
		t.Fatal(err)
	}
	env.Lang = i18n.ZH

	code := GlabShim(context.Background(), env, []string{"mr", "list"})

	if code == 0 || !strings.Contains(stderr.String(), "没有 globex-alex 在 gitlab.globex.example 上的令牌") ||
		!strings.Contains(stderr.String(), "`gitpersona auth login --profile globex`") {
		t.Errorf("code %d:\n%s", code, stderr.String())
	}
}

func TestGhSwitcherNoteAndOldRulesAreShownInChinese(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.Lang = i18n.ZH

	note := env.ghSwitcherNote([]string{"~/.zshrc", "~/.bashrc"})

	if !strings.HasPrefix(note, "你已经在 ~/.zshrc、~/.bashrc 中切换 gh 账户") {
		t.Errorf("note = %q", note)
	}
	if err := RemoveOldRules(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "没有配置文件接管的旧 includeIf 身份规则；无需更改。") {
		t.Errorf("remove-old-rules output:\n%s", out.String())
	}
}
