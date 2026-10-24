package i18n

// Messages of internal/web.
func init() {
	register(ZH, map[string]string{
		"invalid request: %w":        "请求无效：%w",
		"choose a backup":            "请选择一个备份",
		"at most %d folders at once": "一次最多 %d 个目录",
		"Note: %s does not exist yet; the rule applies once it does.": "注意：%s 还不存在，目录建好后规则才会生效。",
	})
}
