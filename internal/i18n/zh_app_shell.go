package i18n

// Messages of internal/app/shells.go, shell_cmd.go and glab.go.
func init() {
	register(map[string]string{
		// shells.go: `gitpersona shells`
		"default  %s":                    "默认  %s",
		"your login shell":               "你的登录 shell",
		"current  %s":                    "当前  %s",
		"the shell running this command": "运行此命令的 shell",
		"installed shells:":              "已安装的 shell：",
		"integration not installed":      "集成未安装",
		"integration installed":          "集成已安装",
		"unknown":                        "未知",
		"%s  %s (%s, from %s)":           "%s  %s（%s，来自 %s）",

		// shell_cmd.go: install-shell and the doctor's shell checks
		"%s is already set up in %s; leaving it as it is.":                                                               "%s 已在 %s 中配置过；保持原样。",
		"could not tell which shell you use (%q); choose with --shell fish, zsh or bash":                                 "无法判断你使用的 shell（%q）；请用 --shell fish、zsh 或 bash 指定",
		"Setting up %s (your login shell, from %s).":                                                                     "正在配置 %s（你的登录 shell，来自 %s）。",
		"Open a new terminal (or source the file) to start using it.":                                                    "打开新的终端（或 source 该文件）即可开始使用。",
		"%s integration is installed (%s)":                                                                               "%s 集成已安装（%s）",
		"shell integration is turned off (shell_integration = false in %s)":                                              "shell 集成已关闭（%s 中 shell_integration = false）",
		"%s integration is not installed (prompt display and gh/glab switching need it)":                                 "%s 集成未安装（提示符显示和 gh/glab 切换需要它）",
		"run `gitpersona install-shell`":                                                                                 "运行 `gitpersona install-shell`",
		"remove your own gh function if GitPersona's profiles cover the same directories":                                "如果 GitPersona 的配置文件已覆盖相同目录，请删除你自己的 gh 函数",
		"you are using %s right now, but your default shell is %s; %s has no integration":                                "你现在使用的是 %s，但默认 shell 是 %s；%s 没有集成",
		"run `gitpersona install-shell --shell %s` if you use it regularly":                                              "如果你经常使用它，请运行 `gitpersona install-shell --shell %s`",
		"gh in the shell that started gitpersona goes through GitPersona":                                                "启动 gitpersona 的 shell 中的 gh 会经过 GitPersona",
		"gh in the shell that started gitpersona is not GitPersona's (%s), so gh does not follow your directories there": "启动 gitpersona 的 shell 中的 gh 不是 GitPersona 的（%s），所以 gh 在那里不会跟随你的目录",
		"open a new terminal; if it persists, something later in your startup files puts another directory before %s":    "打开新的终端；如果问题仍在，说明启动文件中靠后的某处把另一个目录放到了 %s 前面",
		"Also installed: %s. To set them up too: gitpersona install-shell --shell %s":                                    "还安装了：%s。要一并配置它们：gitpersona install-shell --shell %s",

		// glab.go: the glab entry
		"gitpersona: %v; install it from https://gitlab.com/gitlab-org/cli": "gitpersona：%v；请从 https://gitlab.com/gitlab-org/cli 安装",
		"gitpersona: there is no token for %s on %s, and this directory uses it.\n" +
			"Save one with `gitpersona auth login --profile %s`, or set GITLAB_TOKEN to choose an account yourself.": "gitpersona：没有 %s 在 %s 上的令牌，而这个目录使用它。\n" +
			"请用 `gitpersona auth login --profile %s` 保存一个，或设置 GITLAB_TOKEN 自己选择账户。",
		"gitpersona: run %s: %v": "gitpersona：运行 %s 失败：%v",
	})
}
