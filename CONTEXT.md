# Floe — Project Context

> The single source of truth for **architectural and product decisions**. Visual and
> interaction decisions live in [`DESIGN.md`](./DESIGN.md). Update this document when a
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
git -C <source> diff --binary --full-index --no-renames <base> <C> -- . ':(exclude,literal)<path>' …
```

- **`<base>` is C's first parent**, or the empty tree for a root commit. For a merge that is
  exactly what it brought in, so a merged branch crosses as one commit (see
  [Which commits are listed](#which-commits-are-listed)).

- **Exclusions are matched once, in Go**, against repo-relative paths. Git receives only
  literal exclude pathspecs for the paths that matched. One matcher means the file list, the
  patch and the position comparison can never disagree about what is excluded. Files unticked
  for a single transfer join the same list.
- **`--no-renames`** splits a rename into a delete and an add, so each side is matched
  against the exclusions on its own path. With renames on, a file moved out of an excluded
  path would carry across as a rename of something the target must never see.
- The empty tree's id is taken from `git hash-object -t tree /dev/null` rather than
  hardcoded, so SHA-256 repositories work.
- **User configuration is pinned out.** A `diff.noprefix`, `diff.external` or
  `color.diff=always` would produce a patch apply cannot read, so the diff also runs with
  `--no-color --no-ext-diff --no-textconv --no-relative --src-prefix=a/ --dst-prefix=b/`.
  Apply runs with `--whitespace=nowarn --no-ignore-whitespace`, so `apply.whitespace=error`
  cannot refuse a commit over a trailing space. Content crosses verbatim.
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
git -C <target> cat-file --batch-check                             # which are missing?
git -C <source> cat-file blob <id>                                 # read whole, in Go …
git -C <target> hash-object -w --no-filters --stdin                # … then written
```

- The ids are the old side of each crossing change in `git diff-tree --raw`. An added file
  has none (the all-zeros id), and a submodule's old side is a commit in another repository,
  not a blob. `--full-index` puts the same whole ids on the patch's `index` lines.
- Each blob is read whole before it is written, never piped: in a shell pipe, a failed
  `cat-file` still let `hash-object` store an empty blob.
- Content from stdin is hashed without the target's clean filters (`autocrlf`, LFS) unless
  `--path` is given; `--no-filters` pins that rather than relying on it. Floe asserts the id
  `hash-object` prints equals the one it asked for.
- The imported objects are unreachable: invisible to `git status` and history, and removed
  by git's garbage collection.

### The preview

What a transfer will do is computed by running it against a **temporary index**, so neither
the target's real index nor its working tree is touched:

```
GIT_INDEX_FILE=<tmp> git -C <target> read-tree HEAD                 # the empty tree, with no commits
git -C <target> apply --numstat -z --whitespace=nowarn < patch      # the paths the patch touches
GIT_INDEX_FILE=<tmp> git -C <target> add --refresh --pathspec-from-file=- --pathspec-file-nul
GIT_INDEX_FILE=<tmp> git -C <target> apply --cached --3way < patch
GIT_INDEX_FILE=<tmp> git -C <target> ls-files -u                   # conflicting files
```

`git apply --check --3way` looks like the obvious tool and is not: it never attempts the
3-way merge, so it cannot tell a file that would conflict from one that would be rejected.

- **The patch's paths are refreshed first.** `read-tree` leaves no stat data, so to git every
  file on disk looks changed. Adding a file the target already has is an add/add merge, which
  reads the target's file from disk — so without the refresh the preview refused as "does not
  match index" a patch the real apply turns into a conflict. Only the patch's paths the index
  already has are refreshed (`add --refresh` fails on any other), as literal pathspecs
  (`GIT_LITERAL_PATHSPECS=1`): a whole-index refresh would hash every file in the target.
  The target is clean, so what is on disk is HEAD.
