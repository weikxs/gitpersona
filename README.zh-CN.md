# GitPersona

[English](README.md) | 简体中文

**跟着目录自动切换的 Git 身份。**

在 `~/Projects/acme` 里，你用公司账户提交、推送、开 PR；
换到 `~/Projects/personal`，你又自动变回个人账户。不用执行任何命令，也不用记着切换。

> **状态：早期开发中，尚未发布。** GitHub、GitLab、Bitbucket 及其他主机的身份、命令行、配置 Web 页面、shell 集成和 `gh` / `glab` 切换已可用。

## 为什么需要

很多开发者在同一台电脑上用多个 Git 账户：个人、公司、客户。
Git、`gh`、`glab` 都假定只有一个全局的"当前账户"，于是很容易用错邮箱提交、用错令牌推送，或者用错账户开 PR。

## GitPersona 将会做什么

- **一个目录，一个身份。** 把目录绑定到一个 Profile，里面所有仓库都跟着用这个身份。
- **一套规则管全部：** 提交作者和邮箱、HTTPS 凭据、SSH 密钥、提交签名，以及 `gh` / `glab` 账户。
- **到处都生效：** fish、zsh、bash，IDE 和图形化 Git 客户端，脚本和非交互式 shell。
  借助 Git 自带的 `includeIf`，而不是改写你的全局配置。
- **从不保存令牌。** 凭据始终留在 `gh`、`glab`、系统钥匙串和 ssh-agent 里。
- **一分钟完成配置。** 本地 Web 页面自动发现你已登录的账户，并根据你现有的仓库推荐规则，你只需确认。
- **仓库钉住（Repo Pin）：** 给放在常规目录之外的个别仓库单独指定身份。

```text
~/Projects/personal/**       → octo-alex    （个人，GitHub）
~/Projects/acme/**           → acme-alex    （公司，GitHub）
~/Projects/clients/globex/** → globex-alex  （客户，自建 GitLab）
其他所有目录                   → acme-alex    （默认）
```

## 快速开始

1. 用 GitHub CLI 登录每一个 GitHub 账户：`gh auth login`（每个账户执行一次）。
2. 运行 `gitpersona setup`（或在浏览器里运行 `gitpersona setup --web`）。它会读取你现有的配置，一步整理好：
   - 为 `gh` 已登录的每个账户建立身份，使用账户公开的名字和邮箱（没有公开邮箱时用 noreply 地址）；
   - 接管已有的 `includeIf` 身份，连同其中的 SSH 密钥和签名设置（还设置了其他内容的规则，如 `url.*.insteadOf`，会保留）；
   - 根据常用代码目录（`~/code`、`~/Projects` 等）中仓库的远程地址和仓库邮箱确定目录归属；
   - 读取 `~/.ssh/config` 中的主机别名（如 `git@github-work:…`）及其密钥路径（密钥文件本身从不打开）；
   - 为与所在目录不一致的仓库建议 Pin，并为登录 Shell 加上集成。

   整理结果会先展示出来，确认一次即可；写入前全部备份。无法判断归属的组织目录会列出来让你选择。在浏览器里可以点 "Adjust step by step"（逐步调整）修改任何一项后再应用。
   或者自己描述你的身份：

   ```toml
   version = 1
   default = "acme"            # 所有规则都不匹配时使用（可选）

   [[profile]]
   id      = "personal"
   account = "octo-alex"       # 平台上的账户名
   name    = "Alex Octo"       # 提交作者名
   email   = "1000001+octo-alex@users.noreply.github.com"
   dirs    = ["~/Projects/personal"]

   [[profile]]
   id      = "acme"
   account = "acme-alex"
   name    = "Alex (Acme)"
   email   = "1000002+acme-alex@users.noreply.github.com"
   dirs    = ["~/Projects/acme"]
   ```

3. 运行 `gitpersona apply`，再运行 `gitpersona doctor`。在任意仓库里运行 `gitpersona whoami` 查看当前身份。

## 配置说明

