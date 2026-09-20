<script lang="ts">
  /*
   * Screen 9 — the one confirmation, shared by the conflict and the staged
   * screens, because discarding is one operation whichever of them it is
   * reached from.
   *
   * It is the only dialog in floe, and it is hand-rolled on a native
   * <dialog> opened with showModal(): that already gives the focus trap, Esc
   * and ::backdrop, which is the whole of what a primitives library would have
   * been brought in for.
   *
   * A confirmation states exactly what is lost and what is kept, so all three
   * are named here: tracked changes go, resolutions with them, untracked files
   * stay, and the carried message is removed.
   */
  let {
    open,
    targetName,
    /** Where the target goes back to. Empty when it has no commits yet. */
    branch,
    head,
    busy,
    onkeep,
    ondiscard,
  }: {
    open: boolean
    targetName: string
    branch: string
    head: string
    /** The discard is running: it finishes even if the page goes away. */
    busy: boolean
    onkeep: () => void
    ondiscard: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement | undefined>(undefined)

  /** "main @ 8e3c1aa", or as much of it as the target has. */
  const at = $derived([branch, head && head.slice(0, 7)].filter(Boolean).join(' @ '))

  $effect(() => {
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    else if (!open && dialog.open) dialog.close()
  })
</script>

<!--
  Esc and the backdrop both close the dialog on their own, so the page is told
  through close rather than by intercepting them.
-->
<dialog bind:this={dialog} onclose={onkeep} aria-labelledby="discard-title">
  <span class="title" id="discard-title">Discard this transfer?</span>
  <span class="what">
    {#if at}
      {targetName} goes back to <span class="mono">{at}</span>.
    {:else}
      {targetName} goes back to having no commits.
    {/if}
    Every change to tracked files is discarded, conflict resolutions included. Untracked files
    stay.
  </span>
  <span class="also">The carried commit message is removed too.</span>
  <div class="buttons">
    <button class="ghost" type="button" onclick={onkeep} disabled={busy}>Keep working</button>
    <button class="destructive" type="button" onclick={ondiscard} disabled={busy}>
      {busy ? 'Discarding…' : 'Discard transfer'}
    </button>
  </div>
</dialog>

<style>
  dialog {
    width: 460px;
    padding: 22px 24px;
    border: 1px solid var(--control);
    border-radius: 8px;
    background: var(--raised);
    color: var(--text);
    font: inherit;
    /* The one shadow in the app. */
    box-shadow: 0 16px 40px rgba(0, 0, 0, 0.45);
  }

  dialog[open] {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  dialog::backdrop {
    background: rgba(8, 9, 11, 0.6);
  }

  .title {
    font-size: var(--size-name);
    font-weight: 600;
    color: var(--text-bright);
  }

  .what {
    color: #aeb4bd;
    line-height: 20px;
  }

  .also {
    color: var(--text-muted);
    line-height: 20px;
  }

  /* Cancel on the left, the action on the right. */
  .buttons {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding-top: 4px;
  }

  .ghost,
  .destructive {
    height: 36px;
    padding: 0 18px;
    border-radius: var(--radius);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  .ghost {
    border: 1px solid var(--control);
    background: none;
    color: var(--text);
  }

  /* Filled coral: inside its own confirmation is the one place it is. */
  .destructive {
    border: 0;
    background: var(--refusal);
    color: #1f0f0b;
  }

  .ghost:disabled,
  .destructive:disabled {
    border-color: transparent;
    background: var(--border-inner);
    color: #5c626b;
    cursor: default;
  }
</style>
