# Floe — Roadmap

> **Read this first, every session.** What floe is, what is settled, and what is next. It
> tracks _areas of work_, not commits — git is the build history. The detail behind each
> settled point lives in [`CONTEXT.md`](./CONTEXT.md).

Status: ✅ shipped · 🛠️ in progress · ⏳ next · ⏸️ deferred

## What floe is

A local tool for carrying commits from one git repository into another that shares none of
its history. It lists the source's commits, shows each one's files and diffs, and applies the
one you pick to the target — **staged, never committed** — so the target gets one commit per
source commit, written by you. Paths that must never cross (a README, CI config) are excluded
from every transfer.

## Settled

- **Go, one binary, UI in the browser.** `floe` starts a server on `127.0.0.1` only, with a
  per-launch token in the URL, and opens the default browser. Frontend assets are embedded;
  nothing is fetched at runtime.
- **The UI is Svelte 5 + TypeScript, built with Vite** and embedded in the binary. Node is a
  build-time dependency only; binary size is not a constraint. Diffs are rendered by our own
  component, not a library, so they follow `DESIGN.md`.
- **The UI has to look good.** Mockups are approved before frontend code, and `DESIGN.md`
  records the rules once they're made.
- **Public**, shipped as a Homebrew cask in `Sknoww/homebrew-tap` through GoReleaser, the same
  path as drift. Nothing about any particular pair of repositories is compiled in.
- **Launch:** `floe <source> <target>` opens a pair and remembers it; bare `floe` lists the
  remembered pairs.
- **Floe never commits and never pushes.** A transfer is the source's
  `git diff --binary --no-renames C^ C` (minus exclusions) piped into
  `git apply --index --3way` in the target. `--3way` implies `--index`, which is why a
  transfer lands staged rather than only on disk. You commit it yourself.
- **No shared remote or history is needed — but a 3-way merge needs the preimage.** Blob ids
  are content hashes, so a target file identical to the source's preimage applies cleanly.
  A target file that differs does not have the preimage blob, and without it `--3way` falls
  back to a plain patch that rejects a conflicting edit outright. So floe first copies each
  changed file's preimage blob from the source into the target's object store: unreachable
  objects, invisible to status and history, removed by git's garbage collection. A
  conflicting edit then produces ordinary conflict markers.
- **The target must be clean** before a transfer, so two transfers can't mix into one commit.
- **The source commit's message is carried, not committed.** Floe writes it to the target's
  `.git/SQUASH_MSG`, which `git commit` and VS Code's commit box both pick up. Floe removes
  it on abort, and treats one found beside a clean tree as stale and removes it.
- **Where the target stands is computed, not stored.** Compare the target HEAD's blob ids with
  each source commit's, exclusions removed: zero differences is a match, the smallest non-zero
  count is the nearest, and the remainder is shown as divergence. No position state is
  written into either repository.
- **Configuration lives outside both repositories**, per source/target pair, in the user's
  config directory: exclusions and content-guard patterns.
- **Content guard.** A transfer is refused when the patch's added lines or the source commit's
  message match the pair's patterns.
- Excluded files stay visible in a commit's file list, struck through. Individual files can be
  unticked for a single transfer.
- On a conflict, floe stops with the markers in place and offers open-in-editor or abort.
- **The commit list is the source's first-parent history.** A merge is one entry, diffed
  against its first parent, so a merged branch crosses as one commit.
- **A target with no commits is supported**: its first transfer is the one that gives it a
  history.
- **git 2.32.0 or newer**, checked when a pair is opened.

## Open questions

- **The content guard and conflict markers** (area 6) — the guard scans added lines and the
  message, but a conflict's _theirs_ side can carry source lines the patch has only as
  context, and `merge.conflictStyle=diff3` adds the base lines too. Scan context lines, scan
  the conflicted result, or accept the gap and name it in the preview.
- **Open-in-editor** (area 2) — what it launches: `code`, `$VISUAL`, or a per-user setting.

## Areas

1. ✅ **Plan** — `CONTEXT.md` (stack, conventions, config format). The questions still open
   belong to areas 2 and 6.
2. ⏳ **Design** — mockups of setup, the commit list, a commit's files and diff, the transfer
   preview and the conflict state.
3. ✅ **Git layer** — `internal/git` shells out to `git`: commit list, per-commit files and
   diff, trees and the history walk, dirty check, preimage import, preview, apply, reset and
   `SQUASH_MSG`. `internal/transfer` orders them — refuse before writing — and computes the
   position. Exclusions arrive as a predicate and guard patterns compiled; matching them and
   the config file are area 6. Tested against scratch repositories with unrelated histories.
4. ⏳ **Server** — loopback binding, token, `Host`/`Origin` checks, JSON API over the git layer.
5. ⏳ **UI** — the screens from area 2.
6. ⏳ **Exclusions and content guard** — editable from the UI, including "never transfer" on a
   file row.
7. ⏳ **Release** — GoReleaser, the cask, `README.md`.
8. ⏸️ **Whole-tree comparison** — source HEAD against the target, beyond the per-commit
   nearest match.
