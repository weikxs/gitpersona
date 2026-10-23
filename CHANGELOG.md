# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- GitHub profiles no longer chain the user's own credential helpers, which could store a copy of the `gh` token.
- `doctor` checks sign-in with `gh auth status` and never fetches a token; private keys are never read.

### Added

- One-step setup: `gitpersona setup` and `setup --web` look at what you already have — the accounts signed in to gh,
  existing `includeIf` identities with their SSH keys and signing, the remotes and emails of the repositories in your
  usual code folders, and SSH host aliases in `~/.ssh/config` (key files are only checked to exist, never opened) — and
  organize them into profiles, directory rules, pins, the default profile and the login shell's integration. The page
  opens on the finished proposal ("Your setup is ready"; one click applies it, "Adjust step by step" changes it); the
  command line shows it and asks once (`--yes`, `--no-shell`, `--keep-old-rules`). The `includeIf` rules the profiles
  fully replace are removed in the same backup; rules whose files also set other things (such as `url.*.insteadOf`)
  stay, and setup says why. A first setup takes seconds instead of minutes.
- `setup --web` and the page's setup steps: confirm one profile per signed-in account (reusing an existing `includeIf`
  setup), choose directories with suggestions from repository remotes, review every file including `config.toml`,
  then apply in one backup and see the health checks.
- `ui`: a local configuration page for profiles, directories and pins, with a diff preview before applying,
  health checks, shells and backups. It listens only on 127.0.0.1, uses a one-time link, CSRF protection and a strict
  Content Security Policy, loads nothing from outside, works at phone width and in dark mode.
- `use`, `remove`, `pin`, `unpin` and `list [--json]`: edit the configuration from the command line while keeping
  your comments and layout; every edit is verified, backed up and applied. `doctor` warns about pins to missing repositories.
- `rollback` lists backups and restores one (each command's changes are one backup, so a rollback undoes the whole
  operation); `uninstall` removes GitPersona from your Git configuration and keeps `config.toml`.
- Shell integration: `shells` shows the default (login) shell, the shell in use right now and the installed shells;
  `install-shell` adds the integration (to the login shell by default); `init <shell>` prints it; `prompt` prints the
  account for prompts in a few milliseconds, with items for tide and Powerlevel10k. `doctor` checks both the default
  and the current shell, and `uninstall` removes the integration.
- `gh` follows your directories: with the shell integration, `gh` runs as the account of the current directory's
  profile; the token goes only to that one `gh` process.
- `doctor --fix` repairs what it safely can (regenerates files, moves the block last, installs the login shell's
  integration; never removes files) and `doctor --json`; `whoami --check` fails when Git does not use the profile.
- `setup` creates a first configuration from the GitHub accounts `gh` is signed in to, with each account's public
  name and email (its noreply address when the account shows no public email).
- Plugins for fisher (`conf.d/gitpersona.fish`) and oh-my-zsh / zinit / antidote (`gitpersona.plugin.zsh`), and
  prompt recipes for tide, Powerlevel10k, Starship and bash in `docs/integrations.md`.
- `apply --dry-run` and `apply --yes`; per-command help (`gitpersona help <command>`).
- Quick start, configuration reference and behavior notes in the README.
- `gitpersona apply`: generates per-profile Git configuration and a rules index of `includeIf` entries
  (default profile, then directory rules from least to most specific, then repository pins), adds one
  include to `~/.gitconfig` inside a marked block, backs up every file it changes, and does nothing when
  everything is already up to date.
- `gitpersona whoami` (and `--json`): shows which profile applies to the current repository and why,
  and warns when Git currently uses a different email.
- `gitpersona doctor`: checks the configuration, whether generated files are current, and whether each
  GitHub account is signed in to `gh`; never prints tokens.
- Git credential helper for GitHub: answers HTTPS requests with the profile's account using the token
  `gh` already holds, and stays silent otherwise so the user's own helpers still work.
- Directory rules work when a directory is reached through a symbolic link.

### Fixed

- Setup ties an existing `includeIf` identity that names no account to the right one by what is on disk: the key of
  that account's SSH alias, or the repositories in its folders or that set its email. The folders of existing rules
  are scanned too, along with `~/Developer`, `~/workspace` and `~/Documents/GitHub`. An identity it still cannot tie
  is shown with the profile that would override it, and on the page you can choose its account.
