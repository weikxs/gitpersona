package i18n

// Messages of internal/app/setup.go, setup_plan.go, setup_infer.go,
// setup_claim.go, setup_auto.go, old_rules.go, global_files.go and
// gh_switchers.go.
func init() {
	register(map[string]string{
		// setup.go: what `gitpersona setup` and the setup page say.
		"%s already exists; change it on the configuration page (`gitpersona ui`), with `gitpersona use` and `gitpersona pin`, " +
			"or by hand (after `gitpersona uninstall`, `gitpersona apply` sets GitPersona up again from it)": "%s 已存在；请在配置页（`gitpersona ui`）中修改，或使用 `gitpersona use` 和 `gitpersona pin`，" +
			"或手动修改（先运行 `gitpersona uninstall`，之后 `gitpersona apply` 会根据它重新配置 GitPersona）",
		"choose either --default or --no-default":                     "--default 和 --no-default 只能选一个",
		"--default %s: no such profile; setup proposes %s":            "--default %s：没有这个配置文件；配置建议的有 %s",
		"Apply this setup? Changed files are backed up first. [Y/n] ": "应用这个配置吗？改动的文件会先备份。[Y/n] ",
		"Nothing was written. Run `gitpersona setup --yes` to apply without asking, " +
			"or `gitpersona setup --web` to adjust it in your browser.": "没有写入任何内容。运行 `gitpersona setup --yes` 可不经询问直接应用，" +
			"或运行 `gitpersona setup --web` 在浏览器中调整。",
		"GitPersona is set up. Check it with `gitpersona doctor`; change anything with `gitpersona ui`.": "GitPersona 已配置完成。用 `gitpersona doctor` 检查，用 `gitpersona ui` 修改任何内容。",
		"Open a new terminal so gh and the prompt follow your directories.":                              "打开新的终端，让 gh 和提示符跟随你的目录。",

		"GitPersona found %s, %s for other hosts in your Git settings and looked for repositories in %s:": "GitPersona 找到了 %s，以及你的 Git 设置中其他主机的 %s，并在 %s 中查找了仓库：",
		"GitPersona found %s, %s for other hosts in your Git settings:":                                   "GitPersona 找到了 %s，以及你的 Git 设置中其他主机的 %s：",
		"GitPersona found %s and looked for repositories in %s:":                                          "GitPersona 找到了 %s，并在 %s 中查找了仓库：",
		"GitPersona found %s:": "GitPersona 找到了 %s：",
		"1 GitHub account":     "1 个 GitHub 账户",
		"%d GitHub accounts":   "%d 个 GitHub 账户",
		"1 account":            "1 个账户",
		"%d accounts":          "%d 个账户",

		"GitHub accounts were not looked up: %s.": "没有查询 GitHub 账户：%s。",
		"Looking for repositories stopped after %s, before it finished %s; repositories it did not reach " +
			"are not included. Add their folders with `gitpersona use`.": "查找仓库在 %s 后停止了，没来得及完成 %s；没有查到的仓库不包含在内。" +
			"请用 `gitpersona use` 添加它们的目录。",
		"Repositories nested more than %d folders deep were not looked at; add their folders with `gitpersona use`.": "嵌套超过 %d 层目录的仓库没有查找；请用 `gitpersona use` 添加它们的目录。",
		"Pin %s to %s: %s": "把 %s 钉住到 %s：%s",
		"No default profile: one would override your own includeIf rules everywhere (%s). Outside these " +
			"directories Git keeps your own settings; `gitpersona setup --default %s` sets one anyway.": "没有默认配置文件：默认配置文件会在所有位置覆盖你自己的 includeIf 规则（%s）。在这些" +
			"目录之外，Git 保留你自己的设置；`gitpersona setup --default %s` 可以强行设置一个。",
		"The default profile %s overrides your own includeIf rules everywhere: %s.":                                                                                "默认配置文件 %s 会在所有位置覆盖你自己的 includeIf 规则：%s。",
		"The profiles take over your includeIf rules for %s; they are removed from your Git configuration (--keep-old-rules to keep them).":                        "配置文件接管了你在 %s 的 includeIf 规则；这些规则会从你的 Git 配置中移除（用 --keep-old-rules 保留）。",
		"Kept your includeIf rule for %s, which stays in charge there: %s.":                                                                                        "保留了你在 %s 的 includeIf 规则，它在那里继续生效：%s。",
		"Except in %s, which go to profiles (what the rule does more is lost there).":                                                                              "例外：%s 归配置文件管理（规则多做的部分在那里会丢失）。",
		"Your includeIf rule for %s does more than a profile can (%s), but profile %s applies there, " + "so that is lost; take %s out of the profile to keep it.": "你在 %s 的 includeIf 规则做的比配置文件能做的更多（%s），但那里使用的是配置文件 %s，" + "所以多出来的部分会丢失；要保留它，请把 %s 从该配置文件中去掉。",
		"Changed: %s.": "变更：%s。",
		"Shell integration for %s, so gh and your prompt follow the directory (--no-shell to skip).": "为 %s 配置 shell 集成，让 gh 和提示符跟随目录（用 --no-shell 跳过）。",
		"Note: %s.": "注意：%s。",
		"Outside these directories, commits will use %s (%s <%s>) instead of %s <%s> (--no-default to keep your global Git identity).": "在这些目录之外，提交将使用 %s（%s <%s>），而不是 %s <%s>（用 --no-default 保留你的全局 Git 身份）。",
		"Outside these directories, commits will use %s (%s <%s>) (--no-default to keep your global Git identity).":                    "在这些目录之外，提交将使用 %s（%s <%s>）（用 --no-default 保留你的全局 Git 身份）。",
		"There, commits will also be signed (%s), as %s signs every commit.":                                                           "在那里，提交也会被签名（%s），因为 %s 会为每个提交签名。",
		"There, Git also connects with %s's SSH key %s (instead of your own SSH settings).":                                            "在那里，Git 还会使用 %s 的 SSH 密钥 %s 连接（而不是你自己的 SSH 设置）。",
		"Not taken over: %s for %s (from %s). It never applies today, because %s; setup leaves it as it is.":                           "未接管：%s 在 %s 的规则（来自 %s）。它现在从未生效，因为%s；配置不会改动它。",
		"Replaced: %s for %s (from %s). Profile %s (%s) applies there instead, because GitPersona's rules " +
			"come after yours. To keep it, choose its account with `gitpersona setup --web`.": "已替换：%s 在 %s 的规则（来自 %s）。那里改用配置文件 %s（%s），因为 GitPersona 的规则排在你的规则之后。" +
			"要保留它，请用 `gitpersona setup --web` 选择它的账户。",
		"Not taken over: %s for %s (from %s); GitPersona could not tie it to one account, so your rule keeps " +
			"applying there. To make it a profile, choose its account with `gitpersona setup --web`.": "未接管：%s 在 %s 的规则（来自 %s）；GitPersona 无法把它对应到某一个账户，所以你的规则在那里继续生效。" +
			"要把它变成配置文件，请用 `gitpersona setup --web` 选择它的账户。",
		"Except in %s, which go to profiles.": "例外：%s 归配置文件管理。",
		"Owned by someone else, so the default profile %s applies there unless you choose (run `gitpersona use <profile>` in the folder):": "属于别人，所以除非你另行选择，那里使用默认配置文件 %s（在该目录中运行 `gitpersona use <profile>`）：",
		"Owned by someone else, so your own Git settings apply there unless you choose (run `gitpersona use <profile>` in the folder):":    "属于别人，所以除非你另行选择，那里使用你自己的 Git 设置（在该目录中运行 `gitpersona use <profile>`）：",
		"%s  (%s, owned by %s)": "%s（%s，属于 %s）",
		"%s (default)":          "%s（默认）",
		"account %s on %s (%s)": "账户 %s，位于 %s（%s）",
		"(the account shows no public name, so the name is its login)":             "（该账户没有公开姓名，所以姓名使用它的登录名）",
		"(your includeIf setup sets no name, so the name is your global Git name)": "（你的 includeIf 设置没有指定姓名，所以姓名使用你的全局 Git 姓名）",
		"signs commits (%s)":        "提交会签名（%s）",
		"%s  (your existing setup)": "%s（你现有的设置）",
		"%s  (%s)":                  "%s（%s）",
		"no directories yet":        "还没有目录",

		// setup_plan.go
		"%w\n(GitPersona also found %s in your Git settings, but none names one account " +
			"on one host other than GitHub; add credential.https://<host>.username to its includeIf file, or write the profile in " +
			"config.toml yourself)": "%w\n（GitPersona 在你的 Git 设置中还找到了 %s，但没有一个指明 GitHub 以外某个主机上的某个账户；" +
			"请在它的 includeIf 文件中添加 credential.https://<host>.username，或自己在 config.toml 中编写该配置文件）",
		"setup needs the GitHub CLI: install it from https://cli.github.com and run `gh auth login` for each account": "配置需要 GitHub CLI：请从 https://cli.github.com 安装，并为每个账户运行 `gh auth login`",
		"no GitHub account is signed in to gh; run `gh auth login` for each account, then try again":                  "gh 中没有已登录的 GitHub 账户；请为每个账户运行 `gh auth login`，然后重试",
		"%s already exists; change it on the Profiles page":                                                           "%s 已存在；请在“配置文件”页修改",
		"choose at least one profile":                           "请至少选择一个配置文件",
		"the profiles cannot be written as a configuration: %w": "这些配置文件无法写成配置：%w",

		// A group of its own: gofmt 1.26 and 1.27 align a long run of keys differently.
		"profile %s: %w": "配置文件 %s：%w",
		"1 identity":     "1 个身份",
		"%d identities":  "%d 个身份",

		// setup_auto.go
		"the SSH key %s of your includeIf setup does not exist, so it is left out":          "你的 includeIf 设置中的 SSH 密钥 %s 不存在，因此未采用",
		"the signing key %s of your includeIf setup does not exist, so signing is left out": "你的 includeIf 设置中的签名密钥 %s 不存在，因此不启用签名",

		// setup_claim.go
		"%s for %s":                         "%s 在 %s",
		"your rule for %s":                  "你在 %s 的规则",
		"it contains your rule for %s (%s)": "它包含你在 %s 的规则（%s）",
		"your rule for %s (%s) never applied, because %s; after setup %s uses %s":                                                                   "你在 %s 的规则（%s）从未生效，因为%s；配置之后 %s 将使用 %s",
		"your rule %s (%s) no longer applies in your profiles' folders (%s), where the profile decides: 1 repository there matches it, %s":          "你的规则 %s（%s）在你的配置文件目录（%s）中不再生效，由配置文件决定：那里有 1 个仓库匹配它，%s",
		"your rule %s (%s) no longer applies in your profiles' folders (%s), where the profile decides: %d repositories there match it, such as %s": "你的规则 %s（%s）在你的配置文件目录（%s）中不再生效，由配置文件决定：那里有 %d 个仓库匹配它，例如 %s",
		"your rule %s (%s) no longer applies in your profiles' folders (%s), where the profile decides":                                             "你的规则 %s（%s）在你的配置文件目录（%s）中不再生效，由配置文件决定",

		// setup_infer.go: why a folder or a pin is proposed.
		"%s (the scan stopped in this folder; it may hold more)": "%s（扫描在这个目录停止了，里面可能还有更多）",
		"the repository %s":      "该仓库%s",
		"its one repository":     "它唯一的仓库",
		"all %d repositories":    "全部 %d 个仓库",
		"%d of %d repositories":  "%d/%d 个仓库",
		"%s belong to %s":        "%s属于 %s",
		"%s belongs to %s":       "%s属于 %s",
		"it %s":                  "它%s",
		"all %s":                 "全部%s",
		"1 %s":                   "1 个%s",
		"%d %s":                  "%d 个%s",
		"%s; %s":                 "%s；%s",
		"sets %s":                "设置了 %s",
		"set %s":                 "设置了 %s",
		"uses %s's SSH alias %s": "使用 %s 的 SSH 别名 %s",
		"use %s's SSH alias %s":  "使用 %s 的 SSH 别名 %s",
		"signs in as %s":         "以 %s 登录",
		"sign in as %s":          "以 %s 登录",
		"belongs to %s":          "属于 %s",
		"belong to %s":           "属于 %s",

		// old_rules.go
		"Kept the includeIf rule for %s: %s.":                                           "保留了 %s 的 includeIf 规则：%s。",
		"No old includeIf identity rules that a profile takes over; nothing to change.": "没有配置文件接管的旧 includeIf 身份规则；无需更改。",
		"Removed the old includeIf rule for %s.":                                        "已移除 %s 的旧 includeIf 规则。",

		// global_files.go: why a rule of the user's stays, or never applies.
		"its includeIf section includes more than one file":  "它的 includeIf 段包含不止一个文件",
		"its core.sshCommand has other options":              "它的 core.sshCommand 还有其他选项",
		"it sets a signing key without signing every commit": "它设置了签名密钥，但没有为每个提交签名",
		"it names accounts for more than one host":           "它为不止一个主机指定了账户",
		"it also sets %s":           "它还设置了 %s",
		"it signs with %s":          "它使用 %s 签名",
		"it sets %s for its folder": "它为所在目录设置了 %s",
		"the identity set for everything in %s comes after it": "%s 中为所有位置设置的身份排在它之后",
		"your later rule for %s (%s) overrides it":             "你之后的规则 %s（%s）覆盖了它",

		// gh_switchers.go
		"you already switch gh accounts in %s, and GitPersona's integration switches gh by profile too. " +
			"GitPersona leaves gh alone wherever GH_TOKEN is set, so where your setup sets it, yours decides; " +
			"keeping one of the two avoids surprises": "你已经在 %s 中切换 gh 账户，而 GitPersona 的集成也会按配置文件切换 gh。" +
			"只要设置了 GH_TOKEN，GitPersona 就不会动 gh，所以在你的设置里设置了它的地方，以你的为准；" +
			"两者保留其一可以避免意外",
	})
}
