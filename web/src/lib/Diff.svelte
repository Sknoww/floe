<script lang="ts">
  /*
   * One file's unified diff: two gutters of line numbers, then the line as git
   * wrote it. Floe's own component, never a library, so it follows DESIGN.md.
   * Syntax highlighting is deferred.
   */
  import { parseDiff } from './diff'

  let { diff, path }: { diff: string; path: string } = $props()

  const parsed = $derived(parseDiff(diff))
</script>

{#if parsed.binary}
  <p class="aside">{path} is binary. git shows no lines for it.</p>
{:else if parsed.rows.length === 0}
  <p class="aside">{parsed.note || `${path} has no lines to show.`}</p>
{:else}
  <div class="diff">
    {#each parsed.rows as row, i (i)}
      <div class="row {row.kind}">
        <span class="gutter">{row.kind === 'context' || row.kind === 'delete' ? row.old : ''}</span>
        <span class="gutter">{row.kind === 'context' || row.kind === 'add' ? row.new : ''}</span>
        <span class="text">{row.text}</span>
      </div>
    {/each}
  </div>
{/if}

<style>
  .diff {
    padding: 10px 0;
    font-family: var(--font-mono);
    font-size: var(--size-mono);
    line-height: var(--row-diff);
  }

  .row {
    display: grid;
    grid-template-columns: 48px 48px 1fr;
  }

  .gutter {
    color: var(--line-number);
    text-align: right;
    padding-right: 12px;
    /* The numbers are furniture: selecting the diff should copy the code. */
    user-select: none;
  }

  .text {
    white-space: pre;
    overflow-wrap: normal;
  }

  .add {
    background: var(--add-bg);
    color: var(--add-text);
  }

  .delete {
    background: var(--del-bg);
    color: var(--del-text);
  }

  .hunk {
    background: var(--hunk-bg);
    color: var(--hunk);
  }

  .note {
    color: var(--text-faint);
  }

  .aside {
    margin: 0;
    padding: 14px 20px;
    color: var(--text-faint);
  }
</style>
