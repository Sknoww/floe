<script lang="ts">
  /*
   * One file's unified diff: two gutters of line numbers, then the line as git
   * wrote it. Floe's own component, never a library, so it follows DESIGN.md.
   * Syntax highlighting is deferred.
   *
   * A line the content guard matched is tinted coral with a coral left edge,
   * and the text the pattern matched is marked inside it.
   */
  import { parseDiff } from './diff'
  import type { Part } from './types'

  let {
    diff,
    path,
    hits = {},
  }: {
    diff: string
    path: string
    /*
     * The guard's matches in this file, by the line number they carry: for a
     * line of the patch that is its number in the file's new version, which is
     * the new gutter of an added row. A line the merge brought in is numbered
     * in the merged result instead and is never passed here — it is not in the
     * patch to point at.
     */
    hits?: Record<number, Part[]>
  } = $props()

  const parsed = $derived(parseDiff(diff))
</script>

{#if parsed.binary}
  <p class="aside">{path} is binary. git shows no lines for it.</p>
{:else if parsed.rows.length === 0}
  <p class="aside">{parsed.note || `${path} has no lines to show.`}</p>
{:else}
  <div class="diff">
    {#each parsed.rows as row, i (i)}
      {@const parts = row.kind === 'add' ? cut(hits[row.new], row.text) : undefined}
      <div class="row {row.kind}" class:hit={parts}>
        <span class="gutter">{row.kind === 'context' || row.kind === 'delete' ? row.old : ''}</span>
        <span class="gutter">{row.kind === 'context' || row.kind === 'add' ? row.new : ''}</span>
        <span class="text"
          >{#if parts}+{#each parts as part, p (p)}<span
                class:mark={part.matched}>{part.text}</span
              >{/each}{:else}{row.text}{/if}</span
        >
      </div>
    {/each}
  </div>
{/if}

<script lang="ts" module>
  /**
   * The parts of a matched line, but only when they still spell the row git
   * wrote. The row carries the patch's leading "+" and the parts do not, so
   * they must rebuild the rest of it exactly; anything else means the match
   * and the diff are describing different text, and marking it would put the
   * highlight over the wrong characters.
   */
  function cut(parts: Part[] | undefined, text: string): Part[] | undefined {
    if (!parts || parts.length === 0) return undefined
    return parts.map((p) => p.text).join('') === text.slice(1) ? parts : undefined
  }
</script>

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

  /* The line the guard refused the transfer over. */
  .hit {
    background: var(--hit-bg);
    color: var(--hit-text);
    box-shadow: inset 2px 0 0 var(--hit-edge);
  }

  .mark {
    background: var(--mark-bg);
    color: var(--mark-text);
    border-radius: 2px;
  }

  .aside {
    margin: 0;
    padding: 14px 20px;
    color: var(--text-faint);
  }
</style>
