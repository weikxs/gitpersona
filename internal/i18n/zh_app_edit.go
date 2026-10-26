package i18n

// Messages of internal/app/apply.go, edit.go, maintenance.go,
// profile_cmd.go, shim.go and env.go.
func init() {
	register(map[string]string{ //nolint:gosec // translated messages mention tokens, not credentials
		// apply.go
		"Everything is already up to date.": "一切都已是最新。",
		"Nothing was written (dry run).":    "没有写入任何内容（试运行）。",
		"apply would remove %s; run `gitpersona apply --dry-run` to review, then `gitpersona apply --yes`":                        "apply 将删除 %s；请先运行 `gitpersona apply --dry-run` 查看，再运行 `gitpersona apply --yes`",
		"Not removing anything: apply would remove %s; run `gitpersona apply --dry-run` to review, then `gitpersona apply --yes`": "不会删除任何内容：apply 将删除 %s；请先运行 `gitpersona apply --dry-run` 查看，再运行 `gitpersona apply --yes`",
		"record the removed rules in the backup: %w":                                                                              "无法把已删除的规则记录到备份里：%w",
		"%w; nothing was changed: every file was put back as it was (backup in %s)":                                               "%w；没有做任何改动：所有文件都已恢复原样（备份在 %s）",
		"%w; putting the previous files back also failed (%v): they are saved in %s":                                              "%w；恢复之前的文件也失败了（%v）：它们保存在 %s",
		"Backup saved in %s": "备份已保存在 %s",
		"would remove %s":    "将删除 %s",
		"would create %s":    "将创建 %s",
		"would update %s":    "将更新 %s",
		"removed %s":         "已删除 %s",
		"created %s":         "已创建 %s",
		"updated %s":         "已更新 %s",
		"no configuration yet (%s); run `gitpersona setup` to create it from what you already have":                                                          "还没有配置（%s）；运行 `gitpersona setup`，根据你现有的设置创建",
		"the global Git configuration is disabled (GIT_CONFIG_GLOBAL=%s); GitPersona works through it, so unset GIT_CONFIG_GLOBAL or point it to a file":     "全局 Git 配置已被禁用（GIT_CONFIG_GLOBAL=%s）；GitPersona 要通过它工作，请取消设置 GIT_CONFIG_GLOBAL，或让它指向一个文件",
		"%s: %w. Open the file and keep exactly one pair of the lines %q and %q (or delete both), then run `gitpersona apply` again":                         "%s：%w。请打开该文件，只保留一对 %q 和 %q 行（或把两行都删掉），然后重新运行 `gitpersona apply`",
		"%s is read-only, so the old includeIf rules cannot be removed; make it writable (chmod u+w) or keep them (setup --keep-old-rules)":                  "%s 是只读的，所以无法删除旧的 includeIf 规则；请让它可写（chmod u+w），或保留它们（setup --keep-old-rules）",
		"%s is read-only; make it writable (chmod u+w), or add these lines at its end yourself (keep the marker lines so GitPersona recognizes them):\n\n%s": "%s 是只读的；请让它可写（chmod u+w），或自行把下面几行加到文件末尾（保留标记行，GitPersona 才能认出它们）：\n\n%s",
		"link %s: %w": "链接 %s 失败：%w",
		"the changes are no longer the ones you reviewed (a file changed since); nothing was written: review them again, then apply": "这些改动已不是你查看过的那些（之后有文件被改动）；没有写入任何内容：请重新查看，然后再应用",

		// edit.go
		"%s already uses profile %s through %s; nothing to change.": "%s 已在使用配置文件 %s（通过 %s），无需更改。",
		"%s already uses profile %s.":                               "%s 已在使用配置文件 %s。",
		"%s now uses profile %s (moved from %s).":                   "%s 现在使用配置文件 %s（从 %s 移过来）。",
		"%s now uses profile %s.":                                   "%s 现在使用配置文件 %s。",
		"! Your includeIf rule for %s does more than a profile can (%s); profile %s now applies in %s, so that is lost there.": "! 你为 %s 写的 includeIf 规则功能超出配置文件所能（%s）；配置文件 %s 现在适用于 %s，所以在那里这些功能会失效。",
		"%s has no rule of its own; it uses profile %s through %s (run `gitpersona remove %s` to remove that rule)":            "%s 没有自己的规则；它使用配置文件 %s（来自规则 %s，运行 `gitpersona remove %s` 可删除那条规则）",
		"%s no longer uses profile %s.":     "%s 不再使用配置文件 %s。",
		"%s is pinned to profile %s.":       "%s 已钉住到配置文件 %s。",
		"%s is no longer pinned.":           "%s 已取消钉住。",
		"%s is not inside a Git repository": "%s 不在 Git 仓库里",
		"%s is a worktree; worktrees follow their main repository (%s), so pin that instead": "%s 是工作树；工作树跟随它的主仓库（%s），请改为钉住主仓库",
		"could not change %s safely; edit it by hand (nothing was written)":                  "无法安全地修改 %s；请手动编辑（没有写入任何内容）",
		"%s changed while this change was being prepared; nothing was written, try again":    "准备这次修改时 %s 被改动了；没有写入任何内容，请重试",

		// lock.go
		"create %s: %w":        "无法创建 %s：%w",
		"open the lock %s: %w": "无法打开锁文件 %s：%w",
		"lock %s: %w":          "无法锁定 %s：%w",
		"another gitpersona command (or the configuration page) is changing the configuration; try again when it has finished (lock: %s)": "另一个 gitpersona 命令（或配置页面）正在修改配置；请等它完成后重试（锁文件：%s）",
		"choose a profile: gitpersona %s <%s>":  "请选择一个配置文件：gitpersona %s <%s>",
		"unknown profile %q; choose one of: %s": "未知的配置文件 %q；请从中选择：%s",

		// maintenance.go
		"No backups yet.":      "还没有备份。",
		"%s (latest)":          "%s（最新）",
		"  by `gitpersona %s`": "  由 `gitpersona %s` 创建",
		"Restore one with: gitpersona rollback <name|latest> --yes":           "用下面的命令恢复：gitpersona rollback <名称|latest> --yes",
		"rollback would restore %s; run `gitpersona rollback %s --yes`":       "回滚将恢复 %s；请运行 `gitpersona rollback %s --yes`",
		"Restored %s from %s.\nThe state before the rollback is saved in %s.": "已恢复 %s（来自 %s）。\n回滚前的状态已保存在 %s。",
		"Regenerating the Git configuration for the restored settings:":       "正在按恢复后的设置重新生成 Git 配置：",
		"no backup named %q; run `gitpersona rollback` to list them":          "没有名为 %q 的备份；运行 `gitpersona rollback` 可以列出所有备份",
		"uninstall would change %s; run `gitpersona uninstall --yes`":         "卸载将修改 %s；请运行 `gitpersona uninstall --yes`",
		"Put back your includeIf rule %q, which GitPersona had taken over.":   "已放回你的 includeIf 规则 %q，它原先被 GitPersona 接管。",
		"GitPersona is uninstalled. Your configuration stays in %s.":          "GitPersona 已卸载。你的配置仍保留在 %s。",

		// shim.go
		"%s is not signed in to gh on %s, and this directory uses it.\nSign in with `gh auth login --hostname %s`, or set GH_TOKEN to choose an account yourself.": "%s 没有在 %s 上登录 gh，而这个目录要用它。\n请运行 `gh auth login --hostname %s` 登录，或设置 GH_TOKEN 自己选择账号。",
		"run %s: %v": "运行 %s 失败：%v",
		"%w; install it from https://cli.github.com":      "%w；请从 https://cli.github.com 安装",
		"could not find the real %s on PATH (outside %s)": "在 PATH 中找不到真正的 %s（%s 之外）",

		// env.go
		"find current directory: %w":           "无法确定当前目录：%w",
		"find gitpersona executable: %w":       "无法找到 gitpersona 可执行文件：%w",
		"read global Git settings from %s: %w": "读取全局 Git 设置（来自 %s）失败：%w",
		"read Git settings %s (%s): %w":        "读取 Git 设置 %s（%s）失败：%w",
	})
}
