package e2e

import (
	"strings"
	"testing"
)

// The language switch turns the page, and what GitPersona says on it,
// into Chinese and back, and the page remembers the choice.
func TestTheLanguageSwitchTranslatesThePage(t *testing.T) {
	s := newSandbox(t, true)
	s.run("apply")
	link, _ := s.start("ui")
	b := startBrowser(t)
	b.viewport(1280, 900, false)
	b.open(link)

	click(b, `#lang-switch button[lang="zh-CN"]`)
	b.waitFor("Chinese navigation", `document.querySelector("#nav a[href='#profiles']")?.innerText.includes("账户")`)
	var htmlLang string
	b.eval(`return document.documentElement.lang`, &htmlLang)
	if htmlLang != "zh-CN" {
		t.Errorf("html lang = %q, want zh-CN", htmlLang)
	}
	b.waitFor("Chinese health checks from the server", `document.querySelector("#overview-health")?.innerText.includes("配置有效")`)
	if page := b.text("#view"); !strings.Contains(page, "账户及其目录") {
		t.Errorf("the overview is not in Chinese:\n%s", page)
	}

	b.call("Page.reload", map[string]any{})
	b.waitFor("the page", `document.querySelector("#view section h2")`)
	if h2 := b.text("#view section h2"); h2 != "总览" {
		t.Errorf("after a reload the heading is %q, want the remembered Chinese 总览", h2)
	}

	click(b, `#lang-switch button[lang="en"]`)
	b.waitFor("English navigation", `document.querySelector("#nav a[href='#profiles']")?.innerText.includes("Accounts")`)
	b.waitFor("English health checks", `document.querySelector("#overview-health")?.innerText.includes("configuration is valid")`)
	b.checkClean()
}
