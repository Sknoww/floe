<script lang="ts">
  /*
   * Screen 7 — the transfer stopped on a conflict, in the target column of the
   * main screen's shell.
   *
   * The markers are in the target's working tree and the unmerged entries in
   * its index: floe has done its writing, and what is left is the user's. So
   * the panel says what landed and what did not, and offers the editor — where
   * the carried message is already in the commit box — beside the one way back.
   */
  import Warning from '../lib/icons/Warning.svelte'

  let {
    files,
    targetName,
    busy,
    onopen,
    ondiscard,
  }: {
    /** What each crossing file did, conflicts first. */
    files: { path: string; conflicted: boolean }[]
    targetName: string
    /** The editor is opening, or a discard is running. */
    busy: boolean
    /** Opens the target in the editor, at the first conflicted file. */
    onopen: () => void
    ondiscard: () => void
  } = $props()

  const conflicted = $derived(files.filter((f) => f.conflicted).length)
  const staged = $derived(files.length - conflicted)
</script>

<div class="body">
  <div class="callout">
    <span class="title"><Warning />Stopped on a conflict</span>
    <span class="under">
      {staged}
      {staged === 1 ? 'file' : 'files'} staged cleanly. {conflicted}
      {conflicted === 1 ? 'file has' : 'files have'} conflict markers.
    </span>
  </div>

  <div class="results">
    {#each files as f (f.path)}
      <div class="res" class:lands={f.conflicted}>
        <span class="mono">{f.path}</span>
        <span class="tag {f.conflicted ? 'warn' : 'ok'}">{f.conflicted ? 'conflict' : 'staged'}</span>
      </div>
    {/each}
  </div>

  <span class="says">
    Resolve the markers in your editor and stage the file. Floe checks again when you come back
    to its window. The commit message is already in your commit box.
  </span>
</div>

<div class="spacer"></div>

<div class="actions">
  <button class="primary" type="button" onclick={onopen} disabled={busy}>Open in editor</button>
  <button class="destructive" type="button" onclick={ondiscard} disabled={busy}>
    Discard transfer
  </button>
</div>

<style>
  .body {
    padding: 14px 18px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    min-height: 0;
    overflow-y: auto;
  }

  .callout {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px;
    border-radius: var(--radius);
    background: rgba(217, 179, 108, 0.08);
    border: 1px solid rgba(217, 179, 108, 0.25);
    flex-shrink: 0;
  }

  .title {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--attention);
    font-weight: 600;
  }

  .under {
    color: #aeb4bd;
    line-height: 19px;
  }

  .results {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .res {
    display: flex;
    align-items: center;
    gap: 10px;
    height: var(--row-file);
  }

  .res .mono {
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* The conflicted files are the ones still to be dealt with. */
  .res.lands .mono {
    color: var(--text-bright);
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

  .says {
    color: #9aa1ab;
    line-height: 19px;
  }

  .spacer {
    flex-grow: 1;
  }

  .actions {
    padding: 18px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex-shrink: 0;
  }

  .primary,
  .destructive {
    height: 38px;
    border-radius: var(--radius);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  /* Resolving is what floe recommends, so the editor is the one primary. */
  .primary {
    border: 0;
    background: var(--accent);
    color: #0f1a18;
  }

  /* Coral outline, never filled outside its own confirmation. */
  .destructive {
    border: 1px solid rgba(224, 130, 111, 0.45);
    background: none;
    color: var(--refusal);
  }

  .primary:disabled,
  .destructive:disabled {
    border-color: transparent;
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }
</style>
