# Floe — Design

> The single source of truth for **visual and interaction decisions**. Architectural and
> product decisions live in [`CONTEXT.md`](./CONTEXT.md); what is next lives in
> [`ROADMAP.md`](./ROADMAP.md). It describes what _is_; git is how it got here.

The approved mockups are the design canvas "Floe Mockups" (claude.ai artifact
`ff22ebae-eee3-40a6-9ad6-2415b4960630`). Every screen below is drawn there at 1440×900.

## Direction

- **Dark only.** There is no light theme and no theme setting.
- **Source, crossing, target.** Three columns, left to right in the direction a commit
  travels: the source's history, the commit being carried with its files and diff, and the
  target with its state and every action that writes to it. The direction is the one
  mistake that cannot be undone, so it is the frame of the whole app rather than a label.
- **Dense and technical**, like a git client: flat panels divided by hairlines, no cards,
  no gaps between columns, no shadows except on a dialog.
- **Colour is meaning.** Teal is floe pointing at something or saying it is safe to go on,
  amber is something that needs you, coral is a refusal or an operation that destroys work.
  Diff colours are muted: a screen full of change is the normal case, so `+` and `−` read
  as structure, never as alarm.

## Foundations

### Colour

| Role | Value | Used for |
|---|---|---|
| Ground | `#15171b` | Page and columns |
| Bar | `#121418` | Top bar, inset command chips |
| Raised | `#1a1d22` | Pattern rows, message box, changed-file list |
| Selected | `#1f232a` | Selected commit and file row |
| Border | `#262a31` | Column dividers, panel heads |
| Border, inner | `#22262c` | Dividers inside a panel |
| Control border | `#2f343c` | Ghost buttons, inputs |
| Text | `#d4d7dd` | Body |
| Text, bright | `#eef0f3` | Names, subjects, selected text |
| Text, muted | `#7b828c` | Labels, secondary lines |
| Text, faint | `#6c737e` | Eyebrows, paths, ages |
| Text, dim | `#666c75` | Excluded files, commits older than the target |
| Accent (teal) | `#6fc2b4` | Selection edge, target position, success, primary button |
| Accent line | `#3a6f67` | The "<target> is here" divider |
| Attention (amber) | `#d9b36c` | Conflict, a target with changes, `M` status |
| Refusal (coral) | `#e0826f` | Guard refusal, invalid input, destructive actions |
| Diff add | text `#b9dcbf` on `rgba(111,170,120,.12)`; counts `#8fc59a` | |
| Diff delete | text `#e3b3b3` on `rgba(200,110,110,.12)`; counts `#d59090` | |
| Hunk header | `#7f93b3` on `#1a1d23` | |
| Line numbers | `#525862` | |
| Conflict marker | `#d9b36c` on `rgba(217,179,108,.10)` | The marker lines themselves |
| `ours` | `rgba(127,147,179,.10)`; label `#9fb0c9` | The target's side of a conflict |
| `theirs` | `rgba(111,194,180,.09)`; label `#8fcfc4` | The commit's side |
| Guard hit | text `#f0c2b8` on `rgba(224,130,111,.16)`, 2px `#e0826f` left edge; the matched text `#ffe2dc` on `rgba(224,130,111,.35)`; the line in a match block `#c9a39c` | |

Tinted surfaces (tags, callouts) use their role colour at 8–12% alpha, with a 25–35% alpha
border on a callout.

### Type

- **IBM Plex Sans** for the interface, **JetBrains Mono** for everything git produced:
  commit ids, paths, patterns, diffs, commands. Both are embedded in the binary, since
  floe fetches nothing at runtime. The mockups load them from Google Fonts.
- Base 13px. Diff and mono values 12px, paths under a name 11px.
- **Eyebrow** labels: 11px, uppercase, `0.08em` tracking, faint.
- Names in a panel head 16px/600, the crossing commit's subject 17px/600, page titles
  22px/600.

### Layout

- Top bar 48px: `floe`, then `source → target` with a switcher chevron.
- Columns at 1440 wide: **source 340px · crossing flexible · target 300px**, which leaves
  the diff about 800px.
- Each column opens with a head: eyebrow (`Source`, `Crossing`, `Target`), the name, and
  the path and branch in mono. The crossing head adds the id, author, age and message.
- The target panel's actions sit at its bottom edge, under the sentence that says what
  they will do.
- Rows: commits 40px, files 30px, diff lines 20px.

## Components

