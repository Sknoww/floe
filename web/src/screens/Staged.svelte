<script lang="ts">
  /*
   * Screen 8 — the transfer landed, staged in the target, in the target column
   * of the main screen's shell.
   *
   * Nothing is committed and nothing will be: the panel shows the carried
   * message exactly as it waits in the commit box and says so plainly, because
   * a tool that has just written to a repository is the one place where "floe
   * never commits" could be assumed rather than known.
   */
  import Check from '../lib/icons/Check.svelte'

  let {
    count,
    message,
    targetName,
    busy,
    onopen,
    ondiscard,
  }: {
    /** Files staged in the target. */
    count: number
    /** The carried message, as it waits in SQUASH_MSG. */
    message: string
    targetName: string
    /** The editor is opening, or a discard is running. */
    busy: boolean
    onopen: () => void
    ondiscard: () => void
  } = $props()

  const subject = $derived(message.split('\n', 1)[0] ?? '')
  const body = $derived.by(() => {
    const cut = message.indexOf('\n')
    return cut < 0 ? '' : message.slice(cut + 1).trim()
  })
</script>

<div class="body">
  <div class="callout">
    <span class="title"><Check />Staged, ready to commit</span>
    <span class="under">
      {count}
      {count === 1 ? 'file' : 'files'} staged in {targetName}.
    </span>
  </div>

  {#if message}
    <div class="group">
      <span class="eyebrow">In your commit box</span>
      <div class="message">
        <span class="subject">{subject}</span>
        {#if body}
          <span class="msgbody">{body}</span>
        {/if}
      </div>
    </div>
  {/if}

  <span class="says">
    Commit it in your editor or with <span class="mono">git commit</span>. Floe never commits.
  </span>
</div>

<div class="spacer"></div>

<div class="actions">
  <!-- No primary: the transfer is done, and committing happens elsewhere. -->
  <button class="ghost" type="button" onclick={onopen} disabled={busy}>Open in editor</button>
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
    background: rgba(111, 194, 180, 0.07);
    border: 1px solid rgba(111, 194, 180, 0.25);
    flex-shrink: 0;
  }

  .title {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--accent);
    font-weight: 600;
  }

  .under {
    color: #aeb4bd;
    line-height: 19px;
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  /* The message as it is, wrapped but never reflowed: what is here is what
     `git commit` and the commit box will use. */
  .message {
    padding: 10px 12px;
    border-radius: 4px;
    background: var(--raised);
    border: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .subject {
    color: var(--text-bright);
    font-weight: 500;
  }

  .msgbody {
    color: #9aa1ab;
    line-height: 19px;
    white-space: pre-wrap;
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

  .ghost,
  .destructive {
    height: 38px;
    border-radius: var(--radius);
    font: inherit;
    font-weight: 600;
    background: none;
    cursor: pointer;
  }

  .ghost {
    border: 1px solid var(--control);
    color: var(--text);
  }

  /* Coral outline, never filled outside its own confirmation. */
  .destructive {
    border: 1px solid rgba(224, 130, 111, 0.45);
    color: var(--refusal);
  }

  .ghost:disabled,
  .destructive:disabled {
    border-color: transparent;
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }
</style>
