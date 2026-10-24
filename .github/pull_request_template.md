## What and why

<!-- What this changes and why. Link the issue: "Fixes #123". A feature needs a maintainer's go-ahead on its issue. -->

## How it was tested

<!-- Commands you ran and what you checked by hand. -->

## Use of AI

<!-- If an AI tool wrote a substantial part of this change, say which part and what it did. Otherwise write "none". -->

## Checklist

- [ ] It starts from an issue; a new feature had a maintainer's go-ahead
- [ ] One purpose: no unrelated fixes, refactors or features mixed in
- [ ] Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org/), no tool attribution trailers
- [ ] Tests written first; `make check` and `make lint` pass
- [ ] I understand and have verified every line, including any AI-assisted part
- [ ] `CHANGELOG.md` updated under "Unreleased" for user-visible changes
- [ ] README (English and Chinese), help text and console texts (both languages) updated if behavior changed
- [ ] Only demo identities and demo paths are used (`make privacy` passes)
