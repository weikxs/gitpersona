package web

import (
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/app"
)

func TestStateDoctorChecksFollowThePageLanguage(t *testing.T) {
	w := newAPIWorld(t)

	for lang, want := range map[string]string{
		"zh":    "配置有效",
		"zh-CN": "配置有效",
		"en":    "configuration is valid",
		"":      "configuration is valid",
	} {
		t.Run("lang="+lang, func(t *testing.T) {
			got := w.callIn(t, lang, "GET", "/api/state", nil)

			var s app.StateReport
			decode(t, got, &s)
			var labels []string
			for _, c := range s.Doctor.Checks {
				labels = append(labels, c.Label)
			}
			if !strings.Contains(strings.Join(labels, "\n"), want) {
				t.Errorf("doctor labels for %q = %q, want one containing %q", lang, labels, want)
			}
		})
	}
}

func TestRequestErrorsFollowThePageLanguage(t *testing.T) {
	w := newAPIWorld(t)
	for _, tc := range []struct {
		name, path string
		body       any
		zh, en     string
	}{
		{"missing backup", "/api/rollback", map[string]string{}, "请选择一个备份", "choose a backup"},
		{"unknown shell", "/api/shells", map[string][]string{"shells": {"tcsh"}}, "不支持的 shell", "unsupported shell"},
		{"malformed body", "/api/shells", "not an object", "请求无效：", "invalid request: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zh := w.callIn(t, "zh", "POST", tc.path, tc.body)
			en := w.call(t, "POST", tc.path, tc.body)

			if zh.OK || !strings.Contains(zh.Error, tc.zh) {
				t.Errorf("in Chinese: %+v, want an error containing %q", zh, tc.zh)
			}
			if en.OK || !strings.Contains(en.Error, tc.en) {
				t.Errorf("in English: %+v, want an error containing %q", en, tc.en)
			}
		})
	}
}
