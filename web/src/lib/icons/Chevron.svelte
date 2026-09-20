<script lang="ts">
  // Right points at where a row leads; down opens a menu.
  let {
    size = 14,
    colour = 'var(--text-faint)',
    down = false,
  }: { size?: number; colour?: string; down?: boolean } = $props()
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
  <path d={down ? 'M3.5 5.5L7 9l3.5-3.5' : 'M5.5 3.5L9 7l-3.5 3.5'} />
</svg>
