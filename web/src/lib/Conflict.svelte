<script lang="ts">
  /*
   * A conflicted file as git writes it: one gutter of line numbers, the line
   * verbatim, and a label on the two markers that name the sides. Nothing here
   * resolves or rewrites anything — the text is what is, or will be, on disk,
   * and the numbers are the ones the editor will show.
   */
  import { parseConflict } from './conflict'

  let {
    text,
    targetName,
    commit,
  }: {
    text: string
    /** Whose lines `ours` holds. */
    targetName: string
    /** The crossing commit, whose lines `theirs` holds. */
    commit: string
  } = $props()

  const parsed = $derived(parseConflict(text))
</script>

{#if parsed.rows.length === 0}
  <p class="aside">git left this file empty.</p>
{:else}
  <div class="conflict">
    {#each parsed.rows as row (row.no)}
      <div class="row {row.side}" class:marker={row.marker}>
        <span class="gutter">{row.no}</span>
        <span class="text">{row.text}</span>
        <span class="label {row.label ?? ''}">
          {#if row.label === 'ours'}
            {targetName}’s line
          {:else if row.label === 'theirs'}
            from {commit.slice(0, 7)}
          {/if}
        </span>
      </div>
    {/each}
  </div>
{/if}

<style>
  .conflict {
    padding: 10px 0;
    font-family: var(--font-mono);
    font-size: var(--size-mono);
    line-height: var(--row-diff);
  }

  .row {
    display: grid;
    grid-template-columns: 48px 1fr auto;
    padding-right: 20px;
  }

  .gutter {
    color: var(--line-number);
    text-align: right;
    padding-right: 12px;
    /* The numbers are furniture: selecting the file should copy the code. */
    user-select: none;
  }

  .text {
    white-space: pre;
    overflow-wrap: normal;
  }

  .ours {
    background: var(--ours-bg);
  }

  .theirs {
    background: var(--theirs-bg);
  }

  /* Neither side, and only there under diff3: quiet rather than tinted. */
  .base {
    color: var(--text-dim);
  }

  .marker {
    color: var(--attention);
    background: var(--marker-bg);
  }

  .label {
    font-family: var(--font-sans);
    font-size: var(--size-path);
    padding-left: 12px;
    user-select: none;
  }

  .label.ours {
    color: var(--ours-label);
  }

  .label.theirs {
    color: var(--theirs-label);
  }

  .aside {
    margin: 0;
    padding: 14px 20px;
    color: var(--text-faint);
  }
</style>
