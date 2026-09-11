# Floe — Project Context

> The single source of truth for **architectural and product decisions**. Visual and
> interaction decisions will live in [`DESIGN.md`](./DESIGN.md). Update this document when a
> decision is made — it describes what _is_, not how it got here (that's git). What is next
> lives in [`ROADMAP.md`](./ROADMAP.md).

## App

Floe carries commits from one git repository into another that shares none of its history.
It lists the source's commits, shows each one's files and diff, and applies the one you pick
to the target **staged, never committed**. You write the commit yourself — in VS Code or on
the command line — so the target gets one commit per source commit, authored by you. Paths
that must never cross are excluded from every transfer, and a content guard refuses a
transfer whose patch or message matches the pair's patterns.

Nothing about any particular pair of repositories is compiled in. Floe is public.

## The transfer (load-bearing — read before touching the git layer)

A transfer moves exactly one source commit `C`. Every step below was probed against scratch
repositories with unrelated histories before it was written here.

### The patch

```
git -C <source> diff --binary --no-renames <C>^ <C> -- . ':(exclude,literal)<path>' …
```

- **Exclusions are matched once, in Go**, against repo-relative paths. Git receives only
  literal exclude pathspecs for the paths that matched. One matcher means the file list, the
  patch and the position comparison can never disagree about what is excluded. Files unticked
  for a single transfer join the same list.
- **`--no-renames`** splits a rename into a delete and an add, so each side is matched
  against the exclusions on its own path. With renames on, a file moved out of an excluded
  path would carry across as a rename of something the target must never see.
- A root commit is diffed against the empty tree, taken from `git hash-object -t tree
  /dev/null` rather than a hardcoded id, so SHA-256 repositories work.
- **Both repositories must use the same object format.** Blob ids are the whole mechanism; a
  SHA-1 source and a SHA-256 target share none. Floe refuses such a pair when it is opened.

### The preimage import

`git apply --3way` merges against the file's preimage blob — the id on the patch's `index`
line. A target file identical to the source's preimage already has that blob (ids are
content hashes) and applies cleanly. A target file that differs does not, and git then falls
back to a plain patch that **rejects a conflicting edit outright** instead of producing
markers.

So before previewing or applying, floe copies every missing preimage blob into the target:

```
git -C <target> cat-file -e <id>                                   # missing?
git -C <source> cat-file blob <id> | git -C <target> hash-object -w --no-filters --stdin
```

- `--no-filters` is required: without it the target's clean filters (`autocrlf`, LFS) would
  rewrite the content and store it under a different id. Floe asserts the id `hash-object`
  prints equals the one it asked for.
- The imported objects are unreachable: invisible to `git status` and history, and removed
  by git's garbage collection.

### The preview

What a transfer will do is computed by running it against a **temporary index**, so neither
the target's real index nor its working tree is touched:

```
GIT_INDEX_FILE=<tmp> git -C <target> read-tree HEAD
GIT_INDEX_FILE=<tmp> git -C <target> apply --cached --3way < patch
GIT_INDEX_FILE=<tmp> git -C <target> ls-files -u                   # conflicting files
```

`git apply --check --3way` looks like the obvious tool and is not: it never attempts the
3-way merge, so it cannot tell a file that would conflict from one that would be rejected.

### Order: refuse before writing

Every check that can refuse a transfer runs before anything is written, so a refusal has
nothing to undo.

1. **The target is clean** — `git status --porcelain`, untracked files not counted. They
   cannot mix into a commit, and a patch that would overwrite one is refused by `git apply`
   itself.
2. **The content guard passes** — the pair's patterns are matched against every added line
   of the filtered patch and against the source commit's full message. Binary patches are
   base85-encoded and cannot be scanned line by line, so the preview names every binary file:
   a visible gap rather than a silent one.
3. **Preimage import, then preview** — the preview is what the user confirms.
4. On confirm, the target is **re-checked**: still clean, and HEAD unchanged since the
   preview (a moved HEAD re-runs the preview rather than applying a stale one).
5. **Apply** — `git -C <target> apply --index --3way < patch`. Exit 0 is clean; exit 1 with
   unmerged entries is a conflict; exit 1 without them is a failure, reported with git's
   stderr.
6. **Write the commit message** (below).

### The commit message

The source commit's message (`git log -1 --format=%B <C>`) is written to
`git -C <target> rev-parse --git-path SQUASH_MSG`. `git commit` uses that file as the message
with no merge in progress and deletes it on commit; VS Code's git extension fills its commit
box from it (`getInputTemplate` in `extensions/git/src/repository.ts`, which prefers
`MERGE_MSG` when both exist).

- **A `SQUASH_MSG` beside a clean tree is stale**, whoever wrote it: git deletes the file on
  commit, and a clean tree has nothing for it to describe. Floe removes one whenever it finds
  it, so a transfer discarded by hand cannot pre-fill an unrelated commit.
- VS Code reads `<root>/.git/SQUASH_MSG` literally, so in a linked worktree (where `.git` is
  a file) only `git commit` picks the message up.

### Conflict and abort

A conflict stops with markers in the working tree and unmerged entries in the index. Floe
offers open-in-editor (what it launches is an open question) or abort.

**Abort** is `git reset --hard HEAD` plus removing `SQUASH_MSG`. It is safe only because the
target was clean before the transfer — and it also discards any resolution edits made since,
so it asks for confirmation and says so. Discarding a clean staged transfer is the same
operation.

