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
- **The UI has to look good.** Mockups are approved before frontend code, and
  [`DESIGN.md`](./DESIGN.md) records the rules. The UI is **dark only**, laid out source →
  crossing commit → target.
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
- **Content guard.** A transfer is refused when the patch's added lines, the source commit's
  message, or any line the 3-way merge brings into a file match the pair's patterns — a
  conflict's source side, or a merge driver, can carry lines the patch has only as context.
- **Exclusions are anchored doublestar globs** on whole repo-relative paths. A pattern that
  can never match a file, such as `/README.md` or `docs/`, is an error, not a silent no-op.
- Excluded files stay visible in a commit's file list, struck through. Individual files can be
  unticked for a single transfer.
- On a conflict, floe stops with the markers in place and offers open-in-editor or discard.
- **Open in editor launches VS Code** on the target, at the conflicted file, or the system's
  default text editor where VS Code is not installed. Not `$VISUAL`: a terminal editor
  cannot be opened from a click in the browser.
- **The commit list is the source's first-parent history.** A merge is one entry, diffed
  against its first parent, so a merged branch crosses as one commit.
- **A target with no commits is supported**: its first transfer is the one that gives it a
  history.
- **git 2.32.0 or newer**, checked when a pair is opened.

## Open questions

None.

## Areas

1. ✅ **Plan** — `CONTEXT.md` (stack, conventions, config format).
2. ✅ **Design** — `DESIGN.md` and approved mockups of nine screens: pairs, the main screen,
   pair settings, a target with changes, preview, guard refusal, conflict, staged and the
   discard confirmation. What is left undrawn is listed at the end of `DESIGN.md`.
3. ✅ **Git layer** — `internal/git` shells out to `git`: commit list, per-commit files and
   diff, trees and the history walk, dirty check, preimage import, preview, apply, reset and
   `SQUASH_MSG`. `internal/transfer` orders them — refuse before writing — and computes the
   position. Exclusions arrive as a predicate and guard patterns compiled; matching them and
   the config file are area 6. Tested against scratch repositories with unrelated histories.
4. ⏳ **Server** — loopback binding, token, `Host`/`Origin` checks, JSON API over the git layer.
   The design also needs per-file line counts, which the git layer does not report yet.
5. ⏳ **UI** — the screens from area 2.
6. 🛠️ **Exclusions and content guard** — `internal/pair` reads and writes each pair's config
   and lists the remembered pairs; exclusions are matched and guard patterns compiled there.
   The guard also scans what a 3-way merge brings in. Still to come: editing both from the UI,
   including "never transfer" on a file row, once areas 4 and 5 exist.
7. ⏳ **Release** — GoReleaser, the cask, `README.md`.
8. ⏸️ **Whole-tree comparison** — source HEAD against the target, beyond the per-commit
   nearest match.