| 字段 | 必填 | 含义 |
|---|---|---|
| `version` | 是 | 固定为 `1` |
| `default` | 否 | 没有任何规则或 Pin 匹配时使用的身份 |
| `shell_integration` | 否 | 设为 `false` 表示不在 Shell 中加载 GitPersona，`doctor` 不再建议安装（`setup --no-shell` 会写入） |
| `[[profile]] id` | 是 | 短名：小写字母、数字、`-` |
| `provider` | 否 | 默认 `github`，也可为 `gitlab`、`bitbucket`、`generic`；后三者的令牌按账户存在你自己的凭据存储里（`gitpersona auth login`） |
| `host` | 否 | GitHub 默认为 `github.com` |
| `account`、`name`、`email` | 是 | 平台账户名、提交作者名、提交邮箱 |
| `dirs` | 否 | 这些目录里的仓库使用此身份；最具体的目录优先 |
| `ssh_key` | 否 | 此身份使用的 SSH 私钥路径 |
| `signing` | 否 | `{ method = "ssh", key = "~/.ssh/key.pub" }`、`{ method = "gpg", key = "<密钥 ID>" }` 或 `{ method = "none" }` |
| `[[pin]] repo`、`profile` | — | 把单个仓库绑定到某个身份，优先于目录规则 |

路径必须是绝对路径，或以 `~/` 开头。

## 命令

| 命令 | 作用 |
|---|---|
| `gitpersona apply [--dry-run] [--yes]` | 生成 Git 配置；`--dry-run` 只预览，`--yes` 允许删除已移除身份的文件 |
| `gitpersona whoami [--json] [--check]` | 显示这里 Git 使用哪个身份以及原因；`--check` 在 Git 不一致时返回失败 |
| `gitpersona use <身份>` | 让当前目录使用某个身份（保留 config.toml 里的注释） |
| `gitpersona remove [目录]` | 取消某个目录的身份分配 |
| `gitpersona pin <身份>` / `unpin` | 把当前仓库绑定到某个身份，或取消绑定 |
| `gitpersona list [--json]` | 列出身份、目录和 Pin |
| `gitpersona doctor [--fix] [--json]` | 检查配置并说明如何修复问题；`--fix` 自动修复能安全修复的问题 |
| `gitpersona auth login\|logout\|status [--profile <id>]` | 在你自己的凭据存储（系统钥匙串或 Git Credential Manager）里为 GitLab、Bitbucket 等配置文件保存、删除或检查令牌，每个账户一个；缺令牌时 `status` 返回失败 |
| `gitpersona check-ssh [--profile <id>]` | 用每个配置文件的 SSH 密钥连接它的主机，报告主机认出的账户；只在你主动运行时执行，GitPersona 不打开密钥（由 ssh 使用） |
| `gitpersona shells [--json]` | 显示默认 shell、当前正在用的 shell 和已安装的 shell |
| `gitpersona install-shell [--shell fish,zsh,bash]` | 为 shell 加上提示符显示（以及 gh / glab 切换）；默认只装你的登录 shell |
| `gitpersona prompt [--format]` | 输出这里 Git 使用的账户，供提示符使用（`{account}`、`{profile}`、`{host}`） |
| `gitpersona rollback [<名称>\|latest] [--yes]` | 列出备份，或恢复某个备份（恢复前会先备份当前状态） |
| `gitpersona uninstall [--yes]` | 从 Git 配置中移除 GitPersona，并放回它接管过的 `includeIf` 规则；保留 config.toml |
| `gitpersona setup [--yes] [--no-shell] [--keep-old-rules] [--default <配置文件> \| --no-default] [--print \| --web]` | 根据现有配置一步完成设置（账户、已有身份、仓库、SSH 别名、Shell）；写入前询问，`--yes` 不询问；`--web` 在浏览器中展示，也可以逐步调整 |
| `gitpersona ui [--no-open]` | 在浏览器中打开配置页面：身份、目录、Pin、应用前的差异预览、体检、shell 和备份。只监听 127.0.0.1，链接只能用一次，15 分钟无操作自动退出 |
| `gitpersona help <命令>` | 查看某个命令的参数 |

提示符和插件管理器（tide、Powerlevel10k、Starship、fisher、oh-my-zsh）的配置方法：见 [docs/integrations.md](docs/integrations.md)。

## 行为说明

