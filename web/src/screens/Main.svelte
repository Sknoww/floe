<script lang="ts">
  /*
   * The three-column shell — source, crossing, target — left to right in the
   * direction a commit travels, and the screens that live in it: screen 2, the
   * main screen; screen 4 when the target has changes; screen 5, the preview;
   * screen 6, a refusal by the content guard; screen 7, a transfer stopped on
   * a conflict; and screen 8, one staged and waiting to be committed.
   *
   * Which of them is on screen is the phase below. The columns are the same
   * three throughout — only the crossing detail and the target column change,
   * and the target column is where the target's state and every action that
   * writes to it live, so each phase's is its own file beside this one.
   *
   * Screens 5 and 6 are where the user is, so the page holds them. Screens 7
   * and 8 are where the *target* is: a transfer of floe's waiting in it, which
   * outlives the page and is read back from the target itself. So they are not
   * chosen here — they are what the target says, and a reload or a restart
   * lands on them again.
   */
  import { Api, ApiError } from '../lib/api'
  import { parseConflict } from '../lib/conflict'
  import { since } from '../lib/age'
  import { tilde } from '../lib/paths'
  import Arrow from '../lib/icons/Arrow.svelte'
  import Chevron from '../lib/icons/Chevron.svelte'
  import CommitList from '../lib/CommitList.svelte'
  import Conflict from '../lib/Conflict.svelte'
  import ConflictPanel from './Conflict.svelte'
  import Diff from '../lib/Diff.svelte'
  import Discard from '../lib/Discard.svelte'
  import FileList from '../lib/FileList.svelte'
  import GuardPanel from './GuardRefused.svelte'
  import PreviewPanel from './Preview.svelte'
  import StagedPanel from './Staged.svelte'
  import TopBar from '../lib/TopBar.svelte'
  import Warning from '../lib/icons/Warning.svelte'
  import type { Tag } from '../lib/FileList.svelte'
  import type {
    Commit,
    CommitDetail,
    Conflict as ConflictFile,
    File,
    Match,
    Pair,
    Preview,
    Part,
    Target,
    Transfer,
  } from '../lib/types'

  let {
    api,
    pair,
    home,
    onpairs,
    onsettings,
    onpair,
  }: {
    api: Api
    pair: Pair
    home: string | undefined
    onpairs: () => void
    /** Screen 3, from the counts that say how many patterns there are. */
    onsettings: () => void
    /** The pair as the config file now has it, when a re-read finds it moved. */
    onpair: (p: Pair) => void
  } = $props()

  let branch = $state('')
  let commits = $state<Commit[]>([])
  let target = $state<Target | undefined>(undefined)
  let detail = $state<CommitDetail | undefined>(undefined)
  let selected = $state('')
  let openFile = $state('')
  let diff = $state('')
  /** The open file as it stands in the target, markers and all — screen 7. */
  let conflict = $state<ConflictFile | undefined>(undefined)
  let skip = $state<string[]>([])
  let failure = $state('')

  /** Which screen the shell is showing. */
  type Phase = 'browse' | 'preview' | 'guard' | 'conflict' | 'staged'
  /** The phases the page chooses; the target's own override them below. */
  let stage = $state<'browse' | 'preview' | 'guard'>('browse')
  /** Screen 5: the checks that passed and what the transfer will do. */
  let preview = $state<Preview | undefined>(undefined)
  /** Screen 6: every match of the check that refused. */
  let matches = $state<Match[]>([])
  /** Screen 9: the one confirmation, for a discard from either screen. */
  let confirming = $state(false)
  /** A preview, an apply, a discard or a launch is running. */
  let busy = $state(false)

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
  const conflicts = $derived(target?.conflicts ?? [])
  /** The user's own changes: dirty with no transfer of floe's to explain them. */
  const theirs = $derived(target && target.dirty.length > 0 && !waiting ? target.dirty : [])
  const clean = $derived(!!target && target.dirty.length === 0)
  const canPreview = $derived(clean && crossing.length > 0 && !busy)

  /*
   * A transfer waiting in the target is not one of the page's own states: it
   * is in the repository, and it is what the screen is about until it is
   * committed or discarded.
   */
  const phase = $derived<Phase>(waiting ? (conflicts.length > 0 ? 'conflict' : 'staged') : stage)
  const landed = $derived(phase === 'conflict' || phase === 'staged')

  const tags = $derived.by(() => {
    if (!waiting) return {}
    return { [waiting.commit]: conflicts.length > 0 ? 'conflict' : 'staged' }
  })

  /*
   * What crossed, and what each file did. Conflicted files come first: they
   * are the ones still waiting on the user.
   */
  const crossed = $derived.by(() => {
    if (!waiting) return []
    return files
      .filter((f) => !f.excluded && !waiting.skip.includes(f.path))
      .map((f) => ({ path: f.path, conflicted: conflicts.includes(f.path) }))
      .sort((a, b) => Number(b.conflicted) - Number(a.conflicted))
  })

  /*
   * What each crossing file's row says in place of its line counts: what the
   * preview found it will do, what it did, or that the guard matched in it. In
   * none of those cases does the row move — the list is still what crosses, in
   * git's order.
   */
  const fileTags = $derived.by<Record<string, Tag>>(() => {
    const out: Record<string, Tag> = {}
    if (phase === 'preview' && preview) {
      for (const f of preview.result.files) {
        out[f.path] = preview.result.conflicts.includes(f.path)
          ? { text: 'will conflict', role: 'warn' }
          : { text: 'merges cleanly', role: 'ok' }
      }
    } else if (landed) {
      for (const f of crossed) {
        out[f.path] = f.conflicted
          ? { text: 'conflict', role: 'warn' }
          : { text: 'staged', role: 'ok' }
      }
    } else if (phase === 'guard') {
      for (const m of matches) if (m.path) out[m.path] = { text: 'guard', role: 'bad' }
    }
    return out
  })

  /** The file open in the crossing column will land with markers in it. */
  const openPredicted = $derived(
    phase === 'preview' && !!preview && preview.result.conflicts.includes(openFile),
  )
  /** It already has, and the markers are on disk in the target. */
  const openConflicted = $derived(landed && conflicts.includes(openFile))

  /*
   * The guard's matches in the open file, by line number, for the diff to mark.
   * A line the merge brought in is numbered in the merged result rather than in
   * the file's new version, so it is left out: the patch has no line there to
   * point at, and the match block already says where it came from.
   */
  const hits = $derived.by<Record<number, Part[]>>(() => {
    if (phase !== 'guard') return {}
    const out: Record<number, Part[]> = {}
    for (const m of matches) if (m.path === openFile && !m.merged) out[m.lineNo] = m.parts
    return out
  })

  void load()

  async function load() {
    try {
      const [list, state] = await Promise.all([api.commits(pair.id), api.target(pair.id)])
      branch = list.branch
      commits = list.commits
      target = state
      // A transfer waiting in the target picks the commit itself, below.
      if (commits.length > 0 && !state.transfer) await select(firstToCross())
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

  /*
   * A transfer waiting in the target decides what is on screen: the commit it
   * carried, with the files it left out still unticked, and the first
   * conflicted file open.
   */
  $effect(() => {
    const w = waiting
    if (w && selected !== w.commit) void carry(w)
  })

  async function carry(w: Transfer) {
    await select(w.commit, conflicts[0])
    if (selected === w.commit) skip = w.skip
  }

  async function select(id: string, prefer?: string) {
    selected = id
    detail = undefined
    openFile = ''
    diff = ''
    conflict = undefined
    skip = []
    back()
    try {
      const d = await api.commit(pair.id, id)
      // A slower request for a commit since deselected must not win.
      if (selected !== id) return
      detail = d
      const first = d.files.find((f) => f.path === prefer) ?? d.files.find((f) => !f.excluded)
      if (first) await show(first.path)
    } catch (e) {
      failure = message(e)
    }
  }

  async function show(path: string) {
    openFile = path
    diff = ''
    conflict = undefined
    const of = selected
    try {
      // A file left with markers is read out of the target's working tree:
      // what is on disk there is what has to be resolved, and the commit's own
      // diff no longer describes it.
      if (landed && conflicts.includes(path)) {
        const c = await api.conflict(pair.id, path)
        if (openFile !== path || selected !== of) return
        conflict = c
        return
      }
      // The diff is fetched even for a file the preview shows as it will land
      // instead: it is the same request either way, and it means Back returns to
      // a diff that is already there rather than to an empty pane.
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

  /** Back to screen 2, dropping a preview or a refusal that no longer holds. */
  function back() {
    stage = 'browse'
    preview = undefined
    matches = []
  }

  /*
   * Screen 5. Every check that can refuse the transfer runs here, before
   * anything visible is written, so a refusal has nothing to undo — it is a
   * screen, not an accident.
   */
  async function runPreview() {
    busy = true
    const of = selected
    try {
      const p = await api.preview(pair.id, of, skip)
      if (selected !== of) return
      matches = []
      preview = p
      stage = 'preview'
      // A predicted conflict is the thing to look at, so open it.
      const first = p.result.conflicts[0]
      if (first) await show(first)
    } catch (e) {
      if (selected !== of) return
      if (e instanceof ApiError && e.code === 'guard') {
        // Screen 6. The target is untouched, so the refusal is the whole
        // outcome: show every match, over the file the first one is in.
        preview = undefined
        matches = e.matches
        stage = 'guard'
        // Over the file the first match is in, unless it is already open: a
        // later match's file is not the one to jump to.
        const first = matches.find((m) => m.path)
        if (first && first.path !== openFile) await show(first.path)
        return
      }
      back()
      failure = message(e)
      // A target that changed under us is screen 4, and says so itself.
      if (e instanceof ApiError && e.code === 'dirty') await recheck()
    } finally {
      busy = false
    }
  }

  /*
   * Applies the preview the screen is showing. The target is re-checked first,
   * and a HEAD that moved since means the preview described a transfer that no
   * longer holds: floe previews again rather than applying a stale one.
   */
  async function apply() {
    if (!preview) return
    busy = true
    try {
      await api.apply(pair.id, preview.id)
      back()
      // The target now holds the transfer, and says which screen that is.
      target = await api.target(pair.id)
      const first = (target.conflicts ?? [])[0]
      if (first) await show(first)
    } catch (e) {
      failure = message(e)
      if (e instanceof ApiError && (e.code === 'stale' || e.code === 'preview_gone')) {
        busy = false
        await runPreview()
        return
      }
      back()
      await recheck()
    } finally {
      busy = false
    }
  }

  /*
   * Screens 7 and 8. The editor is opened on the target — VS Code, where the
   * carried message is already in the commit box — at the conflicted file, and
   * at the marker git wrote when floe has that file's text to find it in.
   */
  async function openEditor() {
    const path = conflicts.includes(openFile) ? openFile : (conflicts[0] ?? '')
    const line =
      conflict && conflict.path === path && !conflict.binary
        ? (parseConflict(conflict.content).rows.find((r) => r.marker)?.no ?? 0)
        : 0
    busy = true
    try {
      await api.editor(pair.id, path, line)
    } catch (e) {
      failure = message(e)
    } finally {
      busy = false
    }
  }

  /*
   * Screen 9's action. Discard is `git reset --hard`, so the server offers it
   * only for a transfer floe recorded — changes floe did not make are never
   * discarded.
   */
  async function discard() {
    busy = true
    try {
      await api.discard(pair.id)
      confirming = false
      openFile = ''
      conflict = undefined
      target = await api.target(pair.id)
      // The transfer is gone, so the commit is a commit again: reopen it, this
      // time with nothing unticked.
      if (selected) await select(selected)
    } catch (e) {
      confirming = false
      failure = message(e)
      await recheck()
    } finally {
      busy = false
    }
  }

  /** Screen 6's way out that leaves the pair's settings alone. */
  function untick(path: string) {
    setSkip(path, false)
  }

  /*
   * The target, and the pair's own patterns beside it: floe re-reads the
   * config file on every request, so a hand edit applies at once on the server
   * and the counts on this screen would otherwise say something else. Patterns
   * that moved change what crosses, so the commit is read again too.
   */
  async function recheck() {
    try {
      const [state, fresh] = await Promise.all([api.target(pair.id), api.pair(pair.id)])
      target = state
      const moved = !same(fresh.exclude, pair.exclude) || !same(fresh.guard, pair.guard)
      onpair(fresh)
      // Not while a transfer waits: its files are the ones it carried, and
      // reselecting would drop what it left out.
      if (moved && selected && !waiting) await select(selected)
    } catch (e) {
      failure = message(e)
    }
  }

  // Floe checks the target again when its window regains focus: the user may
  // have gone to commit or discard what is in the way, or to resolve a
  // conflict floe stopped on.
  $effect(() => {
    const onfocus = () => void recheck()
    window.addEventListener('focus', onfocus)
    return () => window.removeEventListener('focus', onfocus)
  })

  // A conflict resolved in the editor stops being one, and the file the
  // crossing column is showing goes back to being the commit's own diff.
  $effect(() => {
    if (landed && openFile && conflict && !conflicts.includes(openFile)) void show(openFile)
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
        locked={landed}
        onselect={select}
      />
    </section>

    <!-- Crossing -->
    <section class="column crossing">
      {#if commit}
        <div class="head wide">
          <span class="eyebrow">Crossing{eyebrow(phase)}</span>
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
          <FileList
            {files}
            selected={openFile}
            {skip}
            tags={fileTags}
            locked={phase === 'preview'}
            applied={landed}
            onselect={show}
            onskip={setSkip}
          />
          {#if openMeta}
            <div class="openbar">
              <span class="mono name">{openMeta.path}</span>
              {#if openConflicted}
                <span class="muted">in {pair.target.name}’s working tree</span>
              {:else if openPredicted}
                <span class="muted">what lands in {pair.target.name}</span>
              {:else if !openMeta.binary}
                <span class="mono added">+{openMeta.added}</span>
                <span class="mono deleted">−{openMeta.deleted}</span>
              {/if}
            </div>
            <div class="diffpane">
              {#if openConflicted}
                <!-- The file as git wrote it, out of the target's working tree. -->
                {#if !conflict}
                  <p class="aside">Reading {openMeta.path} from {pair.target.name}…</p>
                {:else if conflict.binary}
                  <p class="aside">
                    {openMeta.path} is binary, so there are no lines to show. Resolve it in your editor.
                  </p>
                {:else}
                  <Conflict
                    text={conflict.content}
                    targetName={pair.target.name}
                    commit={selected}
                  />
                {/if}
              {:else if openPredicted}
                <!-- The file as git will write it, markers and all. -->
                <Conflict
                  text={preview?.result.conflicted?.[openMeta.path] ?? ''}
                  targetName={pair.target.name}
                  commit={selected}
                />
              {:else if diff}
                <Diff {diff} path={openMeta.path} {hits} />
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

      {#if phase === 'conflict'}
        <ConflictPanel
          files={crossed}
          targetName={pair.target.name}
          {busy}
          onopen={openEditor}
          ondiscard={() => (confirming = true)}
        />
      {:else if phase === 'staged'}
        <StagedPanel
          count={crossed.length}
          message={target?.message ?? ''}
          targetName={pair.target.name}
          {busy}
          onopen={openEditor}
          ondiscard={() => (confirming = true)}
        />
      {:else if phase === 'preview' && preview}
        <PreviewPanel
          {preview}
          targetName={pair.target.name}
          {busy}
          onback={back}
          onapply={apply}
        />
      {:else if phase === 'guard'}
        <GuardPanel
          {matches}
          targetName={pair.target.name}
          {skip}
          excluded={files.filter((f) => f.excluded).map((f) => f.path)}
          {busy}
          onopen={show}
          onuntick={untick}
          onguard={onsettings}
          onpreview={runPreview}
        />
      {:else}
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
            <!-- The counts are the way in to the patterns behind them. -->
            <button class="kv link" type="button" onclick={onsettings}>
              <span>Never crosses</span>
              <span>{pair.exclude.length} {pair.exclude.length === 1 ? 'pattern' : 'patterns'}</span>
            </button>
            <button class="kv link last" type="button" onclick={onsettings}>
              <span>Content guard</span>
              <span>{pair.guard.length} {pair.guard.length === 1 ? 'pattern' : 'patterns'}</span>
            </button>
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
          {/if}
        {/if}

        <div class="spacer"></div>

        <div class="actions">
          <span class="muted says">
            {#if !target}
              Reading {pair.target.name}…
            {:else if theirs.length > 0}
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
            <button class="primary" type="button" disabled={!canPreview} onclick={runPreview}>
              {busy ? 'Previewing…' : 'Preview transfer'}
            </button>
          </div>
        </div>
      {/if}
    </section>
  </div>
</div>

<!-- Screen 9: one confirmation, since discarding is one operation. -->
<Discard
  open={confirming}
  targetName={pair.target.name}
  branch={target?.branch ?? ''}
  head={target?.head ?? ''}
  {busy}
  onkeep={() => (confirming = false)}
  ondiscard={discard}
/>

<script lang="ts" module>
  /** Two pattern lists, as the config file orders them. */
  function same(a: string[], b: string[]): boolean {
    return a.length === b.length && a.every((p, i) => p === b[i])
  }

  /** A commit message past its subject line. */
  function body(message: string): string {
    const cut = message.indexOf('\n')
    return cut < 0 ? '' : message.slice(cut + 1).trim()
  }

  /*
   * What the crossing column's eyebrow says the commit is doing. A guard
   * refusal leaves it alone: nothing happened to the commit, which is the
   * whole point of that screen.
   */
  function eyebrow(phase: string): string {
    return phase === 'preview' || phase === 'conflict' || phase === 'staged' ? ` · ${phase}` : ''
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
    line-height: 19px;
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

  /* A row that leads somewhere, drawn as the rows around it. */
  .kv.link {
    border-left: 0;
    border-right: 0;
    border-top: 0;
    background: none;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .kv.link:hover > span:last-child {
    color: var(--accent);
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
