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
  config directory: exclusions and content-guard patterns. Floe re-reads the file on every
  request, so a hand edit is as valid as the settings screen — and the page re-reads it too,
  rather than saving a copy taken when the pair was opened.
- **Content guard.** A transfer is refused when the patch's added lines, the source commit's
  message, or any line the 3-way merge brings into a file match the pair's patterns — a
  conflict's source side, or a merge driver, can carry lines the patch has only as context.
- **Exclusions are anchored doublestar globs** on whole repo-relative paths. A pattern that
  can never match a file, such as `/README.md` or `docs/`, is an error, not a silent no-op.
- Excluded files stay visible in a commit's file list, struck through. Individual files can be
  unticked for a single transfer.
- On a conflict, floe stops with the markers in place and offers open-in-editor or discard.
- **A transfer waiting in the target is not one of the page's states.** It is in the
  repository, so the conflict and staged screens are what the target says rather than
  something the page chose, and a reload or a restart lands back on them.
- **Discard is offered only for a transfer floe recorded.** Floe records the transfer it
  applies outside both repositories, and it counts only while that transfer still waits in
  the target — after a restart too. Changes floe did not make are never discarded.
- **Open in editor launches VS Code** on the target, at the conflicted file, or the system's
  default text editor where VS Code is not installed. Not `$VISUAL`: a terminal editor
  cannot be opened from a click in the browser. `internal/editor` finds VS Code's command
  line tool on `PATH` or inside its macOS application bundle, and opens the repository as a
  folder so the carried message is in the commit box.
- **The commit list is the source's first-parent history.** A merge is one entry, diffed
  against its first parent, so a merged branch crosses as one commit.
- **A target with no commits is supported**: its first transfer is the one that gives it a
  history.
- **git 2.32.0 or newer**, checked when a pair is opened.
- **The page is one screen at a time, with no router.** Which pair is open lives in the URL
  fragment beside the token, so reloading lands back where you were. Fonts are self-hosted
  and embedded; the one dialog and one menu are hand-rolled rather than a primitives library.

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
4. ✅ **Server** — `main.go` and `internal/server`: loopback binding, the token, `Host` and
   `Origin` checks, and the JSON API in `CONTEXT.md` over the transfer and pair layers. The
   git layer gained what the screens need: line counts, one file's diff, the branch, status
   letters, the carried message, and a conflict as it will land. A transfer floe applies is
   recorded, so Discard survives a restart and is never offered over the user's own work.
   `web/embed.go` embeds a placeholder until area 5 builds the frontend.
5. ✅ **UI** — the screens from area 2, against the API. `web/` is a Vite project building
   Svelte 5 + TypeScript into the `dist/` the binary embeds, with the foundations from
   `DESIGN.md` as custom properties in `src/app.css`, the fonts self-hosted, a typed client
   over the whole API, and the token and open pair read from the URL fragment. The dev loop
   is live: `npm run dev` proxies `/api` to a `dev`-tagged floe, with hot reload. Built so
   far: **screen 1, Pairs**, including a missing repository and an unreadable pair file;
   **screen 2, the main screen**, with the commit list and where the target stands, the
   crossing commit's files with unticking, and floe's own diff component over git's unified
   output; **screen 4**, the same screen with a target that has changes; **screen 5, the
   preview**, with the checks that passed, what each crossing file will do, and a predicted
   conflict shown as it will land; and **screen 6, refused by the guard**, with every match
   over the line that matched, marked where the pattern hit, and `Untick this file` as the
   way out that leaves the pair's settings alone. Screens 2, 4, 5 and 6 are one shell —
   `screens/Main.svelte` — and a target column per phase. **Apply transfer** is wired: a
   HEAD that moved is previewed again rather than applied stale. Then **screen 7, the
   conflict**, and **screen 8, staged** — which the target decides, not the page, so they
   survive a reload and a restart — with the file rows losing their checkboxes for the tag
   that says what each one did, the commit list pinned to the transfer, the conflicted file
   read back out of the target's working tree, and **Open in editor** over
   `internal/editor`; and **screen 9, the discard confirmation**, the one dialog, on a
   native `<dialog>`. Last, **screen 3, pair settings**, reached from the target panel's
   counts and from screen 6's **Edit guard**, with every pattern checked over the API as it
   is typed. All nine screens of area 2 are built.
6. 🛠️ **Exclusions and content guard** — `internal/pair` reads and writes each pair's config
   and lists the remembered pairs; exclusions are matched and guard patterns compiled there.
   The guard also scans what a 3-way merge brings in. The API saves both lists and checks a
   pattern as it is typed, with floe's suggested fix, and screen 3 is that screen. What is
   left is **"never transfer" on a file row** — a shortcut for what screen 3 already does
   the long way, and undrawn: it needs a design decision before it is built.
7. ✅ **Release** — `.goreleaser.yaml` builds the frontend in its before hooks, then
   darwin/linux × amd64/arm64 with `main.version` stamped, and a cask for
   `Sknoww/homebrew-tap` that strips quarantine. `ci.yml` runs the frontend tests and build
   and the Go suite on every push; `release.yml` does the same on a `v*` tag, after checking
   the tap token, before GoReleaser publishes. `README.md` and an MIT `LICENSE`. **v0.1.0**
   is out: `brew install Sknoww/tap/floe`.
8. ⏸️ **Whole-tree comparison** — source HEAD against the target, beyond the per-commit
   nearest match.