- **优先级：** Pin → 最具体的目录规则 → `default`。Git 按仓库 `.git` 目录的位置应用规则，所以 **worktree 跟随它的主仓库**。在任何仓库之外只有 `default` 生效；`whoami` 还会告诉你在这里新克隆的仓库会用哪个身份。
- **你自己的 `includeIf` 规则：** GitPersona 的规则排在你的规则之后，所以在你分给配置文件的目录（以及 Pin）里以配置文件为准；其他地方你的规则完全照旧。默认身份在所有地方生效，因此只要你还有自己的身份规则，`setup` 就不设默认身份（`--default <配置文件>` 可以强制设置，并会说明它覆盖了你的哪些规则）。做了配置文件做不到的事的规则（x509 签名、带端口或代理的 SSH 命令、只用于 tag 的密钥，不论是否设置身份），连同包含它们的外层目录，都留给你的规则。按 Git 实际的读取顺序（`~/.config/git/config`、`~/.gitconfig` 及其 include 的文件）本来就不生效的规则（被你后面的规则或全局身份覆盖）不受保护。配置文件只在可能沿用其他身份密钥的地方重置签名和 SSH 命令。属于另一个已登录账户的仓库会 Pin 到该账户，推送时使用该账户。`setup` 会删除被配置文件完全接管的规则，`uninstall` 会把它们放回原来后面那一节（或那条规则）之前。`doctor` 会说明你的哪些规则被配置文件覆盖。
- **凭据：** 对 GitHub 身份，GitPersona 是该主机唯一的凭据助手，借用 `gh` 已保存的令牌，因此不会有其他助手另存一份。如果某个账户没有在 `gh` 登录，Git 会要求输入密码；`gitpersona doctor` 会告诉你该登录哪个账户。对 GitLab、Bitbucket 等主机，像 `gh` 对 GitHub 那样，由你自己的凭据助手（系统钥匙串、Git Credential Manager）按"主机 + 账户"各保存一个令牌：`gitpersona auth login` 把令牌存进去（输入不回显，绝不写入明文文件），每个配置文件只指定账户名，由对应的令牌应答。GitPersona 不保存任何副本。在 GitLab 配置文件的目录里运行 `glab` 时，也会用该账户的令牌（只放进那一次进程的 `GITLAB_TOKEN`），除非你自己设置了令牌或其他主机。即使没有 `gh`，`gitpersona setup` 也会把你已有的 GitLab、Bitbucket 或公司服务器身份变成配置文件：`includeIf` 文件里写明了账户（`credential.https://<主机>.username`）的，或者其仓库都以同一用户推送到同一台这类主机的。
- **gh 也跟着目录走：** 装上 shell 集成后，在某个身份的目录里运行 `gh`，就会使用该身份的账户。GitPersona 只把该账户的令牌放进那一次 `gh` 进程的环境变量，不输出、不保存。当你自己设置了 `GH_TOKEN`/`GITHUB_TOKEN`、用 `GH_HOST` 指向其他主机，或没有 GitHub 身份适用时，不做任何改动。
- **Shell：** 默认 shell 从系统用户记录读取（而不是 `$SHELL`，终端、IDE 和各种工具经常会改它），当前正在用的 shell 从进程关系识别，已安装的 shell 从 `/etc/shells` 读取。fish 使用 `conf.d` 下的独立文件；zsh 使用 `~/.zshenv`；bash 使用 `~/.bashrc`。tide 中添加 `gitpersona` 显示项；Powerlevel10k 中添加 `gitpersona` 段；其他提示符调用 `gitpersona_prompt`。
- **隐私：** GitPersona 从不打开、读取、列出或使用你的私钥，也从不输出或保存令牌。
- **你的文件：** 只在全局 Git 配置里加一个带标记的块（会跟随符号链接形式的 `~/.gitconfig` 和 `GIT_CONFIG_GLOBAL`），改动前备份，并在 `~/.config/gitpersona/backups/` 保留最近 20 份备份。

## 安装

尚未发布。正式版将通过 Homebrew 发布：

```sh
brew install weikxs/tap/gitpersona
```

从源码构建（需要 Go 1.25 或更高版本）：

```sh
git clone https://github.com/weikxs/gitpersona.git
cd gitpersona
make build
./bin/gitpersona version
```

## 参与贡献

欢迎贡献。请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 和[开发规范](docs/development/standards.md)。

## 安全

请私下报告安全漏洞，详见 [SECURITY.md](SECURITY.md)。

## 许可证

[MIT](LICENSE)
