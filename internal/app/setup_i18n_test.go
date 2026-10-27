package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/i18n"
	"github.com/weikxs/gitpersona/internal/shell"
)

func TestSetupProposalIsShownInChinese(t *testing.T) {
	_, env := autoWorld(t)
	env.Lang = i18n.ZH
	out := &strings.Builder{}
	env.Stdout = out
	env.Stdin = strings.NewReader("n\n")

	if err := Setup(context.Background(), env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"GitPersona 找到了 2 个 GitHub 账户，并在 ~/Projects、~/code 中查找了仓库：",
		"~/code/mine（它唯一的仓库属于 octo-alex）",
		"SSH 密钥 ~/.ssh/id_acme",
		"把 ~/code/acme/side 钉住到 octo-alex：该仓库设置了 alex@octo.example",
		"在这些目录之外，提交将使用 octo-alex（Alex Octo <alex@octo.example>），而不是 Demo Person <person@demo.example>",
		"~/code/other（它唯一的仓库，属于 other-org）",
		"应用这个配置吗？改动的文件会先备份。[Y/n]",
		"没有写入任何内容",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the Chinese summary should say %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "GitPersona found") {
		t.Errorf("the Chinese summary should have no English sentence left:\n%s", out.String())
	}
}

func TestSetupProposalFieldsFollowTheLanguage(t *testing.T) {
	_, env := autoWorld(t)
	english, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	env.Lang = i18n.ZH

	chinese, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if got := english.Profiles[1].Found[0].Why; !strings.Contains(got, "belong to acme-alex") {
		t.Errorf("English reason = %q", got)
	}
	if got := chinese.Profiles[1].Found[0].Why; !strings.Contains(got, "属于 acme-alex") || strings.Contains(got, "belong") {
		t.Errorf("Chinese reason = %q", got)
	}
	if got := chinese.Pins[0].Why; got != "该仓库设置了 alex@octo.example" {
		t.Errorf("Chinese pin reason = %q", got)
	}
	// Codes and paths stay the same in both languages.
	if english.Pins[0].Repo != chinese.Pins[0].Repo || english.Pins[0].Profile != chinese.Pins[0].Profile ||
		english.Default != chinese.Default || len(english.Profiles) != len(chinese.Profiles) {
		t.Errorf("the proposal differs beyond its sentences:\n%+v\n%+v", english, chinese)
	}
}

