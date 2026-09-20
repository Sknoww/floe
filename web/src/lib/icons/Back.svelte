<script lang="ts">
  // The way back out of a screen that is not the transfer.
  let { size = 14, colour = 'var(--text-muted)' }: { size?: number; colour?: string } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 14 14"
  fill="none"
  stroke={colour}
  stroke-width="1.5"
  aria-hidden="true"
>
  <path d="M11 7H3M6 4L3 7l3 3" />
</svg>
