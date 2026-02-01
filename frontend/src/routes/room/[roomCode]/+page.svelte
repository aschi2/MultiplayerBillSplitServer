<script lang="ts">
  import { onMount } from 'svelte';
  import Avatar from '$lib/components/Avatar.svelte';
  import { formatCents } from '$lib/utils';
  import type { RoomDoc, ReceiptParseResult, Item, Participant } from '$lib/types';

  export let data;
  let roomCode = data.roomCode as string;
  let ws: WebSocket | null = null;
  let room: RoomDoc | null = null;
  let identity = { userId: '', name: '', initials: '', colorSeed: '' };
  let showAssign = false;
  let activeItemId: string | null = null;
  let receiptResult: ReceiptParseResult | null = null;
  let showReceiptReview = false;
  let warningBanner = false;
  let items: Item[] = [];
  let participants: Participant[] = [];

  const apiBase = import.meta.env.VITE_API_BASE_URL;
  const wsBase = import.meta.env.VITE_WS_BASE_URL;

  const connectWS = () => {
    ws = new WebSocket(`${wsBase}/${roomCode}`);
    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
    if (message.type === 'snapshot') {
      room = message.doc;
    }
      if (message.type === 'op') {
        // naive re-fetch by requesting snapshot is omitted in MVP
      }
    };
  };

  const toggleAssign = (itemId: string, userId: string) => {
    if (!ws || !room) return;
    const item = room.items[itemId];
    const currently = item.assigned?.[userId] ?? false;
    ws.send(
      JSON.stringify({
        type: 'op',
        op: {
          kind: 'assign_item',
          actor_id: identity.userId,
          payload: { item_id: itemId, user_id: userId, on: !currently }
        }
      })
    );
  };

  const submitReceipt = async (event: Event) => {
    const target = event.target as HTMLInputElement;
    if (!target.files?.[0]) return;
    const form = new FormData();
    form.append('file', target.files[0]);
    const res = await fetch(`${apiBase}/receipt/parse`, { method: 'POST', body: form });
    if (!res.ok) return;
    const result = (await res.json()) as ReceiptParseResult;
    receiptResult = result;
    warningBanner = result.warnings?.length > 0;
    showReceiptReview = true;
  };

  const confirmReceipt = () => {
    if (!receiptResult || !ws) return;
    receiptResult.items.forEach((item: ReceiptParseResult['items'][number], index: number) => {
      ws?.send(
        JSON.stringify({
          type: 'op',
          op: {
            kind: 'set_item',
            actor_id: identity.userId,
            payload: {
              item: {
                id: `${Date.now()}-${index}`,
                name: item.name,
                quantity: item.quantity ?? 1,
                unit_price_cents: item.unit_price_cents ?? item.line_price_cents ?? 0,
                line_price_cents: item.line_price_cents ?? 0,
                discount_cents: item.discount_cents ?? 0,
                discount_percent: item.discount_percent ?? 0,
                assigned: {}
              }
            }
          }
        })
      );
    });
    showReceiptReview = false;
  };

  onMount(() => {
    const stored = localStorage.getItem(`room:${roomCode}:identity`);
    if (stored) {
      identity = JSON.parse(stored);
    }
    connectWS();
  });

  $: items = room ? (Object.values(room.items) as Item[]) : [];
  $: participants = room ? (Object.values(room.participants) as Participant[]) : [];
</script>

