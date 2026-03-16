<script lang="ts">
  export let initials = '?';
  export let color = '#94a3b8';
  export let size = 40;
  export let badge: string | null = null;
  export let title: string | undefined;
  /** When set to a boolean, shows a completion ring: red (false) or green (true). */
  export let finished: boolean | undefined = undefined;

  $: ringColor = finished === true ? '#22c55e' : finished === false ? '#ef4444' : undefined;
  $: outerSize = ringColor ? size + 6 : size;
</script>

<div class="relative inline-block" title={title || initials} aria-label={title || initials} role="img">
  {#if ringColor}
    <div
      class="rounded-full flex items-center justify-center"
      style="width: {outerSize}px; height: {outerSize}px; background: {ringColor};"
    >
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        class="rounded-full"
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
    </div>
  {:else}
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
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
  {/if}
  {#if badge}
    <span
      class="absolute -bottom-1 -right-1 min-w-[16px] h-[16px] px-[4px] inline-flex items-center justify-center rounded-full bg-primary-400 text-white text-[10px] leading-none font-semibold shadow"
      aria-hidden="true"
    >
      {badge}
    </span>
  {/if}
</div>
