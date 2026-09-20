<script lang="ts">
  /*
   * The files a commit changes. An excluded file stays in the list, struck
   * through and labelled: a file that quietly vanished would be
   * indistinguishable from one the commit never touched. Individual files can
   * be unticked for a single transfer.
   *
   * A preview or a refusal replaces a row's line counts with what it found
   * there: what the file will do when it lands, or that the guard matched in
   * it.
   */
  import type { File } from './types'

  let {
    files,
    selected,
    skip,
    tags = {},
    locked = false,
    onselect,
    onskip,
  }: {
    files: File[]
    selected: string
    /** Paths unticked for this transfer. */
    skip: string[]
    /** By path: a result or a refusal, in place of the line counts. */
    tags?: Record<string, Tag>
    /*
     * A preview is computed for the ticked files it was given, so unticking
     * one would describe a transfer floe has not checked. Screen 6 leaves this
     * off: unticking the file the guard matched in is the way out of it.
     */
    locked?: boolean
    onselect: (path: string) => void
    onskip: (path: string, crossing: boolean) => void
  } = $props()

  const crossing = $derived(files.filter((f) => !f.excluded && !skip.includes(f.path)).length)

  // Excluded files sink to the bottom: the list reads as what crosses, with
  // what never does beneath it. The sort is stable, so each group keeps git's
  // path order. Unticking does not move a row — it is a choice for one
  // transfer, not the pair's configuration, and the row would jump under the
  // click that made it.
  const ordered = $derived(
    [...files].sort((a, b) => Number(a.excluded) - Number(b.excluded)),
  )
</script>

<div class="files">
  <div class="eyebrow count">Files · {crossing} of {files.length} cross</div>
  {#each ordered as f (f.path)}
    {@const ticked = !f.excluded && !skip.includes(f.path)}
    <div class="row" class:selected={f.path === selected} class:out={!ticked}>
      <button
        class="tick"
        type="button"
        disabled={f.excluded || locked}
        aria-label={f.excluded
          ? `${f.path} never crosses`
          : `${ticked ? 'Do not transfer' : 'Transfer'} ${f.path}`}
        aria-pressed={ticked}
        onclick={() => onskip(f.path, !ticked)}
      >
        {#if ticked}
          <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
            <rect x="0.75" y="0.75" width="12.5" height="12.5" rx="2" fill="#6fc2b4" stroke="#6fc2b4" />
            <path d="M3.5 7.2l2.3 2.3 4.7-4.9" fill="none" stroke="#15171b" stroke-width="1.6" />
          </svg>
        {:else}
          <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
            <rect x="0.75" y="0.75" width="12.5" height="12.5" rx="2" fill="none" stroke="#3a3f47" />
            <path d="M4 7h6" stroke="#4a5059" stroke-width="1.5" />
          </svg>
        {/if}
      </button>

      <button class="open" type="button" onclick={() => onselect(f.path)}>
        <span class="mono status {statusRole(f.status)}">{f.status}</span>
        <span class="mono path" class:struck={f.excluded}>{f.path}</span>
        {#if f.excluded}
          <span class="label">excluded</span>
        {:else if tags[f.path]}
          <span class="tag {tags[f.path]!.role}">{tags[f.path]!.text}</span>
        {:else if f.binary}
          <span class="label">binary</span>
        {:else}
          <span class="mono added">+{f.added}</span>
          <span class="mono deleted" class:none={f.deleted === 0}>−{f.deleted}</span>
        {/if}
      </button>
    </div>
  {/each}
</div>

<script lang="ts" module>
  /** What a row says instead of its counts, in the colour of its meaning. */
  export type Tag = { text: string; role: 'ok' | 'warn' | 'bad' }

  /** M amber, A green, D red — DESIGN.md's file row. */
  function statusRole(status: string): string {
    return status === 'A' ? 'add' : status === 'D' ? 'del' : 'mod'
  }
</script>

<style>
  .files {
    padding: 10px 0;
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    max-height: 40vh;
    overflow-y: auto;
  }

  .count {
    padding: 2px 20px 6px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    height: var(--row-file);
    flex-shrink: 0;
    padding: 0 20px;
  }

  .row:hover:not(.selected) {
    background: var(--raised);
  }

  .selected {
    background: var(--selected);
  }

  .out {
    color: var(--text-dim);
  }

  .tick {
    display: flex;
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
  }

  .tick:disabled {
    cursor: default;
  }

  .open {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .status {
    width: 10px;
    flex-shrink: 0;
  }

  .out .status {
    color: inherit !important;
  }

  .mod {
    color: var(--attention);
  }

  .add {
    color: var(--add-count);
  }

  .del {
    color: var(--del-count);
  }

  .path {
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .selected .path {
    color: var(--text-bright);
  }

  .out .path {
    color: inherit;
  }

  .struck {
    text-decoration: line-through;
  }

  .label {
    flex-shrink: 0;
    font-size: var(--size-path);
  }

  .tag {
    flex-shrink: 0;
    font-size: var(--size-path);
    padding: 1px 7px;
    border-radius: 3px;
  }

  .tag.ok {
    color: var(--accent);
    background: rgba(111, 194, 180, 0.1);
  }

  .tag.warn {
    color: var(--attention);
    background: rgba(217, 179, 108, 0.12);
  }

  .tag.bad {
    color: var(--refusal);
    background: rgba(224, 130, 111, 0.12);
  }

  .added {
    flex-shrink: 0;
    color: var(--add-count);
  }

  .deleted {
    flex-shrink: 0;
    color: var(--del-count);
  }

  .deleted.none {
    color: var(--text-faint);
  }

  .out .added,
  .out .deleted {
    color: inherit;
  }
</style>
