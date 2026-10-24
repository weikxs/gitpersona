package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/i18n"
)

// inChinese is env with the page's language set to Chinese.
func inChinese(env Env) Env {
	env.Lang = i18n.ZH
	return env
}

func TestVerifyFindingsAndViaAreShownInChinese(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "user.email", "someone@else.example")
	runGit(t, w.repos["blog"], "config", "credential.https://github.com.username", "acme-alex")

	zh, err := inChinese(env).VerifyRepos(context.Background(), nil, false)
	if err != nil {
		t.Fatal(err)
	}

	billing := repoCheck(t, zh, "~/Projects/acme/billing")
	if len(billing.Findings) == 0 || !strings.Contains(billing.Findings[0].Text, "提交使用的是") ||
		!strings.Contains(billing.Findings[0].Text, "someone@else.example") {
		t.Fatalf("billing findings = %+v, want the Chinese sentence naming the author", billing.Findings)
	}
	if got := billing.Remotes[0].Via; got != "gh 令牌" {
		t.Errorf("billing via = %q, want the Chinese text", got)
	}
	blog := repoCheck(t, zh, "~/Projects/personal/blog")
	if got := blog.Remotes[0].ChosenBy; got != "仓库里的 credential.username" {
		t.Errorf("blog chosen by = %q, want the Chinese text", got)
	}
	if !strings.Contains(findingText(blog), "远程 origin 以 acme-alex 推送") {
		t.Errorf("blog findings = %q, want the Chinese note", findingText(blog))
	}
}

func TestVerifyCodesAreTheSameInBothLanguages(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "user.email", "someone@else.example")
	runGit(t, w.repos["blog"], "config", "credential.https://github.com.username", "acme-alex")
	ctx := context.Background()

	en, err := env.VerifyRepos(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	zh, err := inChinese(env).VerifyRepos(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(en.Repositories) != len(zh.Repositories) || len(en.Repositories) == 0 {
		t.Fatalf("repositories: %d in English, %d in Chinese", len(en.Repositories), len(zh.Repositories))
	}
	for i, e := range en.Repositories {
		z := zh.Repositories[i]
		if e.Path != z.Path || e.Verdict != z.Verdict || e.Reason != z.Reason || len(e.Findings) != len(z.Findings) {
			t.Errorf("%s: codes differ: %+v vs %+v", e.Path, e, z)
		}
		for j, f := range e.Findings {
			if f.Level != z.Findings[j].Level {
				t.Errorf("%s: finding level %q vs %q", e.Path, f.Level, z.Findings[j].Level)
			}
		}
		for j, r := range e.Remotes {
			if r.ViaKind != z.Remotes[j].ViaKind || r.ChosenKind != z.Remotes[j].ChosenKind || r.PushAs != z.Remotes[j].PushAs {
				t.Errorf("%s: remote codes differ: %+v vs %+v", e.Path, r, z.Remotes[j])
			}
		}
	}
	billing := repoCheck(t, en, "~/Projects/acme/billing")
	if billing.Remotes[0].ViaKind != ViaGH || billing.Remotes[0].ChosenKind != "" || billing.Verdict != VerdictProblem {
		t.Errorf("billing codes = %+v", billing)
	}
	blog := repoCheck(t, en, "~/Projects/personal/blog")
	if blog.Remotes[0].ChosenKind != ChosenRepoUsername {
		t.Errorf("blog chosen kind = %q", blog.Remotes[0].ChosenKind)
	}
	data, err := json.Marshal(zh.Repositories[0].Remotes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"via_kind":"`) {
		t.Errorf("remote JSON = %s, want a via_kind field", data)
	}
}

func TestVerifyKindsOfOtherWaysToPush(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "remote", "set-url", "origin", "https://x-access-token:ghp_demoSecret123@github.com/acme-corp/billing.git")
	runGit(t, w.repos["blog"], "remote", "set-url", "origin", "git@github.com:octo-alex/blog.git")
	env = withSSH(env, func(host string) string { return "hostname " + host + "\ncontrolpath none\n" })

	r := verify(t, env)

	if got := repoCheck(t, r, "~/Projects/acme/billing").Remotes[0]; got.ViaKind != ViaURLToken || got.ChosenKind != ChosenURLSecret {
		t.Errorf("token in URL: %+v", got)
	}
	if got := repoCheck(t, r, "~/Projects/personal/blog").Remotes[0]; got.ViaKind != ViaSSHAgent {
		t.Errorf("ssh without a key file: %+v", got)
	}
}

func TestWhoamiIsShownInChinese(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "user.email", "someone@else.example")
	zhEnv, out := w.env(t, w.repos["billing"])
	zhEnv = inChinese(zhEnv)

	if err := Whoami(context.Background(), zhEnv, false); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{"配置文件 acme", "作者    ", "原因    目录规则", "路径    ", "警告    ", "someone@else.example"} {
		if !strings.Contains(got, want) {
			t.Errorf("whoami output lacks %q:\n%s", want, got)
		}
	}
	enOut := whoamiJSON(t, env, w.repos["billing"])
	zhOut := whoamiJSON(t, zhEnv, w.repos["billing"])
	if enOut != zhOut {
		t.Errorf("the JSON report differs by language:\n%s\n%s", enOut, zhOut)
	}
}

func whoamiJSON(t *testing.T, env Env, dir string) string {
	t.Helper()
	env.WorkDir = dir
	report, err := WhoamiReportFor(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWhoamiCheckErrorIsTranslatedByTheLanguageItIsShownIn(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["billing"], "config", "user.email", "someone@else.example")
	env.WorkDir = w.repos["billing"]

	err := WhoamiWith(context.Background(), env, WhoamiOptions{Check: true})

	if err == nil || !strings.Contains(err.Error(), "this repository does not use profile acme") {
		t.Fatalf("English error = %v", err)
	}
	if got := i18n.Text(i18n.ZH, err); !strings.Contains(got, "这个仓库没有使用配置文件 acme") {
		t.Errorf("Chinese error = %q", got)
	}
}
