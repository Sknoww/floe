/*
 * A conflicted file as git writes it, turned into the rows the conflict view
 * draws. The text comes from floe, never from a re-merge here: the preview
 * rebuilds each conflicted file the way `apply --3way` will write it, and the
 * conflict screen reads the file out of the target's working tree. Either way
 * what is drawn is what is (or will be) on disk, markers and all.
 *
 * Lines are numbered as they stand in the result, marker lines included: those
 * are the numbers an editor will show when the user goes to resolve it.
 */

/** Which side of the conflict a line belongs to. */
export type Side = 'plain' | 'ours' | 'base' | 'theirs'

export type ConflictRow = {
  no: number
  text: string
  side: Side
  /** A marker line: amber, and the one row that carries a label. */
  marker: boolean
  /** Whose side the marker opens or closes, for the label beside it. */
  label?: 'ours' | 'theirs'
}

export type ParsedConflict = {
  rows: ConflictRow[]
  /** How many conflicting regions the file has. */
  regions: number
}

/*
 * git's markers are seven characters, alone on the line or followed by a space
 * and a name. The length is `conflict-marker-size`, which an attribute can
 * change; seven is git's default and what floe's own rebuild writes.
 *
 * `|||||||` appears under `merge.conflictStyle=diff3` and `zdiff3` only, and
 * opens the base. A file merged under either style is what the target will get,
 * since floe rebuilds a conflict with the style the apply follows.
 */
const MARKER = /^(<{7}|\|{7}|={7}|>{7})(?: |$)/

export function parseConflict(text: string): ParsedConflict {
  const rows: ConflictRow[] = []
  let regions = 0
  let side: Side = 'plain'
  let no = 0

  for (const line of splitLines(text)) {
    no++
    const marker = MARKER.exec(line)
    if (!marker) {
      rows.push({ no, text: line, side, marker: false })
      continue
    }

    switch (marker[1]![0]) {
      case '<':
        // A `<<<<<<<` inside a conflict is content, not a marker: git only
        // opens a region from outside one.
        if (side !== 'plain') {
          rows.push({ no, text: line, side, marker: false })
          continue
        }
        regions++
        rows.push({ no, text: line, side: 'ours', marker: true, label: 'ours' })
        side = 'ours'
        break
      case '|':
        if (side !== 'ours') {
          rows.push({ no, text: line, side, marker: false })
          continue
        }
        rows.push({ no, text: line, side: 'base', marker: true })
        side = 'base'
        break
      case '=':
        if (side !== 'ours' && side !== 'base') {
          rows.push({ no, text: line, side, marker: false })
          continue
        }
        rows.push({ no, text: line, side: 'theirs', marker: true })
        side = 'theirs'
        break
      default:
        if (side !== 'theirs') {
          rows.push({ no, text: line, side, marker: false })
          continue
        }
        rows.push({ no, text: line, side: 'theirs', marker: true, label: 'theirs' })
        side = 'plain'
        break
    }
  }

  return { rows, regions }
}

function splitLines(text: string): string[] {
  const lines = text.split('\n')
  // A trailing newline terminates the last line rather than starting another.
  if (lines.length > 0 && lines[lines.length - 1] === '') lines.pop()
  return lines
}
