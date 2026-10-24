package i18n

// Messages of internal/backup.
func init() {
	register(ZH, map[string]string{
		"create backup directory: %w":                     "创建备份目录失败：%w",
		"encode backup manifest: %w":                      "编码备份清单失败：%w",
		"create snapshot %s: %w":                          "创建快照 %s 失败：%w",
		"create snapshot in %s: too many snapshots at %s": "在 %s 创建快照失败：%s 处的快照太多",
		"back up %s: %w":                                  "备份 %s 失败：%w",
		"read backup manifest: %w":                        "读取备份清单失败：%w",
		"parse backup manifest: %w":                       "解析备份清单失败：%w",
		"list backups: %w":                                "列出备份失败：%w",
		"remove old backup: %w":                           "删除旧备份失败：%w",
		"read backup of %s: %w":                           "读取 %s 的备份失败：%w",
		"remove %s: %w":                                   "删除 %s 失败：%w",
		"restore %s: %w":                                  "恢复 %s 失败：%w",
		"restore link %s: %w":                             "恢复链接 %s 失败：%w",
	})
}
