<script lang="ts">
  /*
   * Screen 3 — a pair's settings: the paths that never cross, and the content
   * guard. Both are patterns, and floe is the only thing that can say whether
   * one is valid: exclusions are doublestar globs and guard patterns are Go's
   * RE2, and neither behaves the way a JavaScript pattern would. So every
   * pattern is checked over the API as it is typed, and what is shown is
   * floe's own message and floe's own suggested fix.
   *
   * Changes are kept only on Save, which replaces both lists at once — the
   * server saves nothing unless every pattern in them is valid.
   *
   * The pair is read again when the screen opens rather than taken from the
   * screen behind it. The config file is floe's, not the page's: a hand edit
   * is as valid as this screen, and saving a copy taken earlier would drop one
   * silently.
   */
  import { Api, ApiError } from '../lib/api'
  import { tilde } from '../lib/paths'
  import Arrow from '../lib/icons/Arrow.svelte'
  import Back from '../lib/icons/Back.svelte'
  import Chevron from '../lib/icons/Chevron.svelte'
  import Close from '../lib/icons/Close.svelte'
  import TopBar from '../lib/TopBar.svelte'
  import type { Pair } from '../lib/types'

  let {
    api,
    pair,
    home,
    onback,
    onsaved,
  }: {
    api: Api
    pair: Pair
    home: string | undefined
    /** Back to the transfer, dropping anything unsaved. */
    onback: () => void
    /** The pair as it was saved: the screens behind this one work from it. */
    onsaved: (saved: Pair) => void
  } = $props()

  type Kind = 'exclude' | 'guard'

  /** The pair as the config file has it, read when the screen opened. */
  let onFile = $state<Pair | undefined>(undefined)
  /** The lists being edited. Nothing here has reached the config file yet. */
  let exclude = $state<string[]>([])
  let guard = $state<string[]>([])
  let saving = $state(false)
  let failure = $state('')

  void load()

  async function load() {
    try {
      const fresh = await api.pair(pair.id)
      onFile = fresh
      exclude = [...fresh.exclude]
      guard = [...fresh.guard]
    } catch (e) {
      failure = message(e)
    }
  }

  /** What is typed under each section, and what floe says about it. */
  let typed = $state<Record<Kind, string>>({ exclude: '', guard: '' })
  let checked = $state<Record<Kind, { pattern: string; error: string; suggestion: string }>>({
    exclude: { pattern: '', error: '', suggestion: '' },
    guard: { pattern: '', error: '', suggestion: '' },
  })
  /** What floe would actually save for what is typed, escaped where asked. */
  let saved = $state<Record<Kind, string>>({ exclude: '', guard: '' })
  /** Escapes what is typed, so a guard pattern matches it as text. */
  let literal = $state(false)

  /*
   * A check in flight for a pattern that has since been typed over must not
   * win: the answer would be about something the input no longer holds.
   */
  let latest = 0

  const list = (kind: Kind) => (kind === 'exclude' ? exclude : guard)
  const dirty = $derived.by(() => {
    const was = onFile
    if (!was) return false
    return (
      exclude.length !== was.exclude.length ||
      guard.length !== was.guard.length ||
      exclude.some((p, i) => p !== was.exclude[i]) ||
      guard.some((p, i) => p !== was.guard[i])
    )
  })

  /** A pattern already in its list would be an entry that does nothing. */
  function duplicate(kind: Kind, pattern: string): boolean {
    return list(kind).includes(pattern)
  }

  /*
   * Addable once floe has answered about exactly what the input now holds,
   * without an error, and the list does not have it already.
   */
  function addable(kind: Kind): boolean {
    return (
      typed[kind].trim() !== '' &&
      checked[kind].pattern === typed[kind] &&
      checked[kind].error === '' &&
      !duplicate(kind, saved[kind])
    )
  }

  async function check(kind: Kind, pattern: string) {
    typed[kind] = pattern
    if (pattern === '') {
      checked[kind] = { pattern: '', error: '', suggestion: '' }
      saved[kind] = ''
      return
    }
    const mine = ++latest
    try {
      const out = await api.checkPattern(kind, pattern, kind === 'guard' && literal)
      if (mine !== latest) return
      // The answer is kept under the text it was about: the input may have
      // moved on since, and Add compares the two.
      checked[kind] = { pattern, error: out.error ?? '', suggestion: out.suggestion ?? '' }
      saved[kind] = out.pattern
    } catch (e) {
      if (mine !== latest) return
      failure = message(e)
    }
  }

  function add(kind: Kind) {
    if (!addable(kind)) return
    const pattern = saved[kind]
    if (kind === 'exclude') exclude = [...exclude, pattern]
    else guard = [...guard, pattern]
    typed[kind] = ''
    checked[kind] = { pattern: '', error: '', suggestion: '' }
    saved[kind] = ''
  }

  function remove(kind: Kind, i: number) {
    if (kind === 'exclude') exclude = exclude.filter((_, n) => n !== i)
    else guard = guard.filter((_, n) => n !== i)
  }

  /** Takes floe's suggestion for what was typed, and checks that instead. */
  function accept(kind: Kind) {
    void check(kind, checked[kind].suggestion)
  }

  async function save() {
    saving = true
    failure = ''
    try {
      onsaved(await api.saveSettings(pair.id, exclude, guard))
    } catch (e) {
      failure = message(e)
    } finally {
      saving = false
    }
  }

  /** Back to the pair's saved patterns, leaving the config file alone. */
  function revert() {
    exclude = [...(onFile?.exclude ?? [])]
    guard = [...(onFile?.guard ?? [])]
    typed = { exclude: '', guard: '' }
    checked = {
      exclude: { pattern: '', error: '', suggestion: '' },
      guard: { pattern: '', error: '', suggestion: '' },
    }
    saved = { exclude: '', guard: '' }
    failure = ''
  }

  function message(e: unknown): string {
    return e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e)
  }
