<script lang="ts">
  /*
   * Screen 2 — the main screen, and screen 4 when the target has changes.
   * Source, crossing, target: three columns, left to right in the direction a
   * commit travels. Every action that writes to the target sits in the target
   * column, under the sentence that says what it will do.
   */
  import { Api, ApiError } from '../lib/api'
  import { since } from '../lib/age'
  import { tilde } from '../lib/paths'
  import Arrow from '../lib/icons/Arrow.svelte'
  import Chevron from '../lib/icons/Chevron.svelte'
  import CommitList from '../lib/CommitList.svelte'
  import Diff from '../lib/Diff.svelte'
  import FileList from '../lib/FileList.svelte'
  import TopBar from '../lib/TopBar.svelte'
  import Warning from '../lib/icons/Warning.svelte'
  import type { Commit, CommitDetail, File, Pair, Target } from '../lib/types'

  let {
    api,
    pair,
    home,
    onpairs,
  }: { api: Api; pair: Pair; home: string | undefined; onpairs: () => void } = $props()

  let branch = $state('')
  let commits = $state<Commit[]>([])
  let target = $state<Target | undefined>(undefined)
  let detail = $state<CommitDetail | undefined>(undefined)
  let selected = $state('')
  let openFile = $state('')
  let diff = $state('')
  let skip = $state<string[]>([])
  let failure = $state('')

  /** Where the target stands in the list, and whether it is an exact match. */
  const at = $derived.by(() => {
    const p = target?.position
    if (!p) return -1
    const id = p.match || p.nearest
    return id ? commits.findIndex((c) => c.id === id) : -1
  })
  const isNearest = $derived(!!target?.position.nearest && !target.position.match)
  /** Commits the target does not have yet — everything above where it stands. */
  const behind = $derived(at >= 0 ? at : commits.length)

  const commit = $derived(commits.find((c) => c.id === selected))
  const files = $derived<File[]>(detail?.files ?? [])
  const crossing = $derived(files.filter((f) => !f.excluded && !skip.includes(f.path)))
  const openMeta = $derived(files.find((f) => f.path === openFile))

  /** A transfer of floe's waiting in the target — screens 7 and 8. */
  const waiting = $derived(target?.transfer ?? null)
  /** The user's own changes: dirty with no transfer of floe's to explain them. */
  const theirs = $derived(target && target.dirty.length > 0 && !waiting ? target.dirty : [])
  const clean = $derived(!!target && target.dirty.length === 0)
  const canPreview = $derived(clean && crossing.length > 0)

  const tags = $derived.by(() => {
    if (!waiting) return {}
    const state = target && target.conflicts.length > 0 ? 'conflict' : 'staged'
    return { [waiting.commit]: state }
  })

  void load()

  async function load() {
    try {
      const [list, state] = await Promise.all([api.commits(pair.id), api.target(pair.id)])
      branch = list.branch
      commits = list.commits
      target = state
      if (commits.length > 0) await select(firstToCross())
    } catch (e) {
      failure = message(e)
    }
  }

  /** The next commit to carry: the oldest one the target does not have yet. */
  function firstToCross(): string {
    if (at > 0) return commits[at - 1]!.id
    if (at === 0) return commits[0]!.id
    return commits[commits.length - 1]!.id
  }

  async function select(id: string) {
    selected = id
    detail = undefined
    openFile = ''
    diff = ''
    skip = []
    try {
      const d = await api.commit(pair.id, id)
      // A slower request for a commit since deselected must not win.
      if (selected !== id) return
      detail = d
      const first = d.files.find((f) => !f.excluded)
      if (first) await show(first.path)
    } catch (e) {
      failure = message(e)
    }
  }

  async function show(path: string) {
    openFile = path
    diff = ''
    const of = selected
    try {
      const d = await api.diff(pair.id, of, path)
      if (openFile !== path || selected !== of) return
      diff = d.diff
    } catch (e) {
      failure = message(e)
    }
  }

  function setSkip(path: string, crossThis: boolean) {
    skip = crossThis ? skip.filter((p) => p !== path) : [...skip, path]
  }

  async function recheck() {
    try {
      target = await api.target(pair.id)
    } catch (e) {
      failure = message(e)
    }
  }

  // Floe checks the target again when its window regains focus: the user may
  // have gone to commit or discard what is in the way.
  $effect(() => {
    const onfocus = () => void recheck()
    window.addEventListener('focus', onfocus)
    return () => window.removeEventListener('focus', onfocus)
  })

  function message(e: unknown): string {
    return e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e)
  }
</script>

