<script lang="ts">
  // The direction a commit travels: source → target.
  let { size = 16, colour = 'var(--text-faint)' }: { size?: number; colour?: string } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 16 16"
  fill="none"
  stroke={colour}
  stroke-width="1.5"
  aria-hidden="true"
>
  <path d="M3 8h10M9 4l4 4-4 4" />
</svg>
