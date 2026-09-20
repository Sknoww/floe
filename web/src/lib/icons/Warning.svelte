<script lang="ts">
  let { size = 14, colour = 'var(--attention)' }: { size?: number; colour?: string } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 14 14"
  fill="none"
  stroke={colour}
  stroke-width="1.4"
  aria-hidden="true"
>
  <path d="M7 1.8l5.6 10H1.4z" />
  <path d="M7 5.6v2.6M7 9.8v.3" />
</svg>
