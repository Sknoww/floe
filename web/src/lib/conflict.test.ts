import { describe, expect, it } from 'vitest'
import { parseConflict } from './conflict'

/*
 * Both fixtures are `git merge-file -p` output, verbatim — the same text floe
 * rebuilds a conflicted file with, and byte for byte what `apply --3way`
 * leaves in the working tree (see CONTEXT.md).
 */
const merged = `a
<<<<<<< ours.txt
ours line
=======
theirs line
>>>>>>> theirs.txt
c
`

/** Under merge.conflictStyle=diff3 or zdiff3, which adds the base. */
const diff3 = `a
<<<<<<< ours.txt
ours line
||||||| base.txt
base line
=======
theirs line
>>>>>>> theirs.txt
c
`

describe('parseConflict', () => {
  it('sides each line and numbers them as the file stands', () => {
    const { rows, regions } = parseConflict(merged)
    expect(regions).toBe(1)
    expect(rows.map((r) => [r.no, r.side, r.marker])).toEqual([
      [1, 'plain', false],
      [2, 'ours', true],
      [3, 'ours', false],
      [4, 'theirs', true],
      [5, 'theirs', false],
      [6, 'theirs', true],
      [7, 'plain', false],
    ])
  })

  it('labels the markers that open and close the conflict', () => {
    const rows = parseConflict(merged).rows
    expect(rows.filter((r) => r.label).map((r) => [r.no, r.label])).toEqual([
      [2, 'ours'],
      [6, 'theirs'],
    ])
  })

  it('keeps the line as git wrote it, marker and name included', () => {
    const rows = parseConflict(merged).rows
    expect(rows[1]!.text).toBe('<<<<<<< ours.txt')
    expect(rows[5]!.text).toBe('>>>>>>> theirs.txt')
  })

  it('reads the base under diff3', () => {
    const { rows, regions } = parseConflict(diff3)
    expect(regions).toBe(1)
    expect(rows.map((r) => r.side)).toEqual([
      'plain',
      'ours',
      'ours',
      'base',
      'base',
      'theirs',
      'theirs',
      'theirs',
      'plain',
    ])
    // The base opens a section but names no side to label.
    expect(rows[3]).toMatchObject({ no: 4, text: '||||||| base.txt', marker: true })
    expect(rows[3]!.label).toBeUndefined()
  })

  it('counts every region', () => {
    const two = merged + merged
    expect(parseConflict(two).regions).toBe(2)
  })

  it('treats a marker out of its turn as content', () => {
    // A file that writes about conflicts, and never opens one.
    const prose = `Resolve it by deleting the =======\nand the >>>>>>> theirs line.\n`
    const { rows, regions } = parseConflict(prose)
    expect(regions).toBe(0)
    expect(rows.every((r) => !r.marker && r.side === 'plain')).toBe(true)
  })

  it('does not open a second region inside one', () => {
    const nested = `<<<<<<< ours.txt\n<<<<<<< not a marker here\n=======\nt\n>>>>>>> theirs.txt\n`
    const { rows, regions } = parseConflict(nested)
    expect(regions).toBe(1)
    expect(rows[1]).toMatchObject({ no: 2, side: 'ours', marker: false })
  })

  it('needs the exact marker, not a longer run', () => {
    const { rows } = parseConflict('<<<<<<<<\n========\n')
    expect(rows.every((r) => !r.marker)).toBe(true)
  })

  it('ends the last line at the trailing newline', () => {
    expect(parseConflict('one\ntwo\n').rows).toHaveLength(2)
    expect(parseConflict('').rows).toHaveLength(0)
  })
})
