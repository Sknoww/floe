import { describe, expect, it } from 'vitest'
import { countChanges, parseDiff } from './diff'

/** Exactly what internal/git's FileDiff returns for a modified file. */
const modified = `diff --git a/internal/server/api.go b/internal/server/api.go
index 83e2072..3ca8681 100644
--- a/internal/server/api.go
+++ b/internal/server/api.go
@@ -88,9 +88,13 @@ func (s *Server) listPairs(w http.ResponseWriter, r *http.Request) {
 		}
 		out = append(out, j)
 	}
+	home, _ := os.UserHomeDir()
 	writeJSON(w, http.StatusOK, struct {
+		Home  string           \`json:"home,omitempty"\`
 		Pairs []rememberedJSON \`json:"pairs"\`
-	}{out})
+	}{home, out})
 }
`

const added = `diff --git a/web/src/main.ts b/web/src/main.ts
new file mode 100644
index 0000000..ad4f7f8
--- /dev/null
+++ b/web/src/main.ts
@@ -0,0 +1,2 @@
+import { mount } from 'svelte'
+import App from './App.svelte'
`

describe('parseDiff', () => {
  it('drops the header and keeps the hunk', () => {
    const { rows } = parseDiff(modified)
    expect(rows[0]).toEqual({
      kind: 'hunk',
      text: '@@ -88,9 +88,13 @@ func (s *Server) listPairs(w http.ResponseWriter, r *http.Request) {',
    })
    expect(rows.some((r) => r.text.startsWith('diff --git'))).toBe(false)
    expect(rows.some((r) => r.text.startsWith('index '))).toBe(false)
    expect(rows.some((r) => r.text.startsWith('--- a/'))).toBe(false)
  })

  it('numbers each line as it stands on its own side', () => {
    const { rows } = parseDiff(modified)
    // The hunk starts at 88 on both sides.
    expect(rows[1]).toEqual({ kind: 'context', old: 88, new: 88, text: ' \t\t}' })
    expect(rows[2]).toEqual({ kind: 'context', old: 89, new: 89, text: ' \t\tout = append(out, j)' })
    expect(rows[3]).toEqual({ kind: 'context', old: 90, new: 90, text: ' \t}' })
    // An added line advances only the new side.
    expect(rows[4]).toEqual({ kind: 'add', new: 91, text: '+\thome, _ := os.UserHomeDir()' })
    expect(rows[5]).toEqual({
      kind: 'context',
      old: 91,
      new: 92,
      text: ' \twriteJSON(w, http.StatusOK, struct {',
    })
  })

  it('advances only the old side on a deletion', () => {
    const { rows } = parseDiff(modified)
    const deleted = rows.find((r) => r.kind === 'delete')
    expect(deleted).toEqual({ kind: 'delete', old: 93, text: '-\t}{out})' })
  })

  it('keeps the line as git wrote it, marker and all', () => {
    const { rows } = parseDiff(added)
    expect(rows[1]).toEqual({ kind: 'add', new: 1, text: "+import { mount } from 'svelte'" })
  })

  it('starts an added file at line 1 on the new side only', () => {
    const { rows } = parseDiff(added)
    expect(rows.filter((r) => r.kind === 'add').map((r) => r.new)).toEqual([1, 2])
  })

  it('reads a hunk header with no counts', () => {
    const { rows } = parseDiff('@@ -7 +7 @@\n-was\n+is\n')
    expect(rows[1]).toEqual({ kind: 'delete', old: 7, text: '-was' })
    expect(rows[2]).toEqual({ kind: 'add', new: 7, text: '+is' })
  })

  it('carries several hunks, each renumbered from its own header', () => {
    const two = '@@ -1,1 +1,1 @@\n context\n@@ -40,1 +41,1 @@\n far below\n'
    const { rows } = parseDiff(two)
    expect(rows.map((r) => r.kind)).toEqual(['hunk', 'context', 'hunk', 'context'])
    expect(rows[3]).toEqual({ kind: 'context', old: 40, new: 41, text: ' far below' })
  })

  it("keeps git's no-newline aside without numbering it", () => {
    const { rows } = parseDiff('@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n')
    expect(rows[2]).toEqual({ kind: 'note', text: '\\ No newline at end of file' })
    // The aside is not a line of the file, so the add after it is still line 1.
    expect(rows[3]).toEqual({ kind: 'add', new: 1, text: '+b' })
  })

  it('counts a stripped empty context line, so the numbering stays true', () => {
    const { rows } = parseDiff('@@ -1,3 +1,3 @@\n a\n\n b\n')
    expect(rows[2]).toEqual({ kind: 'context', old: 2, new: 2, text: '' })
    expect(rows[3]).toEqual({ kind: 'context', old: 3, new: 3, text: ' b' })
  })

  it('reports a binary file rather than drawing it', () => {
    const d = 'diff --git a/logo.png b/logo.png\nBinary files a/logo.png and b/logo.png differ\n'
    const { rows, binary, note } = parseDiff(d)
    expect(binary).toBe(true)
    expect(note).toBe('Binary files a/logo.png and b/logo.png differ')
    expect(rows).toEqual([])
  })

  it('reports a mode change, which has no hunks at all', () => {
    const d = 'diff --git a/run.sh b/run.sh\nold mode 100644\nnew mode 100755\n'
    const { rows, binary, note } = parseDiff(d)
    expect(binary).toBe(false)
    expect(rows).toEqual([])
    expect(note).toBe('old mode 100644, new mode 100755')
  })

  it('is empty for an empty diff', () => {
    expect(parseDiff('')).toEqual({ rows: [], binary: false, note: '' })
  })

  it('stops at the next file, having been asked for one', () => {
    // A second file's lines drawn under the first one's would put true line
    // numbers against the wrong file.
    const d = '@@ -1 +1 @@\n+one\ndiff --git a/b b/b\n@@ -1 +1 @@\n+two\n'
    const { rows } = parseDiff(d)
    expect(rows.filter((r) => r.kind === 'add').map((r) => r.text)).toEqual(['+one'])
  })
})

describe('countChanges', () => {
  it('counts what the file row shows', () => {
    expect(countChanges(parseDiff(modified).rows)).toEqual({ added: 3, deleted: 1 })
  })

  it('counts nothing in a binary diff', () => {
    expect(countChanges([])).toEqual({ added: 0, deleted: 0 })
  })
})
