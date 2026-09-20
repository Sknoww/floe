<script lang="ts">
  let { size = 14, colour = 'var(--refusal)' }: { size?: number; colour?: string } = $props()
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
  <circle cx="7" cy="7" r="5.5" />
  <path d="M3.1 10.9l7.8-7.8" />
</svg>
