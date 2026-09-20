<script lang="ts">
  /*
   * An opened pair. Screen 2 — the source → crossing → target columns — is the
   * next pass of area 5; until then this shows what the pair is and what floe
   * has read about it, so the API and the dev loop are exercised end to end.
   */
  import { tilde } from '../lib/paths'
  import Arrow from '../lib/icons/Arrow.svelte'
  import Chevron from '../lib/icons/Chevron.svelte'
  import TopBar from '../lib/TopBar.svelte'
  import type { Pair } from '../lib/types'

  let {
    pair,
    home,
    onpairs,
  }: { pair: Pair; home: string | undefined; onpairs: () => void } = $props()
</script>

<div class="screen">
  <TopBar>
    <button class="switcher" type="button" onclick={onpairs}>
      {pair.source.name}
      <Arrow size={14} />
      {pair.target.name}
      <Chevron />
    </button>
  </TopBar>

  <main>
    <section class="panel">
      <div class="head">
        <span class="eyebrow">Source</span>
        <span class="name">{pair.source.name}</span>
        <span class="mono path">{tilde(pair.source.path, home)}</span>
      </div>
      <div class="head">
        <span class="eyebrow">Target</span>
        <span class="name">{pair.target.name}</span>
        <span class="mono path">{tilde(pair.target.path, home)}</span>
      </div>
    </section>

    <section class="counts">
      <span>
        <strong>{pair.exclude.length}</strong>
        {pair.exclude.length === 1 ? 'path never crosses' : 'paths never cross'}
      </span>
      <span>
        <strong>{pair.guard.length}</strong>
        {pair.guard.length === 1 ? 'guard pattern' : 'guard patterns'}
      </span>
      <span class="mono path">{tilde(pair.file, home)}</span>
    </section>

    <p class="pending">
      The source, crossing and target columns are the next pass of area 5. Nothing here
      writes to {pair.target.name}.
    </p>
  </main>
</div>

<style>
  .screen {
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .switcher {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 8px;
    margin: 0;
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

  main {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  .panel {
    display: flex;
    border-bottom: 1px solid var(--border);
  }

  .head {
    flex: 1 1 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 14px 18px;
  }

  .head:first-child {
    border-right: 1px solid var(--border);
  }

  .name {
    font-size: var(--size-name);
    font-weight: 600;
    color: var(--text-bright);
  }

  .path {
    font-size: var(--size-path);
    color: var(--text-faint);
  }

  .counts {
    display: flex;
    align-items: center;
    gap: 22px;
    padding: 12px 18px;
    border-bottom: 1px solid var(--border);
    color: var(--text-muted);
  }

  strong {
    color: var(--text-bright);
    font-weight: 600;
  }

  .pending {
    margin: 0;
    padding: 18px;
    color: var(--text-faint);
  }
</style>