<div class="screen">
  <TopBar>
    <span class="slash">/</span>
    <button class="switcher" type="button" onclick={onpairs}>
      <span class="names">
        {pair.source.name}
        <Arrow />
        {pair.target.name}
      </span>
      <Chevron down />
    </button>
  </TopBar>

  {#if failure}
    <div class="banner">
      <Warning colour="var(--refusal)" />
      <span>{failure}</span>
      <button type="button" onclick={() => (failure = '')}>Dismiss</button>
    </div>
  {/if}

  <div class="columns">
    <!-- Source -->
    <section class="column source">
      <div class="head">
        <span class="eyebrow">Source</span>
        <span class="name">{pair.source.name}</span>
        <span class="mono sub" title={pair.source.path}>
          {tilde(pair.source.path, home)}{branch ? ` · ${branch}` : ''}
        </span>
      </div>
      <CommitList
        {commits}
        {selected}
        {at}
        nearest={isNearest}
        targetName={pair.target.name}
        {tags}
        onselect={select}
      />
    </section>

    <!-- Crossing -->
    <section class="column crossing">
      {#if commit}
        <div class="head wide">
          <span class="eyebrow">Crossing</span>
          <span class="subject">{commit.subject}</span>
          <span class="by">
            <span class="mono">{commit.id.slice(0, 7)}</span>
            · {commit.authorName} · {since(commit.authorTime)}
          </span>
          {#if body(detail?.message ?? '')}
            <p class="body">{body(detail?.message ?? '')}</p>
          {/if}
        </div>

        {#if detail}
          <FileList {files} selected={openFile} {skip} onselect={show} onskip={setSkip} />
          {#if openMeta}
            <div class="openbar">
              <span class="mono name">{openMeta.path}</span>
              {#if !openMeta.binary}
                <span class="mono added">+{openMeta.added}</span>
                <span class="mono deleted">−{openMeta.deleted}</span>
              {/if}
            </div>
            <div class="diffpane">
              {#if diff}
                <Diff {diff} path={openMeta.path} />
              {:else}
                <p class="aside">Reading {openMeta.path}…</p>
              {/if}
            </div>
          {:else}
            <p class="aside">This commit changes no file that crosses.</p>
          {/if}
        {:else}
          <p class="aside">Reading the commit…</p>
        {/if}
      {:else}
        <p class="aside">{commits.length === 0 ? 'This source has no commits.' : 'Reading…'}</p>
      {/if}
    </section>

    <!-- Target -->
    <section class="column target">
      <div class="head">
        <span class="eyebrow">Target</span>
        <span class="name">{pair.target.name}</span>
        <span class="mono sub" title={pair.target.path}>
          {tilde(pair.target.path, home)}{target?.branch ? ` · ${target.branch}` : ''}{target?.head
            ? ` @ ${target.head.slice(0, 7)}`
            : ''}
        </span>
      </div>

      {#if target}
        <div class="facts">
          <div class="kv">
            <span>Working tree</span>
            {#if clean}
              <span class="ok"><span class="pip"></span>Clean</span>
            {:else}
              <span class="warn">
                {target.dirty.length}
                {target.dirty.length === 1 ? 'file changed' : 'files changed'}
              </span>
            {/if}
          </div>
          <div class="kv">
            <span>Stands at</span>
            {#if target.position.match}
              <span><span class="mono">{target.position.match.slice(0, 7)}</span> · exact match</span>
            {:else if target.position.nearest}
              <span>
                <span class="mono">{target.position.nearest.slice(0, 7)}</span> · nearest,
                {target.position.divergent.length}
                {target.position.divergent.length === 1 ? 'file differs' : 'files differ'}
              </span>
            {:else}
              <span class="muted">nowhere in this history</span>
            {/if}
          </div>
          <div class="kv">
            <span>Behind source</span>
            <span>{behind} {behind === 1 ? 'commit' : 'commits'}</span>
          </div>
          <div class="kv">
            <span>Never crosses</span>
            <span>{pair.exclude.length} {pair.exclude.length === 1 ? 'pattern' : 'patterns'}</span>
          </div>
          <div class="kv last">
            <span>Content guard</span>
            <span>{pair.guard.length} {pair.guard.length === 1 ? 'pattern' : 'patterns'}</span>
          </div>
        </div>

        {#if theirs.length > 0}
          <!-- Screen 4: the target has changes of its own. -->
          <div class="blocked">
            <span class="line warn"><Warning /> {pair.target.name} has uncommitted changes</span>
            <ul class="changed">
              {#each theirs as c (c.path)}
                <li><span class="mono st">{c.status}</span><span class="mono">{c.path}</span></li>
              {/each}
            </ul>
            <span class="muted">
              Commit or discard them first, so two transfers cannot mix into one commit.
              Untracked files don't count.
            </span>
          </div>
        {:else if waiting}
          <div class="blocked">
            <span class="line ok-text">A transfer of floe's is waiting in {pair.target.name}.</span>
            <span class="muted">
              The staged and conflict screens are the next pass of area 5.
            </span>
          </div>
        {/if}
      {/if}

      <div class="spacer"></div>

      <div class="actions">
        <span class="muted says">
          {#if !target}
            Reading {pair.target.name}…
          {:else if theirs.length > 0 || waiting}
            Nothing will be transferred while {pair.target.name} has changes.
          {:else if !detail}
            Reading the commit…
          {:else if files.length === 0}
            This commit changes no file.
          {:else if files.every((f) => f.excluded)}
            Every file this commit changes is excluded from this pair.
          {:else if crossing.length === 0}
            Every file is unticked, so nothing would cross.
          {:else}
            {crossing.length}
            {crossing.length === 1 ? 'file' : 'files'} will be staged in {pair.target.name}, with
            this commit's message ready in your commit box. Nothing is committed.
          {/if}
        </span>
        <div class="buttons">
          {#if !clean}
            <button class="ghost" type="button" onclick={recheck}>Check again</button>
          {/if}
          <button class="primary" type="button" disabled={!canPreview}>Preview transfer</button>
        </div>
      </div>
    </section>
  </div>
</div>

<script lang="ts" module>
  /** A commit message past its subject line. */
  function body(message: string): string {
    const cut = message.indexOf('\n')
    return cut < 0 ? '' : message.slice(cut + 1).trim()
  }
</script>

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

  .columns {
    flex-grow: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: var(--column-source) minmax(0, 1fr) var(--column-target);
  }

  .column {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .source,
  .crossing {
    border-right: 1px solid var(--border);
  }

  .crossing {
    min-width: 0;
  }

  .head {
    padding: 16px 18px 14px;
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex-shrink: 0;
  }

  .head.wide {
    padding: 16px 20px 14px;
    gap: 5px;
  }

  .name {
    font-size: var(--size-name);
    font-weight: 600;
    color: var(--text-bright);
  }

  .sub {
    font-size: var(--size-path);
    color: var(--text-faint);
    /* The column is a fixed width; the whole path is in the title. */
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .subject {
    font-size: var(--size-subject);
    font-weight: 600;
    color: var(--text-bright);
  }

  .by {
    color: var(--text-muted);
  }

  .body {
    margin: 2px 0 0;
    color: #aeb4bd;
    line-height: 19px;
    white-space: pre-wrap;
  }

  .openbar {
    height: 40px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 20px;
    border-bottom: 1px solid var(--border);
  }

  .openbar .name {
    font-size: var(--size-mono);
    font-weight: 400;
  }

  .added {
    color: var(--add-count);
  }

  .deleted {
    color: var(--del-count);
  }

  .diffpane {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
  }

  .aside {
    margin: 0;
    padding: 14px 20px;
    color: var(--text-faint);
  }

  .facts {
    padding: 4px 18px;
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
  }

  .kv {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
    padding: 11px 0;
    border-bottom: 1px solid var(--border-inner);
  }

  .kv.last {
    border-bottom: none;
  }

  .kv > span:first-child {
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .kv > span:last-child {
    text-align: right;
  }

  .ok {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--accent);
  }

  .ok-text {
    color: var(--accent);
  }

  .pip {
    width: 6px;
    height: 6px;
    border-radius: 3px;
    background: var(--accent);
  }

  .warn {
    color: var(--attention);
  }

  .muted {
    color: var(--text-muted);
  }

  .blocked {
    padding: 14px 18px;
    border-top: 1px solid var(--border-inner);
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-height: 0;
    overflow-y: auto;
  }

  .line {
    display: flex;
    align-items: center;
    gap: 7px;
  }

  .changed {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .changed li {
    display: flex;
    gap: 8px;
    min-width: 0;
  }

  .changed .mono {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .st {
    color: var(--attention);
    flex-shrink: 0;
  }

  .spacer {
    flex-grow: 1;
  }

  .actions {
    padding: 18px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 12px;
    flex-shrink: 0;
  }

  .says {
    line-height: 19px;
  }

  .buttons {
    display: flex;
    gap: 8px;
  }

  .primary,
  .ghost {
    height: 38px;
    border-radius: var(--radius);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    flex: 1 1 auto;
  }

  .primary {
    border: 0;
    background: var(--accent);
    color: #0f1a18;
  }

  .primary:disabled {
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }

  .ghost {
    border: 1px solid var(--control);
    background: none;
    color: var(--text);
    font-weight: 400;
  }

  .banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 18px;
    background: rgba(224, 130, 111, 0.1);
    border-bottom: 1px solid rgba(224, 130, 111, 0.3);
    color: var(--refusal);
    flex-shrink: 0;
  }

  .banner button {
    margin-left: auto;
    padding: 4px 10px;
    border: 1px solid var(--control);
    border-radius: 4px;
    background: none;
    font: inherit;
    color: var(--text);
    cursor: pointer;
  }
</style>
