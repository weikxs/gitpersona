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

## Quick start

1. Sign in to every GitHub account with the GitHub CLI: `gh auth login` (once per account).
2. Describe your profiles in `~/.config/gitpersona/config.toml`:

   ```toml
   version = 1
   default = "acme"            # used everywhere no rule matches (optional)

   [[profile]]
   id      = "personal"
   account = "octo-alex"       # your account on the host
   name    = "Alex Octo"       # commit author name
   email   = "1000001+octo-alex@users.noreply.github.com"
   dirs    = ["~/Projects/personal"]

   [[profile]]
   id      = "acme"
   account = "acme-alex"
   name    = "Alex (Acme)"
   email   = "1000002+acme-alex@users.noreply.github.com"
   dirs    = ["~/Projects/acme"]
   ```

3. Run `gitpersona apply`, then `gitpersona doctor`. In any repository, `gitpersona whoami` shows who you are.

## Configuration reference

| Key | Required | Meaning |
|---|---|---|
| `version` | yes | Always `1` |
| `default` | no | Profile id used where no rule or pin matches |
| `[[profile]] id` | yes | Short name: lowercase letters, digits, `-` |
| `provider` | no | `github` (default); `gitlab`, `bitbucket`, `generic` set the commit identity and SSH key only, for now |
| `host` | no | Defaults to `github.com` for GitHub |
| `account`, `name`, `email` | yes | Account on the host, commit author name and email |
| `dirs` | no | Directories whose repositories use this profile; the most specific directory wins |
| `ssh_key` | no | Path of the SSH private key Git should use for this profile |
| `signing` | no | `{ method = "ssh", key = "~/.ssh/key.pub" }`, `{ method = "gpg", key = "<key id>" }`, or `{ method = "none" }` |
| `[[pin]] repo`, `profile` | — | Bind one repository to a profile, overriding directory rules |

Paths must be absolute or start with `~/`.

## Commands

| Command | What it does |
|---|---|
| `gitpersona apply [--dry-run] [--yes]` | Generate the Git configuration; `--dry-run` previews, `--yes` allows removing files of deleted profiles |
| `gitpersona whoami [--json]` | Show which profile Git uses here and why |
| `gitpersona use <profile>` | Make the current directory use a profile (keeps your comments in config.toml) |
| `gitpersona remove [dir]` | Stop assigning a directory to a profile |
| `gitpersona pin <profile>` / `unpin` | Bind the current repository to a profile, or remove the pin |
| `gitpersona list [--json]` | List profiles, directories and pins |
| `gitpersona doctor` | Check the setup and explain how to fix problems |
| `gitpersona shells [--json]` | Show your default shell, the shell you use right now, and installed shells |
| `gitpersona install-shell [--shell fish,zsh,bash]` | Add prompt display (and gh switching) to your shells; defaults to your login shell |
| `gitpersona prompt [--format]` | Print the account Git uses here, for prompts (`{account}`, `{profile}`, `{host}`) |
| `gitpersona rollback [<name>\|latest] [--yes]` | List backups, or restore one (the current state is backed up first) |
| `gitpersona uninstall [--yes]` | Remove GitPersona from your Git configuration; keeps config.toml |
| `gitpersona help <command>` | Show the flags of a command |

## How it behaves

- **Precedence:** a pin, then the most specific directory rule, then `default`. Git applies the rules by the
  location of the repository's `.git` directory, so a **worktree follows its main repository**. Outside any
  repository only `default` applies; `whoami` also tells you which profile new clones will get.
- **Credentials:** for GitHub profiles, GitPersona is the only credential helper for that host and borrows the
  token `gh` already holds, so no other helper stores a copy. If an account is not signed in to `gh`, Git asks
  for a password; `gitpersona doctor` tells you which account to sign in.
- **Shells:** your default shell is read from the system user record (not `$SHELL`, which terminals, IDEs and
  tools often change), the shell you use right now from the process tree, and installed shells from `/etc/shells`.
  fish gets its own file in `conf.d`; zsh uses `~/.zshenv`; bash uses `~/.bashrc`. In tide add the
  `gitpersona` item; in Powerlevel10k add the `gitpersona` segment; elsewhere call `gitpersona_prompt`.
- **Privacy:** GitPersona never opens, reads, lists or uses your private keys, and never prints or stores tokens.
- **Your files:** it adds one marked block to your global Git configuration (following a symlinked
  `~/.gitconfig` and `GIT_CONFIG_GLOBAL`), backs up everything it changes, and keeps the newest 20 backups in
  `~/.config/gitpersona/backups/`.

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