## Where the target stands

Computed on demand, never stored in either repository.

- The target side is `git ls-tree -r -z HEAD` → path → (mode, blob id), exclusions removed.
- The source side walks commits oldest-first through `git log --raw -z --no-renames`,
  keeping a running path → (mode, id) map and a running count of paths that differ from the
  target. Each commit updates only the paths it changed, so the cost is the total number of
  changes rather than commits × files.
- Zero differences is a **match**; otherwise the smallest count is the **nearest**, and the
  paths still differing there are shown as divergence.
- The walk assumes a linear order, which ties it to the open question of which commits are
  listed.

## Server

- Binds `127.0.0.1` on an OS-chosen port, never another interface.
- A per-launch random token is required on every API request. It reaches the page in the URL
  **fragment**, which the browser never sends to a server, and the page sends it back in a
  request header. Static assets carry no secrets and are served without it.
- `Host` must be `127.0.0.1:<port>` (DNS rebinding), and `Origin`, when present, must match
  it. No CORS.
- Opens the default browser (`open` on macOS, `xdg-open` on Linux) and runs until
  interrupted.

---

## Architecture decisions

| Area | Decision |
|---|---|
| Language | Go, one binary. Module `github.com/Sknoww/floe`, binary `floe`. The `go.mod` floor tracks the minimum the code needs, never bumped just because a newer toolchain is installed |
| Layout | `main.go` at the root. `internal/git` shells out and parses; `internal/pair` owns pair config and remembered pairs; `internal/transfer` orders the checks, import, preview, apply and position computation; `internal/server` is HTTP, the token and the JSON API. `web/` is the frontend |
| Frontend | **Svelte 5 + TypeScript, built with Vite.** Not SvelteKit: the Go server owns routing and the API, and the UI is one embedded page. Node is a build-time dependency only; binary size is not a constraint |
| Styling | Plain CSS with custom properties, scoped per component; the values come from `DESIGN.md`. No component library — the mockups decide the look. Unstyled primitives are considered in area 2 if menus or dialogs need them |
| Diff rendering | Our own component over git's unified output, so it follows `DESIGN.md`. Binary files render as a named placeholder. Syntax highlighting is deferred; Shiki is the candidate |
| Embedding | `web/embed.go` embeds `all:dist`. `web/dist/` is build output, ignored except a placeholder so `go build` works before the first frontend build |
| Dev loop | The Vite dev server proxies `/api` to `floe` running in a dev mode that accepts the Vite origin. A release binary never does |
| Git access | Shell out via `os/exec`, as drift does; no git library. Every call takes a `context.Context`, reads `-z` output where git offers it, and runs with `LC_ALL=C`. Read-only calls set `GIT_OPTIONAL_LOCKS=0`, so floe's refreshes never contend with the editor's git for the index lock |
| Testing | Real throwaway repositories, never mocks, and every pair has unrelated histories. Hermetic as drift's suite is: `TestMain` sets `GIT_CONFIG_NOSYSTEM=1`, and identity and initial branch are declared per repository. Frontend logic (diff parsing) is tested with Vitest |
| CI | Go tests plus the frontend build and tests on every push and pull request. Push, wait for green on the exact commit, then tag |
| Distribution | GoReleaser on a tag: the frontend is built first (`npm ci && npm run build` in `web/`), then darwin/linux × amd64/arm64, a GitHub release, and a cask pushed to `Sknoww/homebrew-tap` with a `postflight` that strips quarantine. `main.version` is stamped via ldflags. The tap token is checked before anything is published |
| Build target | macOS primary; Linux supported |

## Configuration

- **Root:** `$XDG_CONFIG_HOME/floe/`, else `~/.config/floe/` — drift's convention, rather
  than `os.UserConfigDir()` (`~/Library/Application Support` on macOS).
- **One file per pair**, in `pairs/`, named
  `<source-dir-name>--<target-dir-name>-<first 8 hex of sha256(source + "\0" + target)>.json`:
  readable in a listing, and unique for same-named repositories in different places.
- **A pair is identified by both repositories' absolute, symlink-resolved top-level paths.**
  Moving a repository orphans its pair; bare `floe` shows such a pair as missing rather than
  hiding it.
- **Remembered pairs are the files in `pairs/`**, ordered by `lastOpened`.

```json
{
  "source": "/Users/you/dev/app-internal",
  "target": "/Users/you/dev/app-public",
  "exclude": ["README.md", ".github/**"],
  "guard": ["(?i)acme corp", "internal\\.example\\.com"],
  "lastOpened": "2026-09-11T17:30:00Z"
}
```

- `exclude` — glob patterns on repo-relative paths, `**` crossing directories (doublestar,
  as drift uses).
- `guard` — Go (RE2) regular expressions, case-sensitive unless `(?i)`. The UI can offer
  "match literally" by escaping what the user types.
- Floe writes the file (edits from the UI, `lastOpened`) atomically, via a temp file and
  rename. Hand edits are fine. An unknown field or an invalid pattern is an **error naming
  the file and the field**, never ignored: a setting that silently didn't apply is
  indistinguishable on screen from one that did.

## Open questions

Tracked in [`ROADMAP.md`](./ROADMAP.md#open-questions).
