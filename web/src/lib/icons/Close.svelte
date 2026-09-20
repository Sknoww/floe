<script lang="ts">
  // Removes the row it sits on.
  let { size = 14, colour = 'var(--text-faint)' }: { size?: number; colour?: string } = $props()
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
  <path d="M4 4l6 6M10 4l-6 6" />
</svg>