func TestSetupErrorsAreTranslatedWherePageShowsThem(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	env.Lang = i18n.ZH

	err := Setup(context.Background(), env, SetupOptions{Print: true})

	if err == nil {
		t.Fatal("Setup() without gh should fail")
	}
	if !strings.Contains(err.Error(), "gh auth login") || !strings.HasPrefix(err.Error(), "setup needs the GitHub CLI") {
		t.Errorf("the error keeps its English text for the command line: %v", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.HasPrefix(got, "配置需要 GitHub CLI") {
		t.Errorf("the page shows %q, want the Chinese text", got)
	}
}

func TestExistingConfigIsRefusedInChinese(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	err := Setup(context.Background(), env, SetupOptions{})

	if err == nil {
		t.Fatal("Setup() over an existing config should fail")
	}
	if got := i18n.Text(i18n.ZH, err); !strings.Contains(got, "已存在") || !strings.Contains(got, "`gitpersona ui`") {
		t.Errorf("Chinese error = %q", got)
	}
}

// filesBelow reads every file of a setup in a home directory, with the home
// written as <home> and without repositories' own .git folders, so two
// homes can be compared.
func filesBelow(t *testing.T, home string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(home, path)
		if d.IsDir() && (d.Name() == ".git" || strings.Contains(rel, "backups")) {
			return filepath.SkipDir
		}
		// The generation is a time stamp for shell prompts, not content.
		if !d.Type().IsRegular() || d.Name() == shell.GenerationFile {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = strings.ReplaceAll(string(data), home, "<home>")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestApplyingASetupWritesTheSameFilesInEitherLanguage(t *testing.T) {
	snapshots := map[i18n.Lang]map[string]string{}
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ZH} {
		w, env := autoWorld(t)
		env.Lang = lang
		proposal, err := ProposeSetup(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}
		plan := proposal.Plan()
		plan.Shells = []string{"zsh"}

		if err := ApplySetup(context.Background(), env, plan); err != nil {
			t.Fatalf("ApplySetup(%s): %v", lang, err)
		}
		snapshots[lang] = filesBelow(t, w.home)
	}

	en, zh := snapshots[i18n.EN], snapshots[i18n.ZH]
	if _, ok := en[filepath.Join(".config", "gitpersona", "config.toml")]; !ok {
		t.Fatalf("setup should write config.toml, wrote %v", en)
	}
	if len(en) != len(zh) {
		t.Fatalf("the languages wrote different files: %d and %d", len(en), len(zh))
	}
	for name, text := range en {
		if zh[name] != text {
			t.Errorf("%s differs between languages:\nEN:\n%s\nZH:\n%s", name, text, zh[name])
		}
	}
	for name, text := range zh {
		for _, r := range text {
			if r >= 0x4e00 && r <= 0x9fff {
				t.Errorf("%s has Chinese text in it: content written to files is never translated", name)
				break
			}
		}
	}
}

func TestReasonsInChineseAreWholeSentences(t *testing.T) {
	in := inference{layoutHints: layoutHints{lang: i18n.ZH}}

	if got := in.countPhrase(2, 3); got != "2/3 个仓库" {
		t.Errorf("countPhrase = %q", got)
	}
	if got := in.countPhrase(3, 3); got != "全部 3 个仓库" {
		t.Errorf("countPhrase = %q", got)
	}
	repos := []attribution{
		{account: "octo-alex", how: byEmail, repo: discover.Repo{Dir: "a", Email: "Alex@Octo.example"}},
		{account: "octo-alex", how: byEmail, repo: discover.Repo{Dir: "b", Email: "alex@octo.example"}},
		{account: "octo-alex", how: byUser, repo: discover.Repo{Dir: "c", User: "octo-alex"}},
	}

	got := in.reason(repos, "octo-alex", 3, 3)

	if want := "全部 3 个仓库属于 octo-alex；2 个设置了 alex@octo.example、1 个以 octo-alex 登录"; got != want {
		t.Errorf("reason() = %q, want %q", got, want)
	}
	english := inference{}
	if got := english.reason(repos, "octo-alex", 3, 3); got != "all 3 repositories belong to octo-alex; 2 set alex@octo.example, 1 signs in as octo-alex" {
		t.Errorf("English reason() = %q", got)
	}
}

func TestKeepReasonsAreTranslatedButStayNonEmpty(t *testing.T) {
	env := Env{Lang: i18n.ZH}

	for keep, want := range map[string]string{
		"its core.sshCommand has other options": "它的 core.sshCommand 还有其他选项",
		"it also sets core.editor":              "它还设置了 core.editor",
		"it signs with x509":                    "它使用 x509 签名",
		"something a later version words":       "something a later version words",
		"":                                      "",
	} {
		if got := env.keepWhy(keep); got != want {
			t.Errorf("keepWhy(%q) = %q, want %q", keep, got, want)
		}
	}
	if got := (Env{}).keepWhy("it also sets core.editor"); got != "it also sets core.editor" {
		t.Errorf("English keepWhy = %q", got)
	}
}

func TestListsReadInTheLanguageOfTheText(t *testing.T) {
	items := []string{"~/a", "~/b"}

	if got := setupListIn(i18n.EN, items); got != "~/a, ~/b" {
		t.Errorf("English list = %q", got)
	}
	if got := setupListIn(i18n.ZH, items); got != "~/a、~/b" {
		t.Errorf("Chinese list = %q", got)
	}
	if got := setupSemiIn(i18n.ZH, items); got != "~/a；~/b" {
		t.Errorf("Chinese semicolon list = %q", got)
	}
}
