package i18n

// Messages of internal/generate; the Git configuration it writes is never
// translated.
func init() {
	register(ZH, map[string]string{
		"invalid configuration: %w":  "配置无效：%w",
		"profile %q signing key: %w": "配置文件 %q 的签名密钥：%w",
		"profile %q ssh_key: %w":     "配置文件 %q 的 ssh_key：%w",
	})
}