- **The preview reports what lands in each text file**: the lines the result adds to the
  target's own version, from `git diff --no-index --unified=0` of the two. A clean file's
  result is its blob in the temporary index. A conflict is rebuilt from stages 1–3 with
  `git merge-file -p --diff3`, which matches what apply writes apart from the marker lines, so
  both sides and the base count. Deletions and submodules bring in no lines, and binary files
  (a NUL in the first 8000 bytes — git's own test) are left out: git merges none. Each line
  is numbered as it stands in the result.
- **A conflict is shown as it will land.** Each conflicted text file is rebuilt a second time
  with `git merge-file -p` without `--diff3`, which follows the target's
  `merge.conflictStyle` as the apply does. Probed byte for byte against what
  `apply --3way` writes — under no style set, `merge`, `diff3` and `zdiff3`, and for an
  add/add conflict — and that text is what the preview screen shows.

### Order: refuse before writing

Every check that can refuse a transfer runs before anything visible is written — at most
unreachable objects — so a refusal has nothing to undo.

1. **The target is clean** — `git status --porcelain`, untracked files not counted. They
   cannot mix into a commit, and a patch that would overwrite one is refused by `git apply`
   itself.
2. **The content guard passes on the patch** — the pair's patterns are matched against every
   added line of the filtered patch and against the source commit's full message, before the
   import writes anything. Binary patches are base85-encoded and cannot be scanned line by
   line, so the preview names every binary file: a visible gap rather than a silent one. A
   match carries the line's number (from the hunk headers, or in the message) and the byte
   spans the pattern matched, so the refusal can point at the text.
3. **Preimage import, then preview** — the preview is what the user confirms.
4. **The content guard passes on what lands.** A 3-way merge brings in source lines the patch
   has only as context, or not at all. A conflict spans the target's whole edit, so its
   _theirs_ side — and the base, under `merge.conflictStyle=diff3` — can carry source lines far
   outside the hunk; a merge driver such as `merge=union` merges cleanly and keeps them. So the
   guard is also matched against the preview's landed lines, and such a match is reported as
   brought in by the merge. A line the target already has never matches here: only what the
   result adds is scanned. Known limit: a conflicted file under a custom `merge.<name>.driver`
   is rebuilt with git's stock merge, not the driver.
5. On confirm, the target is **re-checked**: still clean, and HEAD unchanged since the
   preview (a moved HEAD re-runs the preview rather than applying a stale one).
6. **Apply** — `git -C <target> apply --index --3way < patch`. Exit 0 is clean; exit 1 with
   unmerged entries is a conflict; exit 1 without them is a failure, reported with git's
   stderr. A failure writes nothing: `git apply` is all or nothing, so one file it cannot
   apply stops the files that would have merged too. `--3way` merges modifications only — a
   source deletion of a file the target has changed fails rather than conflicts — while an
   addition of a file the target already has is an add/add conflict.
7. **Write the commit message** (below).

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
offers open-in-editor or abort, which the UI calls **Discard transfer**.

**Open in editor** launches VS Code on the target repository, at the conflicted file: VS
Code's commit box picks up the carried `SQUASH_MSG` and it has conflict tooling. Where VS
Code is not installed, the file opens in the system's default text editor. On macOS the
`code` command is often not on `PATH` even with VS Code installed, so floe must find the
application itself rather than rely on it. `$VISUAL` is not used: floe is driven from a
browser, and a terminal editor has no terminal to open in. The exact launch commands are
probed and written here when area 5 builds it.

**Abort** is `git reset --hard HEAD` plus removing `SQUASH_MSG`. It is safe only because the
target was clean before the transfer — and it also discards any resolution edits made since,
so it asks for confirmation and says so. Discarding a clean staged transfer is the same
operation. It is offered only for a transfer floe recorded (see
[A transfer of floe's](#a-transfer-of-floes)): changes floe did not make are never reset.

**A target with no commits is supported** — its first transfer is the one that gives it a
history — but it has no HEAD to reset to. There, abort empties the index
(`git read-tree --empty`) and deletes the files that were in it, which is what `reset --hard`
does to a file staged but not in HEAD. None of them can have been untracked files: `git apply`
refuses to overwrite one.

## Which commits are listed

**The source's first-parent history**, newest first (`git log --first-parent`). A merge is
one entry, diffed against its first parent, so a merged branch crosses as one target commit;
the commits on the merged branch are not listed and cannot be picked on their own. That keeps
the history linear, which the position walk depends on.

## Where the target stands

Computed on demand, never stored in either repository.

- The target side is `git ls-tree -r -z HEAD` → path → (mode, blob id), exclusions removed.
- The source side walks the first-parent history oldest-first through
  `git log --first-parent --diff-merges=first-parent --reverse --raw -z --no-renames`,
  keeping a running path → (mode, id) map and a running count of paths that differ from the
  target. Each commit updates only the paths it changed, so the cost is the total number of
  changes rather than commits × files.
- Zero differences is a **match**; otherwise the smallest count is the **nearest**, and the
  paths still differing there are shown as divergence. **Ties go to the newest commit**: a
  history that returns to an earlier state (a revert) is placed at its latest point, where the
  next transfer starts.
- A target with no commits compares as an empty tree.

## Server

- Binds `127.0.0.1` on an OS-chosen port, never another interface.
- A per-launch random token (`crypto/rand.Text`) is required on every API request, in the
  `X-Floe-Token` header. It reaches the page in the URL **fragment** (`#token=…&pair=<id>`),
  which the browser never sends to a server. Static assets carry no secrets and are served
  without it.
- `Host` must be `127.0.0.1:<port>` (DNS rebinding; `421` otherwise), and `Origin`, when
  present, must match it. No CORS, so another origin's page cannot send the token header at
  all. Every response forbids framing, and API responses are `no-store`.
- `floe <source> <target>` opens and remembers the pair before the server starts, so a pair
  floe cannot transfer between is refused in the terminal rather than in a browser. It then
  prints the address, opens the default browser (`open` on macOS, `xdg-open` on Linux) and
  runs until interrupted, letting requests in flight finish.

### The API

JSON over `internal/transfer` and `internal/pair`. A pair is named by its **ID**: its config
file's name without `.json`. The config is read again on every request, so a hand edit
applies at once; the repositories are opened once a launch.

| Route | |
|---|---|
| `GET /api/pairs` | Remembered pairs. A missing repository or a broken file stays listed, with why. `home` is the user's home directory, which the page needs to abbreviate a path to `~/…` and cannot know on its own |
| `GET /api/pairs/{pair}` | Names, paths, the config file, and both pattern lists |
| `POST /api/pairs/{pair}/open` | Opens the repositories and records `lastOpened` |
| `PUT /api/pairs/{pair}/settings` | Replaces `exclude` and `guard`; nothing is saved unless every pattern is valid |
| `POST /api/patterns/check` | One pattern as it is typed: floe's error and suggested fix (`docs/**`). `literal` escapes a guard pattern. The check is Go's: neither doublestar nor RE2 behaves like a JavaScript pattern |
| `GET /api/pairs/{pair}/commits` | The source's branch and first-parent history |
| `GET /api/pairs/{pair}/commits/{id}` | A commit's full message, and its files with line counts and exclusions |
| `GET /api/pairs/{pair}/commits/{id}/diff?path=` | One file's unified diff, fetched when the file is shown |
| `GET /api/pairs/{pair}/target` | Branch, HEAD, changed files, conflicts, the carried message, the position, and the transfer of floe's waiting, if any |
| `GET /api/pairs/{pair}/conflict?path=` | A conflicted file from the working tree — only a path git reports as unmerged |
| `POST /api/pairs/{pair}/preview` | Runs every check and the preview, and keeps the preview under an id |
| `POST /api/pairs/{pair}/apply` | Applies the preview it names |
| `POST /api/pairs/{pair}/discard` | Discards the transfer of floe's waiting in the target |

- **A refusal is `{"error": {"code", "message"}}`**, the message in floe's or git's own words:
  `dirty` (with `paths`), `guard` (with `matches`), `apply_refused` (git's stderr), `stale`,
  `preview_gone`, `no_transfer`, `empty_commit`, `nothing_to_transfer`, `invalid_settings`.
  The page acts on the code and shows the message.
- A guard match arrives with its line cut into `parts` at the matched spans, so the page marks
  them without counting bytes: JavaScript indexes a string by UTF-16 unit.
- **An apply names the preview it confirms.** The server keeps each pair's last preview in
  memory. An id it does not hold — after a restart, or on a second apply — or a preview whose
  pair settings have changed since is refused as `preview_gone`, and the page previews again.
- **Writes are serialized.** Preview, apply, discard, saving settings and every read of the
  transfer record run one at a time, so two tabs never apply at once and a state read never
  clears the record of a transfer being applied. Once started, an apply or a discard runs to
  the end even if the page goes away.

### A transfer of floe's

Discard is `git reset --hard`, so it is offered only for changes floe knows it made — after a
restart too. Before applying, floe records `{commit, targetHead, skip, applied}` in
`transfers/` under the config directory. The record **counts only while the target is as the
transfer left it**: HEAD unchanged, changes to tracked files, and `SQUASH_MSG` waiting. When
the user commits (HEAD moves, and git removes `SQUASH_MSG`) or discards by hand (a clean tree;
`reset --hard` removes `SQUASH_MSG` too), the record no longer counts and is removed. A failed
apply removes its record. Changes with no record that counts are the user's: the target panel
shows them, and a discard is refused as `no_transfer`.

### Dev loop

Built with the `dev` tag, floe listens on `127.0.0.1:5174`, also answers the Vite dev server's
origin `http://localhost:5173` — which proxies `/api` to it — and opens the page there. A
release build has no such origin and an OS-chosen port.

- `npm run dev` in `web/` serves the page on 5173 with hot reload, `strictPort` so it is that
  port or nothing, and `server.proxy` sending `/api` to `127.0.0.1:5174`. `changeOrigin` is
  left off: the `Host` reaches floe as `localhost:5173`, which a dev build answers beside its
  own address, and the `Origin` is the one it accepts.

---

## Architecture decisions

| Area | Decision |
|---|---|
| Language | Go, one binary. Module `github.com/Sknoww/floe`, binary `floe`. The `go.mod` floor tracks the minimum the code needs, never bumped just because a newer toolchain is installed |
| Layout | `main.go` at the root. `internal/git` shells out and parses; `internal/pair` owns pair config and remembered pairs; `internal/transfer` orders the checks, import, preview, apply and position computation; `internal/server` is HTTP, the token and the JSON API; `internal/gittest` makes the throwaway repositories the tests run against. `web/` is the frontend: `src/lib` is what every screen shares (the typed API client, the session, small helpers, icons), `src/screens` is one file per screen in `DESIGN.md`, and `src/app.css` holds the foundations — nothing else declares a colour or a size |
| Frontend | **Svelte 5 + TypeScript, built with Vite.** Not SvelteKit: the Go server owns routing and the API, and the UI is one embedded page. Node is a build-time dependency only; binary size is not a constraint. There is no router: one pair is open at a time, and which one lives in the URL fragment beside the token, so a reload lands back on it. TypeScript is held at 6.x, which is the newest `svelte-check` declares |
| Styling | Plain CSS with custom properties, scoped per component; the values come from `DESIGN.md`. No component library — the mockups decide the look. The design's one dialog (the discard confirmation) and one menu (the pair switcher) are **hand-rolled, with no primitives library**: a native `<dialog>` opened with `showModal()` already gives the focus trap, Esc and `::backdrop`, and one menu does not earn a dependency |
| Diff rendering | Our own component over git's unified output, so it follows `DESIGN.md`. Binary files render as a named placeholder. Syntax highlighting is deferred; Shiki is the candidate |
| Embedding | `web/embed.go` embeds `all:dist`. `web/dist/` is build output, ignored except a placeholder so `go build` works before the first frontend build, which the Vite build writes back after emptying the directory. IBM Plex Sans and JetBrains Mono are self-hosted through `@fontsource` — a build-time dependency, latin subsets only, in the weights `DESIGN.md` names — so Vite hashes the woff2 into `dist/` and the page fetches nothing at runtime |
| Dev loop | The Vite dev server proxies `/api` to `floe` built with the `dev` tag, which accepts the Vite origin (see [Dev loop](#dev-loop)). A release binary never does. `npm run dev`, `npm run build`, `npm run check` (svelte-check) and `npm test` (Vitest) in `web/` |
| Git access | Shell out via `os/exec`, as drift does; no git library. Every call takes a `context.Context`, reads `-z` output where git offers it, and runs with `LC_ALL=C`. Read-only calls set `GIT_OPTIONAL_LOCKS=0`, so floe's refreshes never contend with the editor's git for the index lock. **git 2.32.0 or newer**, checked when a pair is opened: the release in which `apply --3way` tries the merge first and accepts `--cached`, which the preview needs. Object ids reaching the git layer must be full hex ids, so none can be read as an option |
| Testing | Real throwaway repositories, never mocks, and every pair has unrelated histories. Hermetic as drift's suite is: `TestMain` sets `GIT_CONFIG_NOSYSTEM=1` and `GIT_CONFIG_GLOBAL=/dev/null`, and identity and initial branch are declared per repository. Frontend **logic** is tested with Vitest — the helpers and, when it arrives, diff parsing — not the components: the mockups are what the screens are checked against |
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
- **A transfer of floe's is recorded in `transfers/`**, under its pair's file name (see
  [A transfer of floe's](#a-transfer-of-floes)). Floe writes and removes these itself, and
  reads them as strictly as pair files.

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
  as drift uses). A pattern matches the **whole path from the top level**: `README.md` is only
  the top-level README, `docs` is a file named docs, and `docs/**` is everything under the
  docs directory. A pattern that can never match a path git reports — empty, a leading or
  trailing `/`, an empty segment, a `.` or `..` segment — is an error that suggests the
  pattern meant (`README.md`, `docs/**`).
- `guard` — Go (RE2) regular expressions, case-sensitive unless `(?i)`. The UI can offer
  "match literally" by escaping what the user types. An empty pattern would refuse every
  transfer and is an error.
- Floe writes the file (edits from the UI, `lastOpened`) atomically, via a temp file and
  rename. Hand edits are fine. An unknown field or an invalid pattern is an **error naming
  the file and the field**, never ignored: a setting that silently didn't apply is
  indistinguishable on screen from one that did.

## Open questions

Tracked in [`ROADMAP.md`](./ROADMAP.md#open-questions).
