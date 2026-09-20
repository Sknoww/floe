/*
 * floe's API, over fetch. Every request carries the launch's token; a refusal
 * arrives as {"error": {"code", "message"}} and is thrown as an ApiError, whose
 * code the page acts on and whose message it shows in floe's or git's own
 * words.
 */
import type {
  Applied,
  Commits,
  CommitDetail,
  Conflict,
  FileDiff,
  Match,
  Pair,
  Pairs,
  PatternCheck,
  Preview,
  Target,
} from './types'

/** A refusal or a failure, as floe describes it. */
export class ApiError extends Error {
  readonly code: string
  readonly status: number
  /** "dirty": the target's changed files. */
  readonly paths: string[]
  /** "guard": every match. */
  readonly matches: Match[]

  constructor(
    status: number,
    code: string,
    message: string,
    paths: string[] = [],
    matches: Match[] = [],
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.paths = paths
    this.matches = matches
  }
}

export class Api {
  readonly #token: string

  constructor(token: string) {
    this.#token = token
  }

  async #call<T>(method: string, path: string, body?: unknown): Promise<T> {
    let response: Response
    try {
      response = await fetch(path, {
        method,
        headers: {
          'X-Floe-Token': this.#token,
          ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
    } catch (cause) {
      // floe has stopped, or the connection was cut mid-request.
      throw new ApiError(0, 'unreachable', 'floe is not answering: it may have stopped.')
    }

    const text = await response.text()
    let parsed: unknown = null
    if (text !== '') {
      try {
        parsed = JSON.parse(text)
      } catch {
        // A body that is not JSON is the Go server's own plain-text refusal,
        // from a check that runs before the API (Host, method).
        throw new ApiError(response.status, 'failed', text.trim())
      }
    }

    if (!response.ok) {
      const e = (parsed as { error?: Partial<ApiError> } | null)?.error
      throw new ApiError(
        response.status,
        e?.code ?? 'failed',
        e?.message ?? `${method} ${path} failed with ${response.status}`,
        e?.paths ?? [],
        e?.matches ?? [],
      )
    }
    return parsed as T
  }

  pairs(): Promise<Pairs> {
    return this.#call('GET', '/api/pairs')
  }

  pair(id: string): Promise<Pair> {
    return this.#call('GET', `/api/pairs/${encodeURIComponent(id)}`)
  }

  /** Opens a pair's repositories and records when it was opened. */
  open(id: string): Promise<Pair> {
    return this.#call('POST', `/api/pairs/${encodeURIComponent(id)}/open`)
  }

  /** Nothing is saved unless every pattern is valid. */
  saveSettings(id: string, exclude: string[], guard: string[]): Promise<Pair> {
    return this.#call('PUT', `/api/pairs/${encodeURIComponent(id)}/settings`, { exclude, guard })
  }

  /** One pattern as it is typed. `literal` escapes a guard pattern. */
  checkPattern(kind: 'exclude' | 'guard', pattern: string, literal = false): Promise<PatternCheck> {
    return this.#call('POST', '/api/patterns/check', { kind, pattern, literal })
  }

  commits(id: string): Promise<Commits> {
    return this.#call('GET', `/api/pairs/${encodeURIComponent(id)}/commits`)
  }

  commit(id: string, commit: string): Promise<CommitDetail> {
    return this.#call('GET', `/api/pairs/${encodeURIComponent(id)}/commits/${commit}`)
  }

  diff(id: string, commit: string, path: string): Promise<FileDiff> {
    const q = new URLSearchParams({ path })
    return this.#call('GET', `/api/pairs/${encodeURIComponent(id)}/commits/${commit}/diff?${q}`)
  }

  target(id: string): Promise<Target> {
    return this.#call('GET', `/api/pairs/${encodeURIComponent(id)}/target`)
  }

  conflict(id: string, path: string): Promise<Conflict> {
    const q = new URLSearchParams({ path })
    return this.#call('GET', `/api/pairs/${encodeURIComponent(id)}/conflict?${q}`)
  }

  /** Runs every check that can refuse the transfer, and computes what it does. */
  preview(id: string, commit: string, skip: string[] = []): Promise<Preview> {
    return this.#call('POST', `/api/pairs/${encodeURIComponent(id)}/preview`, { commit, skip })
  }

  /** Carries out the preview it names. */
  apply(id: string, preview: string): Promise<Applied> {
    return this.#call('POST', `/api/pairs/${encodeURIComponent(id)}/apply`, { preview })
  }

  discard(id: string): Promise<void> {
    return this.#call('POST', `/api/pairs/${encodeURIComponent(id)}/discard`)
  }

  /**
   * Opens the target in VS Code — or the default text editor where it is not
   * installed — at `path`, which must be one git reports as changed there.
   * `line` numbers from 1; 0 is the file's top.
   */
  editor(id: string, path = '', line = 0): Promise<void> {
    return this.#call('POST', `/api/pairs/${encodeURIComponent(id)}/editor`, { path, line })
  }
}
