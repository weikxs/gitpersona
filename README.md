# GitPersona

English | [简体中文](README.zh-CN.md)

**Git profiles that follow your directories.**

Work in `~/Projects/acme` and you commit, push and open pull requests as your work account.
Switch to `~/Projects/personal` and you are your personal account again. No commands to run, nothing to remember.

> **Status: early development.** The core engine (`apply`, `whoami`, `doctor`, GitHub credentials) works; shell integration, the web UI and other providers are in progress. Not released yet.

## Why

Many developers use several Git accounts on one machine: personal, work, and client accounts.
Git, `gh` and `glab` each assume one global "current account", so it is easy to commit with the wrong email,
push with the wrong token, or open a pull request as the wrong user.

## What GitPersona will do

- **One directory, one identity.** Bind directories to a profile; every repository inside follows it.
- **One rule set for everything:** commit author and email, HTTPS credentials, SSH key, commit signing,
  and the `gh` / `glab` account.
- **Works everywhere:** fish, zsh and bash, IDEs and GUI Git clients, scripts and non-interactive shells.
  It uses Git's own `includeIf` instead of rewriting your global config.
- **Never stores tokens.** Credentials stay in `gh`, `glab`, the system keychain and ssh-agent.
- **Configure in a minute.** A local web page discovers the accounts you are signed in to and suggests
  rules from the repositories you already have; you just confirm.
- **Repo pins** for the odd repository that lives outside its usual directory.

```text
~/Projects/personal/**      → octo-alex    (personal, GitHub)
~/Projects/acme/**          → acme-alex    (work, GitHub)
~/Projects/clients/globex/** → globex-alex (client, self-hosted GitLab)
everything else             → acme-alex    (default)
```

## Install

Not released yet. Releases will be published through Homebrew:

```sh
brew install weikxs/tap/gitpersona
```

To build from source (Go 1.25 or newer):

```sh
git clone https://github.com/weikxs/gitpersona.git
cd gitpersona
make build
./bin/gitpersona version
```

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) and the
[development standards](docs/development/standards.md) first.

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
