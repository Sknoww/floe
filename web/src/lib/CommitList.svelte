<script lang="ts">
  /*
   * The source's first-parent history, newest first. A state dot says whether
   * the target has the commit yet, and a teal divider reads "<target> is here"
   * between where the target stands and the commits after it.
   */
  import { since } from './age'
  import type { Commit } from './types'

  let {
    commits,
    selected,
    /** Index of the commit the target stands at, or -1. */
    at,
    /** True when `at` is the nearest rather than an exact match. */
    nearest,
    targetName,
    tags,
    onselect,
  }: {
    commits: Commit[]
    selected: string
    at: number
    nearest: boolean
    targetName: string
    tags: Record<string, string>
    onselect: (id: string) => void
  } = $props()

  /** Older history: the target already has it, so it is not going to cross. */
  const older = (i: number) => at >= 0 && i > at
</script>

<div class="list">
  {#each commits as c, i (c.id)}
    {#if i === at}
      <div class="divider">
        <span class="rule"></span>
        <span class="here">{targetName} is {nearest ? 'nearest here' : 'here'}</span>
        <span class="rule"></span>
      </div>
    {/if}
    <button
      class="row"
      class:selected={c.id === selected}
      class:older={older(i)}
      type="button"
      onclick={() => onselect(c.id)}
    >
      <span class="dot" class:at={i === at} class:past={older(i)} class:on={c.id === selected}
      ></span>
      <span class="mono id">{c.id.slice(0, 7)}</span>
      <span class="subject">{c.subject}</span>
      {#if tags[c.id]}
        <span class="tag {tags[c.id]}">{tags[c.id]}</span>
      {:else}
        <span class="age">{short(c.authorTime)}</span>
      {/if}
    </button>
  {/each}
</div>

<script lang="ts" module>
  /** "2h", "1d" — the column is 340px wide and the subject needs the room. */
  function short(when: string): string {
    const phrase = since(when)
    const m = /^(\d+) (\w)/.exec(phrase)
    if (m) return m[1]! + m[2]!
    return phrase === 'moments ago' ? 'now' : phrase.startsWith('a minute') ? '1m' : phrase
  }
</script>

<style>
  .list {
    padding-top: 6px;
    display: flex;
    flex-direction: column;
    overflow-y: auto;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    height: var(--row-commit);
    flex-shrink: 0;
    padding: 0 16px 0 14px;
    border: 0;
    border-left: 2px solid transparent;
    background: none;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .row:hover:not(.selected) {
    background: var(--raised);
  }

  .selected {
    background: var(--selected);
    border-left-color: var(--accent);
  }

  .older {
    color: var(--text-muted);
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 4px;
    flex-shrink: 0;
    /* Hollow: a commit the target does not have yet. */
    border: 1.5px solid #8a919b;
  }

  .dot.on {
    border-color: var(--accent);
  }

  /* Filled teal: where the target stands. */
  .dot.at {
    border-color: var(--accent);
    background: var(--accent);
  }

  /* Filled grey: older history. */
  .dot.past {
    border-color: #3a3f47;
    background: #3a3f47;
  }

  .id {
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .selected .id {
    color: #9aa1ab;
  }

  .older .id {
    color: inherit;
  }

  .subject {
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .selected .subject {
    color: var(--text-bright);
  }

  .age {
    flex-shrink: 0;
    color: var(--text-faint);
  }

  .older .age {
    color: inherit;
  }

  /* While a transfer is in progress, its state replaces the age. */
  .tag {
    flex-shrink: 0;
    font-size: var(--size-path);
    padding: 1px 6px;
    border-radius: 3px;
  }

  .tag.staged {
    color: var(--accent);
    background: rgba(111, 194, 180, 0.12);
  }

  .tag.conflict {
    color: var(--attention);
    background: rgba(217, 179, 108, 0.12);
  }

  .divider {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 16px;
  }

  .rule {
    flex-grow: 1;
    height: 1px;
    background: var(--accent-line);
  }

  .here {
    color: var(--accent);
    font-size: var(--size-mono);
  }
</style>