<div class="min-h-screen bg-surface-900 text-surface-50 pb-24">
  <header class="px-6 pt-6 pb-4 space-y-3">
    <div class="accent-gradient rounded-3xl p-5 shadow-2xl flex items-center justify-between">
      <div>
        <p class="text-sm text-white/70">Room</p>
        <h1 class="text-2xl font-semibold text-white">{room?.name || 'Shared Bill'}</h1>
      </div>
      <div class="flex items-center gap-2">
        <Avatar initials={identity.initials} color={`#${identity.colorSeed}`} size={44} />
        <button class="btn btn-sm btn-outline border-white/30 text-white">Change name</button>
      </div>
    </div>
    <div class="flex gap-2 overflow-x-auto pt-2">
      {#if room}
        {#each participants as participant}
          <div class="flex flex-col items-center text-xs">
            <Avatar initials={participant.initials} color={`#${participant.colorSeed}`} size={36} />
            <span class="text-surface-200">{participant.name}</span>
          </div>
        {/each}
      {/if}
    </div>
  </header>

  <main class="px-6 space-y-4">
    {#if warningBanner}
      <div class="rounded-xl bg-warning-500/20 text-warning-200 px-4 py-3 text-sm border border-warning-500/40">
        Receipt import is incomplete—please review.
      </div>
    {/if}

    <section class="space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Items</h2>
        {#if room && Object.keys(room.items).length === 0}
          <label class="btn btn-sm btn-outline">
            Upload receipt
            <input type="file" class="hidden" accept="image/*" on:change={submitReceipt} />
          </label>
        {:else}
          <span class="text-xs text-surface-200">Receipt upload only available before items.</span>
        {/if}
      </div>
      {#if room}
        {#each items as item}
          <div
            role="button"
            tabindex="0"
            class="glass-card rounded-2xl p-4 flex items-center justify-between w-full text-left"
            on:click={() => toggleAssign(item.id, identity.userId)}
            on:keydown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                toggleAssign(item.id, identity.userId);
              }
            }}
          >
            <div>
              <p class="font-semibold text-white">{item.name}</p>
              <p class="text-sm text-surface-200">{formatCents(item.line_price_cents)}</p>
            </div>
            <div class="flex items-center gap-2">
              <button class="btn btn-sm btn-outline" on:click|stopPropagation={() => { showAssign = true; activeItemId = item.id; }}>
                Assign...
              </button>
              {#if item.assigned?.[identity.userId]}
                <span class="badge bg-primary-500 text-white">Me</span>
              {/if}
            </div>
          </div>
        {/each}
      {/if}
    </section>
  </main>

  <div class="fixed bottom-0 inset-x-0 bg-surface-900/90 border-t border-surface-800 px-6 py-3 flex gap-2 backdrop-blur">
    <button class="btn btn-primary flex-1">Add Item</button>
    <button class="btn btn-outline">Tax/Tip</button>
    <button class="btn btn-outline">Summary</button>
  </div>

  {#if showAssign && activeItemId && room}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Assign participants</h3>
        <div class="space-y-2">
          {#each participants as participant}
            <button
              class="w-full flex items-center justify-between rounded-xl border border-surface-800 px-4 py-3 hover:border-primary-500/60"
              on:click={() => toggleAssign(activeItemId!, participant.id)}
            >
              <div class="flex items-center gap-2">
                <Avatar initials={participant.initials} color={`#${participant.colorSeed}`} size={32} />
                <span>{participant.name}</span>
              </div>
              <span class="text-sm text-surface-500">Tap to toggle</span>
            </button>
          {/each}
        </div>
        <button class="btn btn-outline w-full" on:click={() => (showAssign = false)}>Done</button>
      </div>
    </div>
  {/if}

  {#if showReceiptReview && receiptResult}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 max-h-[80vh] overflow-y-auto text-white">
        <h3 class="text-lg font-semibold">Review receipt</h3>
        <p class="text-sm text-surface-200">Edit items before importing.</p>
        {#each receiptResult.items as item, index}
          <div class="border border-surface-800 rounded-xl p-3">
            <input class="input w-full" bind:value={receiptResult.items[index].name} />
          </div>
        {/each}
        <button class="btn btn-primary w-full" on:click={confirmReceipt}>Import Items</button>
        <button class="btn btn-outline w-full" on:click={() => (showReceiptReview = false)}>Cancel</button>
      </div>
    </div>
  {/if}
</div>
