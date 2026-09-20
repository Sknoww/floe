<script lang="ts">
  import type { Snippet } from 'svelte'

  // 48px, `floe` on the left and whatever the screen puts beside it.
  let { children }: { children?: Snippet } = $props()
</script>

<header>
  <span class="wordmark mono">floe</span>
  {@render children?.()}
</header>

<style>
  header {
    height: var(--bar-height);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 18px;
    border-bottom: 1px solid var(--border);
    background: var(--bar);
  }

  .wordmark {
    font-weight: 700;
    font-size: 15px;
    color: var(--text-bright);
  }
</style>
