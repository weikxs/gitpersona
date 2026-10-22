# GitPersona

[English](README.md) | 简体中文

**跟着目录自动切换的 Git 身份。**

在 `~/Projects/acme` 里，你用公司账户提交、推送、开 PR；
换到 `~/Projects/personal`，你又自动变回个人账户。不用执行任何命令，也不用记着切换。

> **状态：早期开发中。** 核心引擎（`apply`、`whoami`、`doctor`、GitHub 凭据）已可用；shell 集成、Web 页面和其他平台正在开发中。尚未发布。

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