</script>

<div class="screen">
  <TopBar>
    <span class="slash">/</span>
    <button class="switcher" type="button" onclick={onback}>
      <span class="names">
        {pair.source.name}
        <Arrow />
        {pair.target.name}
      </span>
      <Chevron down />
    </button>
  </TopBar>

  <main>
    <button class="back" type="button" onclick={onback}><Back />Back to transfers</button>

    <div class="title">
      <h1>Pair settings</h1>
      <span class="mono file" title={pair.file}>{tilde(pair.file, home)}</span>
    </div>

    {#if failure}
      <p class="failure">{failure}</p>
    {/if}

    {#if !onFile}
      <p class="reading">Reading {pair.file.split('/').pop()}…</p>
    {:else}
      <section>
        <div class="about">
          <span class="eyebrow">Never crosses</span>
          <span class="lead">
            Paths matching these are left out of every transfer. Each pattern matches the whole
            path from the repository's top level, and <span class="mono">**</span> crosses
            directories.
          </span>
        </div>

        {#if exclude.length > 0}
          <div class="patterns">
            {#each exclude as p, i (`${i}:${p}`)}
              <div class="pattern">
                <span class="mono">{p}</span>
                <button type="button" aria-label={`Remove ${p}`} onclick={() => remove('exclude', i)}>
                  <Close />
                </button>
              </div>
            {/each}
          </div>
        {/if}

        <div class="adding">
          <div class="entry">
            <input
              class="mono"
              class:bad={!!checked.exclude.error}
              type="text"
              placeholder="Add a pattern"
              aria-label="A path that never crosses"
              value={typed.exclude}
              oninput={(e) => check('exclude', e.currentTarget.value)}
              onkeydown={(e) => e.key === 'Enter' && add('exclude')}
            />
            <button
              class="addbtn"
              type="button"
              disabled={!addable('exclude')}
              onclick={() => add('exclude')}
            >
              Add
            </button>
          </div>
          {#if checked.exclude.error}
            <!-- floe's own words, and the pattern floe thinks was meant. -->
            <div class="why">
              <span class="bad-text">{checked.exclude.error}</span>
              {#if checked.exclude.suggestion}
                <button class="mono fix" type="button" onclick={() => accept('exclude')}>
                  Use {checked.exclude.suggestion}
                </button>
              {/if}
            </div>
          {:else if duplicate('exclude', saved.exclude)}
            <div class="why"><span class="muted">This pair already never crosses that.</span></div>
          {/if}
        </div>
      </section>

      <section>
        <div class="about">
          <span class="eyebrow">Content guard</span>
          <span class="lead">
            A transfer is refused when an added line, the commit message, or a line the merge
            brings in matches one of these. Go regular expressions, case-sensitive unless they
            start with <span class="mono">(?i)</span>.
          </span>
        </div>

        {#if guard.length > 0}
          <div class="patterns">
            {#each guard as p, i (`${i}:${p}`)}
              <div class="pattern">
                <span class="mono">{p}</span>
                <button type="button" aria-label={`Remove ${p}`} onclick={() => remove('guard', i)}>
                  <Close />
                </button>
              </div>
            {/each}
          </div>
        {/if}

        <div class="adding">
          <div class="entry">
            <input
              class="mono"
              class:bad={!!checked.guard.error}
              type="text"
              placeholder="Add a pattern"
              aria-label="A pattern that refuses a transfer"
              value={typed.guard}
              oninput={(e) => check('guard', e.currentTarget.value)}
              onkeydown={(e) => e.key === 'Enter' && add('guard')}
            />
            <label class="literal">
              <input
                type="checkbox"
                checked={literal}
                onchange={(e) => {
                  literal = e.currentTarget.checked
                  void check('guard', typed.guard)
                }}
              />
              Match literally
            </label>
            <button
              class="addbtn"
              type="button"
              disabled={!addable('guard')}
              onclick={() => add('guard')}
            >
              Add
            </button>
          </div>
          {#if checked.guard.error}
            <div class="why"><span class="bad-text">{checked.guard.error}</span></div>
          {:else if duplicate('guard', saved.guard)}
            <div class="why"><span class="muted">This pair already guards against that.</span></div>
          {:else if literal && saved.guard && saved.guard !== typed.guard}
            <!-- Escaped, so what would be saved is not what was typed. -->
            <div class="why">
              <span class="muted">Saved as <span class="mono">{saved.guard}</span></span>
            </div>
          {/if}
        </div>
      </section>

      <div class="actions">
        <button class="primary" type="button" disabled={!dirty || saving} onclick={save}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button class="ghost" type="button" disabled={!dirty || saving} onclick={revert}>
          Discard changes
        </button>
      </div>
    {/if}
  </main>
</div>

<style>
  .screen {
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .slash {
    color: #3a3f47;
  }

  .switcher {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 8px;
    border: 0;
    border-radius: 4px;
    background: none;
    font: inherit;
    color: var(--text-bright);
    font-weight: 500;
    cursor: pointer;
  }

  .switcher:hover {
    background: var(--raised);
  }

  .names {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  main {
    width: 820px;
    align-self: center;
    padding: 32px 0 48px;
    display: flex;
    flex-direction: column;
    gap: 20px;
    overflow-y: auto;
  }

  .back {
    align-self: flex-start;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    color: var(--text-muted);
    cursor: pointer;
  }

  .back:hover {
    color: var(--text);
  }

  .title {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  h1 {
    margin: 0;
    font-size: var(--size-title);
    font-weight: 600;
    color: var(--text-bright);
  }

  /* Where the patterns actually live: a hand edit is as valid as this screen. */
  .file {
    font-size: var(--size-path);
    color: var(--text-faint);
  }

  .reading {
    margin: 0;
    color: var(--text-faint);
  }

  .failure {
    margin: 0;
    padding: 10px 12px;
    border-radius: var(--radius);
    background: rgba(224, 130, 111, 0.08);
    border: 1px solid rgba(224, 130, 111, 0.3);
    color: var(--refusal);
    line-height: 19px;
  }

  section {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 18px 20px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .about {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .lead {
    color: #9aa1ab;
    line-height: 19px;
  }

  .patterns {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .pattern {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 34px;
    padding: 0 12px;
    border-radius: 4px;
    background: var(--raised);
  }

  .pattern .mono {
    flex: 1 1 auto;
    min-width: 0;
    color: var(--text-bright);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pattern button {
    display: flex;
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
  }

  .adding {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .entry {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  input[type='text'] {
    flex-grow: 1;
    min-width: 0;
    height: 34px;
    padding: 0 12px;
    border: 1px solid var(--control);
    border-radius: 4px;
    background: var(--bar);
    color: var(--text-bright);
  }

  input[type='text']::placeholder {
    font-family: var(--font-sans);
    font-size: var(--size-base);
    color: #5c626b;
  }

  /* Coral is the refusal: the pattern would not be saved as it stands. */
  input.bad {
    border-color: rgba(224, 130, 111, 0.6);
  }

  .literal {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-shrink: 0;
    color: #9aa1ab;
    cursor: pointer;
  }

  .literal input {
    margin: 0;
    accent-color: var(--accent);
  }

  .addbtn {
    flex-shrink: 0;
    height: 34px;
    padding: 0 18px;
    border: 1px solid var(--control);
    border-radius: var(--radius);
    background: none;
    font: inherit;
    font-weight: 600;
    color: var(--text);
    cursor: pointer;
  }

  .addbtn:disabled {
    border-color: transparent;
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }

  .why {
    display: flex;
    align-items: center;
    gap: 12px;
    line-height: 19px;
  }

  .bad-text {
    flex-grow: 1;
    color: var(--refusal);
  }

  .muted {
    color: var(--text-muted);
  }

  /* floe's suggestion, one click away. */
  .fix {
    flex-shrink: 0;
    padding: 0;
    border: 0;
    background: none;
    font-family: var(--font-mono);
    font-size: var(--size-mono);
    color: var(--accent);
    cursor: pointer;
  }

  .fix:hover {
    color: #9adbd0;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .primary,
  .ghost {
    height: 36px;
    padding: 0 18px;
    border-radius: var(--radius);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  .primary {
    border: 0;
    background: var(--accent);
    color: #0f1a18;
  }

  .ghost {
    border: 1px solid var(--control);
    background: none;
    color: var(--text);
  }

  .primary:disabled,
  .ghost:disabled {
    border-color: transparent;
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }
</style>
