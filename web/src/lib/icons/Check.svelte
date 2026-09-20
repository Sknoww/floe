<script lang="ts">
  // A check that passed: floe saying it is safe to go on.
  let { size = 14, colour = 'var(--accent)' }: { size?: number; colour?: string } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 14 14"
  fill="none"
  stroke={colour}
  stroke-width="1.6"
  aria-hidden="true"
>
  <path d="M2.5 7.3l3 3 6-6.3" />
</svg>