- Removing old `includeIf` rules (`doctor --fix`, the page's button, setup) only removes a rule when a profile with
  the same email covers its folder; before, a rule whose identity no profile had could be deleted. `doctor` now warns
  when a profile overrides such a rule, and reports rules kept on purpose as fine.
- Setup pins repositories of another account in subfolders without a rule of their own, a scanned folder that is a
  repository itself, and repositories kept inside other repositories; an https remote that names its user
  (`https://user@github.com/...`) counts as evidence. Reasons name every SSH alias the repositories use.
- Setup says when looking for repositories stopped at its time limit (now 10 seconds) instead of silently showing
  partial counts, and says which keys of an existing setup it left out because their files are missing.
- Declining the shell integration that setup offers (`setup --no-shell`, or unticking every shell on the page) writes
  `shell_integration = false`, so `doctor` and `doctor --fix` no longer push it.
- When some profile signs, a profile without signing resets the signing key and format to your own settings or Git's
  defaults, so `git commit -S` in a pinned repository no longer signs with another profile's key.
- Setup no longer trusts weak clues: a single repository an account owns no longer ties an
  existing identity to that account, the strongest clue wins when two identities point to one account, and a pin for
  a repository outside every rule follows the default you choose.
- `includeIf` rules written with the home's absolute path are shown and stored as `~/...`.
- The preview on the "Your setup is ready" page follows the boxes you tick or untick.
- `signing = { method = "none" }` turns commit and tag signing off even when you sign everything globally; before,
  it did nothing and commits still tried to sign.
- The configuration page compares a directory typed as an absolute path with the stored `~/` form: moving a
  directory from another profile always asks first, duplicates are caught in the setup steps, and messages show `~`.
- Error messages on the page (for example a `config.toml` that does not parse) show the home directory as `~`; with a
  broken configuration the page explains how to fix it or restore a backup instead of showing an empty Profiles view.
- The setup steps follow the browser's Back and Forward buttons and survive a reload.
- Setup step 1 names the identity used outside your directories today and offers "No default" to keep it; a name
  taken from the login (no public name on GitHub) says so.
- Hand-made `includeIf` identity rules can be removed from the page or with `doctor --fix`, after a backup; only
  rules a profile fully replaces are removed, and comments (including GitPersona's block markers) are kept.
- `~/.gitconfig` includes the rules as `~/.config/...` instead of an absolute path, so a dotfiles repository works on
  every machine (the next `apply` updates it).
- Pinning a repository reached through a symlink stores the path as you reached it.
- The Shells page lists the startup files it changes, with a preview, before writing them.
- After a failed change caused by an edit outside the page, the page shows the configuration as it now is.
- Plainer words: backups say what they were taken before ("before adding a directory to work"), success is shown
  once, and the `gh` check names the shell that started GitPersona. The "already open" message says how to stop a
  page whose tab or terminal is gone. Buttons on profile cards name their profile for screen readers, and
  `/static/` no longer lists files.
- Directory suggestions include folders whose repositories belong to an organization: the setup page lists them
  with their owner so you can choose the profile. Symlinked folders such as `~/code` are scanned, and folders that do
  not exist are named instead of reported as having no suggestions.
- Saving a profile whose SSH key or SSH signing key file does not exist is refused, and `doctor` reports it, instead of
  every commit failing later. GitPersona only checks that the file is there; it never opens it.
- `doctor` warns about directory rules for folders that do not exist and about hand-made `includeIf` identity rules
  left from before GitPersona.
- The configuration page asks before moving a directory from another profile, explains what removing the default
  profile changes, reports a restored backup as a restore, and writes nothing (and makes no backup) when a save
  changes nothing.
- Setup page: Enter in a field adds the directory or scans instead of skipping the step; a profile you leave out no
  longer blocks Next; duplicate directories are caught when added; old messages are cleared.
- Opening a second page no longer ends the first page's session, and only one page can run at a time, so two pages
  never edit the configuration together. After Close the page offers nothing that would fail.
- Diffs on the page show your home directory as `~`, so screenshots do not reveal it.
- New keys are added after a profile's last key and aligned like the others.
- Keyboard focus returns to the button or the result after an action; plainer wording on the Shells page; doctor
  advice links to the matching part of the page; `setup --web` with an existing configuration opens the overview.
- The configuration page checks the format of profile and account names before sending them: the browser had
  ignored the field patterns, because a `-` in a character class must be escaped in the patterns browsers use.
- Profiles without signing restore your own global signing settings instead of turning signing off.
- The oldest backup (the state before GitPersona) is never pruned; backups record which command made them.
- `uninstall` restores startup files exactly, including ones without a final newline; edits keep CRLF line
  endings and accept quoted keys.
- In a profile's directory, `gh` refuses to act as another account when the profile's account is not signed in;
  `gh auth` commands pass through untouched.
- The shell integration keeps GitPersona first on `PATH` even after macOS `path_helper`; bash no longer creates a
  `~/.bash_profile` that hides `~/.profile`.
- A symlinked `~/.gitconfig` is updated through the link; `GIT_CONFIG_GLOBAL` is honored; a read-only file is refused.
- Upgrading gitpersona with a package manager no longer breaks the credential helper.
- `whoami` reports exactly what Git applies in worktrees and outside repositories.
- Line breaks and other control characters in the configuration are rejected instead of breaking Git.
- `apply` asks for `--yes` before removing files and keeps only the newest 20 backups.
- `apply` keeps the GitPersona block last, so later `git config --global` settings or `gh auth setup-git`
  cannot silently override profiles; `doctor` flags settings after the block.
- `whoami` works in bare repositories and inside `.git`, and warns about `GIT_AUTHOR_EMAIL` /
  `GIT_COMMITTER_EMAIL` and repository-local `user.email` with the exact fix; `doctor` checks the current repository.
- Directory names containing `[`, `*` or `?` match literally.
- `doctor` no longer reports accounts as signed out when `gh` cannot be queried.
- Blank names, emails with angle brackets and a `.pub` file as `ssh_key` are rejected.
- Project scaffold: Go module, `gitpersona version` / `help` commands, Makefile, CI, release configuration,
  privacy check, contribution and development standards.
