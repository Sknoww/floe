/*
 * git's unified diff of one file, turned into the rows the diff component
 * draws. Floe renders diffs itself rather than with a library, so they follow
 * DESIGN.md: two gutters of line numbers, then the line as git wrote it,
 * marker and all.
 *
 * The text git produced is never reflowed or re-indented here. What crosses is
 * verbatim, and what is shown is what crosses.
 */

export type DiffRow =
  | { kind: 'hunk'; text: string }
  | { kind: 'context'; old: number; new: number; text: string }
  | { kind: 'add'; new: number; text: string }
  | { kind: 'delete'; old: number; text: string }
  /** git's own aside, such as "\ No newline at end of file". */
  | { kind: 'note'; text: string }

export type ParsedDiff = {
  rows: DiffRow[]
  /** git will not show the content: it says so instead. */
  binary: boolean
  /** What git reported when there are no hunks — a mode change, an empty file. */
  note: string
}

/** `@@ -12,11 +12,10 @@ optional section heading` */
const HUNK = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/

export function parseDiff(diff: string): ParsedDiff {
  const rows: DiffRow[] = []
  let binary = false
  let note = ''
  let old = 0
  let now = 0
  let inHunk = false

  for (const line of splitLines(diff)) {
    const hunk = HUNK.exec(line)
    if (hunk) {
      rows.push({ kind: 'hunk', text: line })
      old = Number(hunk[1])
      now = Number(hunk[3])
      inHunk = true
      continue
    }

    if (!inHunk) {
      // The header, until the first hunk. Most of it is machinery the screen
      // has already said (the path, the blob ids); what it does carry is the
      // reason there may be no hunks at all.
      if (line.startsWith('Binary files') || line.startsWith('GIT binary patch')) {
        binary = true
        note = line
      } else if (line.startsWith('old mode ') || line.startsWith('new mode ')) {
        note = note === '' ? line : note + ', ' + line
      }
      continue
    }

    switch (line[0]) {
      case ' ':
        rows.push({ kind: 'context', old: old++, new: now++, text: line })
        break
      case '+':
        rows.push({ kind: 'add', new: now++, text: line })
        break
      case '-':
        rows.push({ kind: 'delete', old: old++, text: line })
        break
      case '\\':
        rows.push({ kind: 'note', text: line })
        break
      case undefined:
        // An empty line where git writes a single space for an empty context
        // line: something along the way trimmed it. Count it as context, or
        // every line number below would be wrong.
        rows.push({ kind: 'context', old: old++, new: now++, text: '' })
        break
      default:
        // The next file's header. FileDiff asks git for one path, so this
        // parser is a one-file parser: drawing a second file's lines under the
        // first one's would put true line numbers against the wrong file.
        return { rows, binary, note }
    }
  }

  return { rows, binary, note }
}

function splitLines(diff: string): string[] {
  const lines = diff.split('\n')
  // A trailing newline is a terminator, not an empty last line.
  if (lines.length > 0 && lines[lines.length - 1] === '') lines.pop()
  return lines
}

/** The `+n`/`−n` counts a file row shows, from the rows themselves. */
export function countChanges(rows: DiffRow[]): { added: number; deleted: number } {
  let added = 0
  let deleted = 0
  for (const r of rows) {
    if (r.kind === 'add') added++
    else if (r.kind === 'delete') deleted++
  }
  return { added, deleted }
}
