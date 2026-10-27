package i18n

// Messages of internal/app/verify*.go, whoami.go, state.go and list.go.
func init() {
	register(map[string]string{ //nolint:gosec // translated messages mention tokens, not credentials
		// verify.go
		"commits use %s, not the profile's %s: %s": "提交使用的是 %s，而不是配置文件的 %s：%s",
		"no author": "没有作者",

		// verify_print.go
		"pinned repository": "钉住的仓库",
		"directory rule":    "目录规则",
		"default profile":   "默认配置文件",
		"no rule matches":   "没有规则匹配",
		"some repositories would commit or push as another identity; see above": "有些仓库会以另一个身份提交或推送；见上文",
		"not found: %s": "未找到：%s",
		"the scan stopped early or did not go deep enough; some repositories may be missing":                               "扫描提前结束，或没有扫得足够深；可能漏掉了一些仓库",
		"%d repositories in %s: %d as expected, %d with notes, %d with problems, %d unconfirmed, %d outside your profiles": "%d 个仓库（位于 %s）：%d 个符合预期，%d 个有提示，%d 个有问题，%d 个未确认，%d 个在配置文件之外",
		"(`gitpersona verify --ssh` asks the SSH hosts which account the unconfirmed ones push as)":                        "（`gitpersona verify --ssh` 会询问 SSH 主机，未确认的仓库是以哪个账号推送的）",
		"(`gitpersona verify --all` lists every repository; `gitpersona ui` shows them on a page)":                         "（`gitpersona verify --all` 会列出所有仓库；`gitpersona ui` 会在页面上显示）",
		"no profile: Git uses your own settings":                                                                           "没有配置文件：Git 使用你自己的设置",
		"%s · %s":                                                                                                          "%s · %s",
		"    author  %s":                                                                                                   "    作者    %s",
		"an account GitPersona cannot tell":                                                                                "GitPersona 无法判断的账号",
		"    push    %s → %s as %s (%s)":                                                                                   "    推送    %s → %s，账号 %s（%s）",

		// verify_remote.go
		"local repository":                               "本地仓库",
		"a token written in the remote URL":              "写在远程 URL 里的令牌",
		"the remote URL":                                 "远程 URL",
		"the remote URL names it":                        "远程 URL 指定了它",
		"credential.username in the repository":          "仓库里的 credential.username",
		"core.sshCommand in the repository":              "仓库里的 core.sshCommand",
		"gh token":                                       "gh 令牌",
		"GitPersona":                                     "GitPersona",
		"your credential helper":                         "你的凭据助手",
		"ssh-agent or ssh's default keys":                "ssh-agent 或 ssh 的默认密钥",
		"SSH key %s, %s greets %s":                       "SSH 密钥 %s，%s 问候的账号是 %s",
		"a key from ssh-agent, %s greets %s":             "来自 ssh-agent 的密钥，%s 问候的账号是 %s",
		"SSH key %s, a deploy key for %s":                "SSH 密钥 %s，是 %s 的部署密钥",
		"SSH key %s, refused: %s":                        "SSH 密钥 %s，被拒绝：%s",
		"SSH key %s, the host did not say which account": "SSH 密钥 %s，主机没有说明是哪个账号",
		"remote %s is on %s, where profile %s does not decide; your own settings apply":                                                                                                    "远程 %s 在 %s 上，配置文件 %s 不管那里；使用你自己的设置",
		"remote %s has a token written in its URL (kept in plain text in .git/config); Git pushes with it, whichever account it belongs to: `git remote set-url %s https://%s` removes it": "远程 %s 的 URL 里写了令牌（以明文保存在 .git/config 里）；无论令牌属于哪个账号，Git 都用它推送：运行 `git remote set-url %s https://%s` 可以去掉它",
		"pushes to remote %s do not sign in as an account (%s); the profile's account is %s":                                                                                               "推送到远程 %s 时没有以某个账号登录（%s）；配置文件的账号是 %s",
		"which account remote %s pushes as is not in your settings (%s); `gitpersona verify --ssh` asks %s, or give profile %s an ssh_key":                                                 "你的设置里看不出远程 %s 是以哪个账号推送的（%s）；`gitpersona verify --ssh` 会询问 %s，也可以给配置文件 %s 设置 ssh_key",
		"remote %s pushes as %s, which the repository chooses itself (%s)":                                                                                                                 "远程 %s 以 %s 推送，这是仓库自己选的（%s）",
		"remote %s pushes as %s, not as the profile's %s (%s)":                                                                                                                             "远程 %s 以 %s 推送，而不是配置文件的 %s（%s）",
		"%s is not signed in to gh on %s, so pushes to remote %s fail; run `gh auth login --hostname %s`":                                                                                  "%s 没有在 %s 上登录 gh，所以推送到远程 %s 会失败；请运行 `gh auth login --hostname %s`",
		"if %s refuses the first key, ssh also offers %s, the key of %s":                                                                                                                   "如果 %s 拒绝第一个密钥，ssh 还会提供 %s，它是 %s 的密钥",
		"remote %s belongs to your account %s, but pushes go as %s":                                                                                                                        "远程 %s 属于你的账号 %s，但推送用的是 %s",

		// verify_ssh.go
		"remote %s: ssh shares one connection to %s (ControlMaster, ControlPath %s) with pushes that use %s; a push here can go out over it, signed in with that key instead of %s. Set `ControlMaster no` for %s in ~/.ssh/config, or give the profile an ssh_key (GitPersona's SSH command never shares a connection)": "远程 %s：ssh 到 %s 的连接是共用的（ControlMaster，ControlPath %s），与使用 %s 的推送共用；这里的推送可能走这条连接，以那个密钥而不是 %s 登录。请在 ~/.ssh/config 里为 %s 设置 `ControlMaster no`，或给配置文件设置 ssh_key（GitPersona 的 SSH 命令从不共用连接）",

		// whoami.go
		"this repository does not use profile %s: %s":                                                             "这个仓库没有使用配置文件 %s：%s",
		"the environment variable %s overrides the profile; unset it (for example in your direnv or shell setup)": "环境变量 %s 覆盖了配置文件；请取消设置（例如在 direnv 或 shell 设置里）",
		"this repository sets user.email %q itself; remove it with `git config --local --unset user.email`":       "这个仓库自己设置了 user.email %q；请用 `git config --local --unset user.email` 删除",
		"this worktree sets user.email %q itself; remove it with `git config --worktree --unset user.email`":      "这个工作树自己设置了 user.email %q；请用 `git config --worktree --unset user.email` 删除",
		"Git currently uses %q from %s":                                                      "Git 目前使用 %q（来自 %s）",
		"Git currently uses %q; run `gitpersona doctor`":                                     "Git 目前使用 %q；请运行 `gitpersona doctor`",
		"Git could not read its settings here (%s); run `git config --list` here to see why": "Git 无法读取这里的设置（%s）；请在这里运行 `git config --list` 查看原因",
		"  author  unknown: Git could not read its settings here":                            "  作者    未知：Git 无法读取这里的设置",
		"Not inside a Git repository (%s).":                                                  "不在 Git 仓库里（%s）。",
		"No GitPersona profile applies here; Git uses its own settings.":                     "这里没有适用的 GitPersona 配置文件；Git 使用它自己的设置。",
		"  author  %s <%s>%s":                                                                "  作者    %s <%s>%s",
		"Repositories created or cloned here will use %s (profile %s, %s).":                  "在这里新建或克隆的仓库将使用 %s（配置文件 %s，%s）。",
		"%s  (profile %s · %s on %s)":                                                        "%s  （配置文件 %s · %s，主机 %s）",
		" (from GIT_AUTHOR_EMAIL)":                                                           "（来自 GIT_AUTHOR_EMAIL）",
		"  reason  %s":                                                                       "  原因    %s",
		"  path    %s":                                                                       "  路径    %s",
		"  warning %s":                                                                       "  警告    %s",
		" (email from %s)":                                                                   "（邮箱来自 %s）",

		// state.go
		"link to %s":                           "链接到 %s",
		"the GitHub CLI (gh) is not installed": "未安装 GitHub CLI（gh）",

		// list.go
		"%s  %s on %s <%s>": "%s  %s，位于 %s <%s>",
		"  dir %s":          "  目录 %s",
		"  pin %s":          "  钉住 %s",
	})
}