- **Commit row**: a state dot, the short id, the subject, the age.
  - A hollow dot is a commit the target does not have yet. A filled teal dot is where the
    target stands. A dim grey dot, with a dimmed row, is older history.
  - A teal divider reads **"app-public is here"** (the target's name) between the target's
    position and the commits after it. On a nearest rather than exact match the same
    divider sits at the nearest commit.
  - The selected row has a 2px teal left edge and the selected background.
  - A subject too long for the column ends in an ellipsis; its full text is in the
    crossing head.
  - While a transfer is in progress, a tag (`staged`, `conflict`) replaces the age on its
    commit.
- **File row**: checkbox, status letter (`M` amber, `A` green, `D` red), path, then line
  counts or a result tag.
  - An excluded file is struck through, dimmed, labelled `excluded`, and its checkbox
    shows a dash. It stays in the list, at the bottom of it: the list reads as what
    crosses, with what never does beneath. Within each group the order is git's. A file
    unticked for one transfer dims where it is and does not move.
  - After apply there are no checkboxes: the row's tag says what happened.
- **Diff**: two gutters, old and new line numbers, 48px each, then the line. Hunk
  headers in their own colour. Our own component, never a library.
- **Conflict view**: the file as git writes it. Marker lines amber on an amber tint; the
  `ours` block tinted blue-grey and labelled with the target (`app-public's line`), the
  `theirs` block tinted teal and labelled with the commit (`from 5b2d8f3`).
- **Guard match**: the pattern in mono, where it matched (file and line, or the commit
  message), the matched line, and `Untick this file`. A line the merge brought in carries
  a `brought in by the merge` tag. In the diff the matched line is tinted coral with a
  coral left edge, and the matched text is marked.
- **Callout**: an icon and a bold title in the role colour, one line under it. Teal for
  staged, amber for a conflict or a predicted conflict, coral for a refusal.
- **Tag**: 11px on a 10–12% tint of its role colour, 3px radius.
- **Buttons**, 38px, 5px radius:
  - Primary: teal fill, dark text. At most one per panel.
  - Ghost: control border, body text.
  - Destructive: coral outline and text; filled coral only inside the confirmation.
  - Disabled: `#22262c` fill, `#5c626b` text.
- **Dialog**: 460px on a darkened page, raised surface, 8px radius, the one shadow in the
  app. Cancel on the left, the action on the right.
- Icons are inline SVG, stroke-based, on a 14–16px grid.

## Screens

1. **Pairs** (bare `floe`). Remembered pairs, most recently opened first: names, both paths,
   when last opened. A pair whose repository is missing stays listed with the missing path
   and why it happened; a pair file that cannot be read stays listed with floe's error. New
   pairs open from a terminal, and the screen shows the command.
2. **Main screen**. The layout above, with the target panel listing its working tree,
   where it stands, how far behind the source it is, and the exclusion and guard counts.
   The sentence above **Preview transfer** says what will happen: how many files are
   staged, that the message is carried, that nothing is committed.
3. **Pair settings**, from the pair name or the target panel's counts. Two sections,
   `Never crosses` and `Content guard`, each with one line on how its patterns match.
   - An invalid pattern is refused as it is added, with floe's own message in coral and
     its suggested fix (`Use docs/**`) one click away.
   - The guard input offers **Match literally**, which escapes what is typed.
   - Changes are kept only on **Save**; **Discard changes** drops them.
4. **Target has changes**. The target panel lists the changed tracked files in amber and
   says to commit or discard them first; untracked files don't count. **Preview transfer**
   is disabled. Floe checks the target again when its window regains focus, and on
   **Check again**.
5. **Preview**. The checks that passed, then what happens to each crossing file (`staged`,
   `conflict`). A predicted conflict gets an amber callout, and the crossing column shows
   that file as it will land. A binary file among those crossing is named under the checks,
   since the guard reads lines and has none to read there. The crossing eyebrow reads
   `Crossing · preview`, each file row's counts give way to what it will do (`merges
   cleanly`, `will conflict`), and no row can be unticked: the preview describes the files
   it was given. The footer names the HEAD it was checked against and says a moved HEAD is
   previewed again. **Back** and **Apply transfer**, the one primary, at twice Back's
   width.
6. **Refused by the guard**. A coral callout that says the target is untouched, then each
   match from the check that refused. Files with a match are tagged `guard` in the file
   list, and the match's path opens that file in the crossing column. Neither way out is
   the one floe recommends, so the footer has no primary: **Edit guard** and
   **Preview again**.
7. **Conflict**. An amber callout, the conflicted files first and then the staged ones,
   and one sentence on what to do next. The crossing column shows the file with its
   markers. **Open in editor** (primary) and **Discard transfer**.
8. **Staged**. A teal callout, the message exactly as it waits in the commit box, and
   "Floe never commits." **Open in editor** and **Discard transfer**.
9. **Discard**. One confirmation for the conflict and the staged screens, since it is one
   operation: where the target goes back to, that every change to tracked files is
   discarded (conflict resolutions included), that untracked files stay, that the carried
   message is removed. **Keep working** and **Discard transfer**.

**Open in editor** launches VS Code on the target, at the conflicted file, or the default
text editor where VS Code is not installed (see `CONTEXT.md`).

## Copy

- **Name the repository**, not its role: "app-public has uncommitted changes", "3 files will
  be staged in app-public". The role is already the column's eyebrow.
- **Say what floe will not do** wherever it could be assumed: "Nothing is committed",
  "Floe never commits", "app-public is untouched".
- **One verb per operation.** Reverting a transfer is **Discard transfer** on every screen
  and in its confirmation.
- **A confirmation states exactly what is lost**, and what is kept.
- Show git's and floe's own words for errors and suggestions rather than paraphrasing them.

## Not yet designed

- git refusing a patch: its message is shown in the target panel. Until then floe shows
  git's own words in the page's banner.
- A HEAD that moved since the preview has no drawing of its own: floe says so in the
  banner, in its own words, and previews again on the spot.
- **Never transfer** on a file row (area 6).
- A long history: windowing the commit list and what the source column shows when the
  target's position is far down it.
- Keyboard navigation.
