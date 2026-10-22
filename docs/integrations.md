# Shell and prompt integrations

`gitpersona install-shell` sets up your shells; this page is for doing it by hand or with a plugin manager.
The integration puts GitPersona's `gh` entry first on `PATH` (so `gh` follows your directories) and defines
`gitpersona_prompt`, which prints the account Git uses in the current directory.

`gitpersona prompt` never runs `git` and never uses the network, so it is safe to call on every prompt.
Use `--format` with `{account}`, `{profile}` and `{host}`, for example `gitpersona prompt --format '{profile}'`.

## fish

```fish
# fisher
fisher install weikxs/gitpersona
# or by hand, in ~/.config/fish/config.fish
gitpersona init fish | source
```

**tide:** add the item, then reload: `set -U tide_right_prompt_items $tide_right_prompt_items gitpersona`

## zsh

```zsh
# oh-my-zsh: clone into $ZSH_CUSTOM/plugins/gitpersona and add `gitpersona` to plugins=(...)
# zinit:     zinit light weikxs/gitpersona
# antidote:  add `weikxs/gitpersona` to ~/.zsh_plugins.txt
# by hand, in ~/.zshrc:
eval "$(gitpersona init zsh)"
```

**Powerlevel10k:** add `gitpersona` to `POWERLEVEL9K_LEFT_PROMPT_ELEMENTS` or `POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS`.

## bash

```bash
# in ~/.bashrc
eval "$(gitpersona init bash)"
PS1='$(gitpersona_prompt) '"$PS1"
```

## Starship (any shell)

```toml
# ~/.config/starship.toml
[custom.gitpersona]
command = "gitpersona prompt"
when = "command -v gitpersona"
format = "[👤 $output]($style) "
style = "bold green"
```
