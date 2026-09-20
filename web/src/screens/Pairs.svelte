<script lang="ts">
  /*
   * Screen 1 — Pairs (bare `floe`). The pairs floe remembers, most recently
   * opened first. A pair whose repository is missing, or whose file cannot be
   * read, stays listed with why: a pair that quietly vanished would be
   * indistinguishable from one that was never remembered.
   */
  import { since } from '../lib/age'
  import { base, dir, tilde } from '../lib/paths'
  import Arrow from '../lib/icons/Arrow.svelte'
  import Blocked from '../lib/icons/Blocked.svelte'
  import Chevron from '../lib/icons/Chevron.svelte'
  import TopBar from '../lib/TopBar.svelte'
  import Warning from '../lib/icons/Warning.svelte'
  import type { Pairs, Remembered } from '../lib/types'

  let {
    pairs,
    onopen,
  }: { pairs: Pairs; onopen: (id: string) => void } = $props()

  const home = $derived(pairs.home)
  /** Where a broken pair file is fixed, named from the file floe listed. */
  const pairsDir = $derived(pairs.pairs.length > 0 ? dir(pairs.pairs[0]!.file) : '')

  function openable(p: Remembered): boolean {
    return !!p.id && !p.error && !(p.missing && p.missing.length > 0)
  }
</script>

<div class="screen">
  <TopBar />

  <main>
    <div class="title">
      <h1>Pairs</h1>
      <span class="muted">Most recently opened first.</span>
    </div>

    {#snippet row(p: Remembered)}
      <div class="what">
        {#if p.error}
          <!-- A file floe cannot read: its own words, and where to fix it. -->
          <span class="line refusal">
            <Blocked />
            <span class="mono">{base(p.file)}</span> can't be read
          </span>
          <span class="mono reason">{p.error}</span>
          {#if pairsDir}
            <span class="faint">
              Fix the file in <span class="mono">{tilde(pairsDir, home)}/</span>, then reload.
            </span>
          {/if}
        {:else if p.source && p.target}
          {@const gone = p.missing ?? []}
          <span class="names" class:dim={gone.length > 0}>
            {p.source.name}
            <Arrow colour={gone.length > 0 ? '#4a5059' : 'var(--text-faint)'} />
            {p.target.name}
          </span>
          {#if gone.length > 0}
            <span class="line attention">
              <Warning />
              <span class="mono">{gone.map((m) => tilde(m, home)).join(', ')}</span>
              {gone.length === 1 ? 'is' : 'are'} missing
            </span>
            <span class="faint">
              Moved or deleted. A pair is remembered by both paths, so moving a repository
              leaves its pair behind.
            </span>
          {:else}
            <span class="mono paths">
              {tilde(p.source.path, home)} → {tilde(p.target.path, home)}
            </span>
          {/if}
        {/if}
      </div>

      {#if p.lastOpened}
        <span class="when" class:faint={!openable(p)}>opened {since(p.lastOpened)}</span>
      {/if}
    {/snippet}

    {#if pairs.pairs.length > 0}
      <ul class="list">
        {#each pairs.pairs as p (p.file)}
          <li>
            {#if openable(p)}
              <button class="row openable" type="button" onclick={() => onopen(p.id!)}>
                {@render row(p)}
                <Chevron />
              </button>
            {:else}
              <div class="row">
                {@render row(p)}
                <span class="no-chevron"></span>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {:else}
      <p class="empty">floe remembers no pairs yet.</p>
    {/if}

    <div class="terminal">
      <span>Open a new pair from a terminal</span>
      <span class="mono command">floe ~/dev/source ~/dev/target</span>
    </div>
  </main>
</div>

<style>
  .screen {
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: auto;
  }

  main {
    width: 820px;
    align-self: center;
    padding: 64px 0 64px;
    display: flex;
    flex-direction: column;
    gap: 24px;
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
    line-height: 1.2;
  }

  .muted {
    color: var(--text-muted);
  }

  .faint {
    color: var(--text-faint);
  }

  .list {
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
  }

  li:not(:last-child) .row {
    border-bottom: 1px solid var(--border-inner);
  }

  .row {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 16px 20px;
    /* A button resets: the row is the control, and looks like a row. */
    margin: 0;
    border: 0;
    background: none;
    font: inherit;
    color: inherit;
    text-align: left;
  }

  .openable {
    cursor: pointer;
  }

  /* The most recently opened pair is where the eye starts. */
  li:first-child .openable {
    background: var(--raised);
  }

  .openable:hover {
    background: var(--selected);
  }

  .what {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .names {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 15px;
    font-weight: 500;
    color: var(--text-bright);
  }

  .names.dim {
    color: var(--text-muted);
  }

  .paths {
    font-size: var(--size-path);
    color: var(--text-faint);
  }

  .line {
    display: flex;
    align-items: center;
    gap: 7px;
  }

  .attention {
    color: var(--attention);
  }

  .refusal {
    color: var(--refusal);
  }

  /* git's and floe's own words, shown as they are. */
  .reason {
    color: #c9a39c;
  }

  .when {
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .no-chevron {
    width: 14px;
    flex-shrink: 0;
  }

  .empty {
    margin: 0;
    color: var(--text-muted);
  }

  .terminal {
    display: flex;
    align-items: center;
    gap: 14px;
    color: var(--text-muted);
  }

  .command {
    color: var(--text);
    padding: 5px 10px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bar);
  }
</style>
