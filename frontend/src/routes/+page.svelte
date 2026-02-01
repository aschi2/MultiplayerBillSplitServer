<script lang="ts">
  import { goto } from '$app/navigation';
  import { initialsFromName } from '$lib/utils';
  import { getApiBase } from '$lib/api';

  let showCreate = false;
  let showJoin = false;

  let createName = '';
  let billName = '';

  let joinCode = '';
  let joinName = '';

  const apiBase = getApiBase();

  const submitCreate = async () => {
    const res = await fetch(`${apiBase}/create-room`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: createName, bill_name: billName })
    });
    if (!res.ok) return;
    const data = await res.json();
    const identity = {
      userId: data.user_id,
      name: createName,
      initials: initialsFromName(createName),
      colorSeed: data.user_id.slice(0, 6)
    };
    localStorage.setItem(`room:${data.room_code}:identity`, JSON.stringify(identity));
    goto(`/room/${data.room_code}`);
  };

  const submitJoin = async () => {
    const res = await fetch(`${apiBase}/join-room`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ room_code: joinCode.toUpperCase(), name: joinName })
    });
    if (!res.ok) return;
    const data = await res.json();
    const identity = {
      userId: data.user_id,
      name: joinName,
      initials: initialsFromName(joinName),
      colorSeed: data.user_id.slice(0, 6)
    };
    localStorage.setItem(`room:${data.room_code}:identity`, JSON.stringify(identity));
    goto(`/room/${data.room_code}`);
  };
</script>

<div class="min-h-screen flex flex-col bg-surface-900 text-surface-50">
  <header class="p-6">
    <div class="accent-gradient rounded-3xl p-6 shadow-2xl text-center">
      <h1 class="text-3xl font-semibold text-white">Themepark Split</h1>
      <p class="text-white/80 mt-2">AI powered multiplayer bill splitting.</p>
    </div>
  </header>

  <main class="flex-1 px-6 pb-24 space-y-4">
    <div
      class="glass-card rounded-3xl p-6 space-y-3 text-center cursor-pointer hover:border-primary-400/60 transition"
      role="button"
      tabindex="0"
      on:click={() => (showCreate = true)}
      on:keydown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          showCreate = true;
        }
      }}
    >
      <h2 class="text-xl font-semibold text-white">Create a new bill</h2>
      <p class="text-surface-200">Start a shared room and invite your crew.</p>
      <button class="btn btn-primary w-full" on:click={() => (showCreate = true)}>Create Bill</button>
    </div>

    <div
      class="glass-card rounded-3xl p-6 space-y-3 text-center cursor-pointer hover:border-primary-400/60 transition"
      role="button"
      tabindex="0"
      on:click={() => (showJoin = true)}
      on:keydown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          showJoin = true;
        }
      }}
    >
      <h2 class="text-xl font-semibold text-white">Join an existing bill</h2>
      <p class="text-surface-200">Enter a room code to jump in.</p>
      <button class="btn btn-secondary w-full" on:click={() => (showJoin = true)}>Join Bill</button>
    </div>
  </main>

  {#if showCreate}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Create Bill</h3>
        <label class="block">
          <span class="text-sm text-surface-200">Your name</span>
          <input class="input w-full" bind:value={createName} placeholder="Alex" />
        </label>
        <label class="block">
          <span class="text-sm text-surface-200">Bill name (optional)</span>
          <input class="input w-full" bind:value={billName} placeholder="Dinner at Sora" />
        </label>
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showCreate = false)}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={submitCreate} disabled={!createName}>Start</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showJoin}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Join Bill</h3>
        <label class="block">
          <span class="text-sm text-surface-200">Room code</span>
          <input class="input w-full uppercase" bind:value={joinCode} placeholder="AB12CD" />
        </label>
        <label class="block">
          <span class="text-sm text-surface-200">Your name</span>
          <input class="input w-full" bind:value={joinName} placeholder="Alex" />
        </label>
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showJoin = false)}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={submitJoin} disabled={!joinCode || !joinName}>Join</button>
        </div>
      </div>
    </div>
  {/if}
</div>
