package i18n

// Messages of internal/shell; the scripts it writes are never translated.
func init() {
	register(ZH, map[string]string{
		"unsupported shell %q (supported: fish, zsh, bash)": "不支持的 shell %q（支持：fish、zsh、bash）",
	})
}
