/*
 * The JSON floe's API sends, as the page sees it. Mirrors internal/server:
 * json.go names every one of these types, and api.go the request bodies.
 * Times arrive as RFC 3339 strings.
 */

export type Repo = { name: string; path: string }

/** One pair file. A file floe cannot read has only its path and the error. */
export type Remembered = {
  id?: string
  file: string
  source?: Repo
  target?: Repo
  lastOpened?: string
  /** Repository paths no longer there. */
  missing?: string[]
  error?: string
}

/** GET /api/pairs. `home` abbreviates a path to "~/…"; it may be absent. */
export type Pairs = { home?: string; pairs: Remembered[] }

export type Pair = {
  id: string
  file: string
  source: Repo
  target: Repo
  exclude: string[]
  guard: string[]
  lastOpened?: string
}

export type Commit = {
  id: string
  parents: string[]
  authorName: string
  authorEmail: string
  authorTime: string
  subject: string
}

export type Commits = { branch: string; commits: Commit[] }

/** A status letter as git reports it: M, A, D, R, U… */
export type Status = string

export type File = {
  path: string
  status: Status
  added: number
  deleted: number
  binary: boolean
  excluded: boolean
  skipped: boolean
}

export type CommitDetail = { id: string; message: string; files: File[] }

export type FileDiff = { path: string; diff: string }

export type Change = { path: string; status: Status }

export type Result = {
  /** A conflicted file has status "U". */
  files: Change[]
  conflicts: string[]
  /** Each conflicted text file as the apply writes it; a preview's only. */
  conflicted?: Record<string, string>
}

/** Where the target stands against the source. */
export type Position = {
  /** "" when no commit matches. */
  match: string
  /** Set only when nothing matches. */
  nearest: string
  divergent: string[]
}

/** A transfer of floe's, recorded before it was applied. */
export type Transfer = {
  commit: string
  targetHead: string
  skip: string[]
  applied: string
}

export type Target = {
  /** "" when HEAD is detached. */
  branch: string
  /** "" for a target with no commits. */
  head: string
  dirty: Change[]
  conflicts: string[]
  /** The carried message waiting in SQUASH_MSG, or null. */
  message: string | null
  position: Position
  /** Changes without one are the user's, and are never discarded. */
  transfer: Transfer | null
}

export type Conflict = { path: string; content: string; binary: boolean }

export type Preview = {
  /** What an apply names. */
  id: string
  commit: string
  targetHead: string
  message: string
  files: File[]
  /** Crossing files the guard cannot scan. */
  binary: string[]
  result: Result
}

export type Applied = { transfer: Transfer | null; result: Result }

/** A matched line, cut where the pattern matched. */
export type Part = { text: string; matched?: boolean }

export type Match = {
  pattern: string
  /** "" for the commit message. */
  path: string
  lineNo: number
  line: string
  parts: Part[]
  /** True when the 3-way merge brought the line in, not the patch. */
  merged: boolean
}

export type PatternCheck = {
  /** As it would be saved. */
  pattern: string
  error?: string
  /** The pattern floe thinks was meant. */
  suggestion?: string
}
