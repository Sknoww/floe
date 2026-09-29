# floe

[![Release](https://img.shields.io/github/v/release/Sknoww/floe?color=blue)](https://github.com/Sknoww/floe/releases) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Floe carries commits from one git repository into another that shares **none of its
history** — an internal repo and its public mirror, a fork that was copied rather than
cloned, a monorepo and the project split out of it. It lists the source's commits, shows
each one's files and diff, and applies the one you pick to the target, **staged and never
committed**: the target gets one commit per source commit, written by you.

Paths that must never cross — a README, CI config, an internal doc — are excluded from every
transfer, and a content guard refuses any transfer that would bring in a line you've said
must stay behind.

**Floe never commits and never pushes.**

## Install

```sh
brew install Sknoww/tap/floe
```

With Go 1.24+ and Node 24 installed, from a checkout:

```sh
npm --prefix web ci && npm --prefix web run build
go build -o floe .
```

Floe needs **git 2.32.0 or newer**. It runs on macOS and Linux.

## Usage

```sh
floe <source> <target>   # open a pair of repositories, and remember it
floe                     # list the pairs floe remembers
```

Floe starts a server on `127.0.0.1` with a per-launch token in the address and opens your
browser on it. Press `Ctrl-C` in the terminal to stop it.

The page is laid out in the direction a commit travels: the **source**'s history on the
left, the **crossing** commit with its files and diff in the middle, and the **target** on
the right. Floe works out where the target stands in the source's history, and how far behind
it is, by comparing content rather than by storing anything in either repository.

1. **Pick a commit.** Untick any file you want to leave behind this time. Excluded files
   stay in the list, struck through.
2. **Preview.** The target must have no changes to tracked files first, so two transfers
   can't mix into one commit. Floe runs the guard and shows what each file will do —
   including a conflict, drawn exactly as it will land.
3. **Apply.** The change lands staged in the target, and the source commit's message waits
   in the commit box (`.git/SQUASH_MSG`, which `git commit` and VS Code both pick up).
   Review it and commit it yourself.

If the transfer conflicts, floe stops with the markers in place and offers **Open in
editor** (VS Code, or your default text editor) or **Discard**. Discard only ever removes a
transfer floe applied; it never touches changes of your own.

### No shared history needed

A transfer is the source commit's diff applied with `git apply --3way`. A 3-way merge needs
the file as it was before the commit, and an unrelated repository doesn't have it — so floe
first copies those preimages from the source into the target's object store as unreachable
objects: invisible to status and history, and removed by git's garbage collection. An edit
that conflicts with the target then produces ordinary conflict markers, rather than a patch
that refuses to apply.

## Configuration

Each pair has one file in `~/.config/floe/pairs/` (or `$XDG_CONFIG_HOME/floe/pairs/`),
outside both repositories. The settings screen edits it, and a hand edit is just as valid —
floe re-reads it on every request.

```json
{
  "source": "/Users/you/dev/app-internal",
  "target": "/Users/you/dev/app-public",
  "exclude": ["README.md", ".github/**"],
  "guard": ["(?i)acme corp", "internal\\.example\\.com"],
  "lastOpened": "2026-09-11T17:30:00Z"
}
```

- **`exclude`** — glob patterns on the whole repo-relative path, with `**` crossing
  directories. `README.md` is only the top-level README; `docs/**` is everything under
  `docs`. A pattern that can never match a file is an error, not a silent no-op.
- **`guard`** — regular expressions ([RE2](https://github.com/google/re2/wiki/Syntax)),
  case-sensitive unless prefixed with `(?i)`. A transfer is refused when the lines it adds,
  the commit message, or any line a 3-way merge brings into a file match one.

An unknown field or an invalid pattern is an error naming the file and the field, never
ignored.

## License

[MIT](LICENSE)
