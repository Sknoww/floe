<script lang="ts">
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
  <path d="M5.5 3.5L9 7l-3.5 3.5" />
</svg>
