<script lang="ts">
  export let initials = '?';
  export let color = '#94a3b8';
  export let size = 40;
</script>

<svg
  width={size}
  height={size}
  viewBox={`0 0 ${size} ${size}`}
  aria-label={initials}
  role="img"
  class="rounded-full shadow-sm"
>
  <circle cx={size / 2} cy={size / 2} r={size / 2} fill={color} />
  <text
    x="50%"
    y="50%"
    dominant-baseline="middle"
    text-anchor="middle"
    fill="white"
    font-size={size * 0.45}
    font-family="'Inter', sans-serif"
  >
    {initials}
  </text>
</svg>
