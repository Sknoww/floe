<script lang="ts">
  /*
   * Screen 5 — the preview, in the target column of the main screen's shell.
   * The target column is where the target's state and every action that writes
   * to it live, so it is the whole of what this screen changes.
   *
   * Reaching here means every check that can refuse the transfer has already
   * passed and nothing visible has been written — at most unreachable objects
   * (see CONTEXT.md). So the checks are listed as passed, and what is left is
   * what the transfer will do.
   */
  import Check from '../lib/icons/Check.svelte'
  import Warning from '../lib/icons/Warning.svelte'
  import type { Preview } from '../lib/types'

  let {
    preview,
    targetName,
    busy,
    onback,
    onapply,
  }: {
    preview: Preview
    targetName: string
    /** An apply is running: it finishes even if the page goes away. */
    busy: boolean
    onback: () => void
    onapply: () => void
  } = $props()

  const conflicts = $derived(preview.result.conflicts)
</script>

<div class="body">
  <div class="group">
    <span class="eyebrow">Checks</span>
    <span class="passed"><Check />Working tree clean</span>
    <span class="passed"><Check />Guard passed on the patch and message</span>
    <span class="passed"><Check />Guard passed on what lands</span>
    {#if preview.binary.length > 0}
      <!--
        The guard scans lines, and a binary patch is base85: it crosses
        unscanned. CONTEXT.md asks for a visible gap rather than a silent one,
        so the files are named here, under the checks they are outside of.
      -->
      <span class="gap">
        The guard reads lines, and git writes none for a binary file. These cross unscanned:
        {#each preview.binary as path (path)}
          <span class="mono">{path}</span>
        {/each}
      </span>
    {/if}
  </div>

  <div class="group tight">
    <span class="eyebrow what">What happens</span>
    {#each preview.result.files as file (file.path)}
      <div class="res">
        <span class="mono">{file.path}</span>
        {#if conflicts.includes(file.path)}
          <span class="tag warn">conflict</span>
        {:else}
          <span class="tag ok">staged</span>
        {/if}
      </div>
    {/each}
  </div>

  {#if conflicts.length > 0}
    <div class="callout">
      <Warning />
      <span>
        {conflicts.length === 1
          ? '1 file will stop with conflict markers. You resolve it in your editor before committing.'
          : `${conflicts.length} files will stop with conflict markers. You resolve them in your editor before committing.`}
      </span>
    </div>
  {/if}
</div>

<div class="spacer"></div>

<div class="actions">
  <span class="says">
    Checked against <span class="mono">{preview.targetHead.slice(0, 7) || 'no commit'}</span>. If
    {targetName}'s HEAD moves before you apply, floe previews again.
  </span>
  <div class="buttons">
    <button class="ghost" type="button" onclick={onback} disabled={busy}>Back</button>
    <button class="primary" type="button" onclick={onapply} disabled={busy}>
      {busy ? 'Applying…' : 'Apply transfer'}
    </button>
  </div>
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

  .group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .group.tight {
    gap: 2px;
  }

  .what {
    padding-bottom: 6px;
  }

  .passed {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .gap {
    color: var(--text-muted);
    line-height: 19px;
  }

  .gap .mono {
    color: var(--text-faint);
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

  .callout {
    display: flex;
    gap: 10px;
    padding: 10px 12px;
    border-radius: var(--radius);
    background: rgba(217, 179, 108, 0.08);
    border: 1px solid rgba(217, 179, 108, 0.25);
    line-height: 19px;
  }

  /* The icon sits with the first line rather than centred on the block. */
  .callout :global(svg) {
    flex-shrink: 0;
    margin-top: 3px;
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
    color: var(--text-muted);
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
  }

  /* Apply is the one primary in the panel, and carries the weight to match. */
  .ghost {
    flex: 1 1 0;
    border: 1px solid var(--control);
    background: none;
    color: var(--text);
    font-weight: 400;
  }

  .primary {
    flex: 2 1 0;
    border: 0;
    background: var(--accent);
    color: #0f1a18;
  }

  .primary:disabled,
  .ghost:disabled {
    background: var(--border-inner);
    border-color: transparent;
    color: #5c626b;
    cursor: default;
  }
</style>
