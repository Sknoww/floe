<script lang="ts">
  /*
   * Screen 6 — refused by the content guard, in the target column of the main
   * screen's shell.
   *
   * The guard runs before anything visible is written, so the refusal has
   * nothing to undo and the callout can say so plainly. Each match names its
   * pattern, where it matched and the line, in floe's own words — and offers
   * the one way out that does not change the pair's settings: untick the file.
   */
  import Blocked from '../lib/icons/Blocked.svelte'
  import type { Match } from '../lib/types'

  let {
    matches,
    targetName,
    skip,
    excluded,
    busy,
    onopen,
    onuntick,
    onguard,
    onpreview,
  }: {
    matches: Match[]
    targetName: string
    /** Paths already unticked for this transfer. */
    skip: string[]
    /** Paths the pair never transfers: there is nothing to untick. */
    excluded: string[]
    busy: boolean
    /** Shows the matched file in the crossing column. */
    onopen: (path: string) => void
    onuntick: (path: string) => void
    /** The pair's settings, where its guard patterns are. */
    onguard: () => void
    onpreview: () => void
  } = $props()

  /*
   * A match in the commit message names no file, and one already unticked or
   * excluded has nothing left to untick — the message would be an action that
   * does nothing.
   */
  function untickable(m: Match): boolean {
    return m.path !== '' && !skip.includes(m.path) && !excluded.includes(m.path)
  }
</script>

<div class="body">
  <div class="callout">
    <span class="title"><Blocked />Refused by the content guard</span>
    <span class="under">
      {matches.length === 1 ? '1 match' : `${matches.length} matches`}. {targetName} is untouched.
    </span>
  </div>

  {#each matches as m, i (i)}
    <div class="match">
      <span class="mono pattern">{m.pattern}</span>
      <span class="where">
        {#if m.path}
          <button class="mono path" type="button" onclick={() => onopen(m.path)}>{m.path}</button>
        {:else}
          the commit message
        {/if}
        · line {m.lineNo}
        {#if m.merged}
          <span class="tag">brought in by the merge</span>
        {/if}
      </span>
      <span class="mono line">{m.line}</span>
      {#if untickable(m)}
        <button class="untick" type="button" onclick={() => onuntick(m.path)}>
          Untick this file
        </button>
      {/if}
    </div>
  {/each}
</div>

<div class="spacer"></div>

<div class="actions">
  <button class="ghost" type="button" onclick={onguard}>Edit guard</button>
  <button class="ghost" type="button" onclick={onpreview} disabled={busy}>
    {busy ? 'Previewing…' : 'Preview again'}
  </button>
</div>

<style>
  .body {
    padding: 14px 18px 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow-y: auto;
  }

  .callout {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px;
    border-radius: var(--radius);
    background: rgba(224, 130, 111, 0.08);
    border: 1px solid rgba(224, 130, 111, 0.3);
    flex-shrink: 0;
  }

  .title {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--refusal);
    font-weight: 600;
  }

  .under {
    color: #aeb4bd;
    line-height: 19px;
  }

  .match {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px 0;
    border-bottom: 1px solid var(--border-inner);
    flex-shrink: 0;
  }

  .pattern {
    color: var(--text-bright);
    word-break: break-all;
  }

  .where {
    color: var(--text-muted);
  }

  .path {
    padding: 0;
    border: 0;
    background: none;
    font-family: var(--font-mono);
    font-size: var(--size-mono);
    color: inherit;
    cursor: pointer;
  }

  .path:hover {
    color: var(--text-bright);
  }

  .tag {
    margin-left: 4px;
    font-size: var(--size-path);
    padding: 1px 7px;
    border-radius: 3px;
    color: var(--attention);
    background: rgba(217, 179, 108, 0.12);
  }

  /* The line as it is, on one line: the diff is where it is read in full. */
  .line {
    font-size: var(--size-path);
    color: var(--match-line);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .untick {
    align-self: flex-start;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    color: var(--accent);
    cursor: pointer;
  }

  .untick:hover {
    color: #9adbd0;
  }

  .spacer {
    flex-grow: 1;
  }

  .actions {
    padding: 18px;
    border-top: 1px solid var(--border);
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }

  /* No primary: neither way out of a refusal is the one floe recommends. */
  .ghost {
    flex: 1 1 0;
    height: 38px;
    border-radius: var(--radius);
    border: 1px solid var(--control);
    background: none;
    font: inherit;
    font-weight: 600;
    color: var(--text);
    cursor: pointer;
  }

  .ghost:disabled {
    border-color: transparent;
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }
</style>
