<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { loadIdentityPrefs, saveIdentityPrefs } from '$lib/identityPrefs';
  import { loadBillHistory, saveBillHistory, upsertBillHistoryEntry, type BillHistoryEntry } from '$lib/billHistory';
  import { loadContacts } from '$lib/contacts';
  import ContactsModal from '$lib/components/ContactsModal.svelte';
  import { initialsFromName, formatCurrency, colorFromSeed } from '$lib/utils';
  import { getApiBase } from '$lib/api';
  import { EXPONENTS, SYMBOLS } from '$lib/currency';

  let showCreate = false;
  let showJoin = false;
  let showContactsModal = false;
  let createName = '';
  let billName = '';
  let createVenmoUsername = '';
  let joinCode = '';
  let joinName = '';
  let joinVenmoUsername = '';
  let createBusy = false;
  let joinBusy = false;
  let createError = '';
  let joinError = '';
  let billHistory: BillHistoryEntry[] = [];
  let billHistoryChecking = false;
  let billHistoryLoadedOnce = false;
  let billHistoryRefreshInFlight = false;
  let billHistoryRefreshTimer: ReturnType<typeof setTimeout> | null = null;
  let billHistoryClockTimer: ReturnType<typeof setInterval> | null = null;
  let billHistoryNowMs = Date.now();
  let landingModalOpen = false;
  let contactsCount = 0;

  const BILL_HISTORY_REFRESH_FAST_MS = 5000;
  const BILL_HISTORY_REFRESH_DEFAULT_MS = 30000;
  const BILL_HISTORY_REFRESH_SLOW_MS = 120000;
  const BILL_HISTORY_REFRESH_MAX_MS = 300000;

  const apiBase = getApiBase();
  const exponentFor = (code: string) => EXPONENTS[code] ?? 2;
  const factorFor = (code: string) => 10 ** exponentFor(code);
  const symbolFor = (code: string) => SYMBOLS[code] ?? '';
  const normalizeVenmoUsername = (value: string) =>
    (value || '')
      .trim()
      .replace(/^@+/, '')
      .replace(/\s+/g, '');
  const normalizeRoomCode = (value: string) => (value || '').trim().toUpperCase();
  const hexSeed = (input: string) => {
    let hash = 0x811c9dc5;
    for (let i = 0; i < input.length; i++) {
      hash ^= input.charCodeAt(i);
      hash = Math.imul(hash, 0x01000193);
    }
    return (hash >>> 0).toString(16).padStart(6, '0').slice(0, 6);
  };
  const joinedAvatarColor = (entry: BillHistoryEntry) => {
    const direct = (entry.joinedColorSeed || '').trim();
    if (direct) {
      return direct.startsWith('#') ? direct : colorFromSeed(direct);
    }
    const fallbackSeed = hexSeed(`${entry.roomCode}:${entry.joinedName || 'guest'}`);
    return colorFromSeed(fallbackSeed);
  };
  const formatShare = (entry: BillHistoryEntry) => {
    if (entry.shareCents === null || entry.shareCents === undefined) return '--';
    return formatCurrency(
      entry.shareCents,
      entry.currency || 'USD',
      symbolFor(entry.currency || 'USD'),
      exponentFor(entry.currency || 'USD')
    );
  };
  const formatTotal = (entry: BillHistoryEntry) => {
    if (entry.totalCents === null || entry.totalCents === undefined) return '--';
    return formatCurrency(
      entry.totalCents,
      entry.currency || 'USD',
      symbolFor(entry.currency || 'USD'),
      exponentFor(entry.currency || 'USD')
    );
  };
  const hasConvertedTotals = (entry: BillHistoryEntry) =>
    !!entry.targetCurrency &&
    entry.targetCurrency !== entry.currency &&
    entry.convertedTotalCents !== null &&
    entry.convertedTotalCents !== undefined;
  const hasConvertedShare = (entry: BillHistoryEntry) =>
    !!entry.targetCurrency &&
    entry.targetCurrency !== entry.currency &&
    entry.convertedShareCents !== null &&
    entry.convertedShareCents !== undefined;
  const formatConvertedTotal = (entry: BillHistoryEntry) => {
    if (!hasConvertedTotals(entry)) return '';
    return formatCurrency(
      entry.convertedTotalCents!,
      entry.targetCurrency || 'USD',
      symbolFor(entry.targetCurrency || 'USD'),
      exponentFor(entry.targetCurrency || 'USD')
    );
  };
  const formatConvertedShare = (entry: BillHistoryEntry) => {
    if (!hasConvertedShare(entry)) return '';
    return formatCurrency(
      entry.convertedShareCents!,
      entry.targetCurrency || 'USD',
      symbolFor(entry.targetCurrency || 'USD'),
      exponentFor(entry.targetCurrency || 'USD')
    );
  };
  const secondsRemainingAt = (entry: BillHistoryEntry, nowMs: number) => {
    if (entry.ttlSecondsRemaining === null || entry.ttlSecondsRemaining === undefined) return null;
    if (entry.ttlFetchedAt === null || entry.ttlFetchedAt === undefined) return null;
    const elapsed = Math.max(0, Math.floor((nowMs - entry.ttlFetchedAt) / 1000));
    return Math.max(0, entry.ttlSecondsRemaining - elapsed);
  };
  const secondsRemaining = (entry: BillHistoryEntry, nowMs: number) => secondsRemainingAt(entry, nowMs);
  const formatTimeRemainingClock = (entry: BillHistoryEntry, nowMs: number) => {
    const remaining = secondsRemaining(entry, nowMs);
    if (remaining === null) return '--:--:--';
    const days = Math.floor(remaining / 86400);
    const hours = Math.floor((remaining % 86400) / 3600);
    const minutes = Math.floor((remaining % 3600) / 60);
    const seconds = remaining % 60;
    if (days > 0) {
      return `${days}d ${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
    }
    const totalHours = Math.floor(remaining / 3600);
    return `${String(totalHours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
  };
  const expiryToneClass = (entry: BillHistoryEntry, nowMs: number) => {
    const remaining = secondsRemaining(entry, nowMs);
    if (remaining === null) return 'bill-history-expiry-unknown';
    if (remaining <= 5 * 60) return 'bill-history-expiry-critical';
    if (remaining <= 60 * 60) return 'bill-history-expiry-soon';
    return 'bill-history-expiry-stable';
  };
  const clearBillHistoryRefreshTimer = () => {
    if (billHistoryRefreshTimer) {
      clearTimeout(billHistoryRefreshTimer);
      billHistoryRefreshTimer = null;
    }
  };
  const computeNextBillHistoryRefreshMs = (entries: BillHistoryEntry[]) => {
    if (!entries || entries.length === 0) return BILL_HISTORY_REFRESH_SLOW_MS;
    const nowMs = Date.now();
    const remainingSeconds = entries
      .map((entry) => secondsRemainingAt(entry, nowMs))
      .filter((value): value is number => value !== null);
    if (remainingSeconds.length === 0) return BILL_HISTORY_REFRESH_DEFAULT_MS;
    const soonest = Math.min(...remainingSeconds);
    if (soonest <= 45) return BILL_HISTORY_REFRESH_FAST_MS;
    if (soonest <= 5 * 60) return 15000;
    if (soonest <= 30 * 60) return BILL_HISTORY_REFRESH_DEFAULT_MS;
    if (soonest <= 2 * 60 * 60) return 60000;
    if (soonest <= 12 * 60 * 60) return BILL_HISTORY_REFRESH_SLOW_MS;
    return BILL_HISTORY_REFRESH_MAX_MS;
  };
  const scheduleBillHistoryRefresh = (entries: BillHistoryEntry[] = billHistory) => {
    clearBillHistoryRefreshTimer();
    const delayMs = computeNextBillHistoryRefreshMs(entries);
    billHistoryRefreshTimer = setTimeout(() => {
      void refreshBillHistory();
    }, delayMs);
  };
  const restartBillHistoryClock = () => {
    if (billHistoryClockTimer) clearInterval(billHistoryClockTimer);
    billHistoryClockTimer = setInterval(() => {
      billHistoryNowMs = Date.now();
    }, 1000);
  };
  const blurActiveElement = () => {
    const active = document.activeElement;
    if (active instanceof HTMLElement) {
      active.blur();
    }
  };

  const setDocumentModalLock = (locked: boolean) => {
    if (typeof document === 'undefined') return;
    document.documentElement.classList.toggle('modal-open', locked);
    document.body.classList.toggle('modal-open', locked);
  };

  const syncBillHistoryFromCookie = () => {
    billHistory = loadBillHistory();
  };

  const syncContactsCount = () => {
    contactsCount = loadContacts().length;
  };

  const openContactsManager = () => {
    showContactsModal = true;
  };

  const refreshBillHistory = async () => {
    if (billHistoryRefreshInFlight) return;
    billHistoryRefreshInFlight = true;
    billHistoryChecking = true;
    try {
      const current = loadBillHistory();
      if (current.length === 0) {
        billHistory = [];
        return;
      }
      const checks = await Promise.all(
        current.map(async (entry) => {
          try {
            const res = await fetch(`${apiBase}/room-status?room_code=${encodeURIComponent(entry.roomCode)}`);
            if (res.status === 404) {
              return null;
            }
            if (!res.ok) {
              return entry;
            }
            const payload = await res.json();
            const nowMs = Date.now();
            const ttlRaw = Number(payload?.expires_in_seconds);
            let ttlSecondsRemaining = Number.isFinite(ttlRaw) && ttlRaw >= 0 ? Math.floor(ttlRaw) : null;
            const previousRemaining = secondsRemainingAt(entry, nowMs);
            // Prevent visible countdown jumps caused by polling jitter/rounding.
            if (
              ttlSecondsRemaining !== null &&
              previousRemaining !== null &&
              ttlSecondsRemaining > previousRemaining &&
              ttlSecondsRemaining-previousRemaining <= 5
            ) {
              ttlSecondsRemaining = previousRemaining;
            }
            const totalRaw = Number(payload?.total_cents);
            const totalCents =
              Number.isFinite(totalRaw) && totalRaw >= 0
                ? Math.round(totalRaw)
                : entry.totalCents ?? null;
            const nextCurrency = `${payload?.currency || entry.currency || 'USD'}`.toUpperCase();
            const nextTargetCurrency = `${payload?.target_currency || entry.targetCurrency || nextCurrency || 'USD'}`.toUpperCase();
            return {
              ...entry,
              billName: (payload?.name || entry.billName || '').trim(),
              currency: nextCurrency,
              targetCurrency: nextTargetCurrency,
              totalCents,
              updatedAt: nowMs,
              ttlSecondsRemaining,
              ttlFetchedAt: ttlSecondsRemaining !== null ? nowMs : null,
              convertedTotalCents: null,
              convertedShareCents: null
            } as BillHistoryEntry;
          } catch {
            return entry;
          }
        })
      );
      const nextBase = checks.filter((entry): entry is BillHistoryEntry => !!entry);

      const pairKeys = Array.from(
        new Set(
          nextBase
            .filter(
              (entry) =>
                !!entry.currency &&
                !!entry.targetCurrency &&
                entry.targetCurrency !== entry.currency &&
                ((entry.totalCents ?? null) !== null || (entry.shareCents ?? null) !== null)
            )
            .map((entry) => `${entry.currency}->${entry.targetCurrency}`)
        )
      );
      const fxRates = new Map<string, number>();
      await Promise.all(
        pairKeys.map(async (pair) => {
          const [base, target] = pair.split('->');
          if (!base || !target) return;
          try {
            const res = await fetch(
              `${apiBase}/fx?base=${encodeURIComponent(base)}&target=${encodeURIComponent(target)}`
            );
            if (!res.ok) return;
            const payload = await res.json();
            const rate = Number(payload?.rate);
            if (Number.isFinite(rate) && rate > 0) {
              fxRates.set(pair, rate);
            }
          } catch {
            // keep base currency values only when conversion fetch fails
          }
        })
      );

      const next = nextBase.map((entry) => {
        const base = `${entry.currency || 'USD'}`.toUpperCase();
        const target = `${entry.targetCurrency || base}`.toUpperCase();
        if (!base || !target || base === target) {
          return {
            ...entry,
            currency: base || 'USD',
            targetCurrency: target || base || 'USD',
            convertedTotalCents: null,
            convertedShareCents: null
          };
        }
        const pair = `${base}->${target}`;
        const rate = fxRates.get(pair);
        if (!rate) {
          return {
            ...entry,
            currency: base,
            targetCurrency: target,
            convertedTotalCents: null,
            convertedShareCents: null
          };
        }
        const sourceFactor = factorFor(base);
        const targetFactor = factorFor(target);
        const convertMinor = (amountMinor: number | null | undefined) => {
          if (amountMinor === null || amountMinor === undefined) return null;
          const major = amountMinor / sourceFactor;
          return Math.max(0, Math.round(major * rate * targetFactor));
        };
        return {
          ...entry,
          currency: base,
          targetCurrency: target,
          convertedTotalCents: convertMinor(entry.totalCents),
          convertedShareCents: convertMinor(entry.shareCents)
        };
      });

      saveBillHistory(next);
      syncBillHistoryFromCookie();
    } finally {
      billHistoryLoadedOnce = true;
      billHistoryChecking = false;
      billHistoryRefreshInFlight = false;
      scheduleBillHistoryRefresh(loadBillHistory());
    }
  };

  const prefillCreateFromCookies = () => {
    const prefs = loadIdentityPrefs();
    billName = '';
    createName = (prefs.name || '').trim();
    createVenmoUsername = (prefs.venmoUsername || '').trim();
  };

  const prefillJoinFromCookies = () => {
    const prefs = loadIdentityPrefs();
    joinCode = '';
    joinName = (prefs.name || '').trim();
    joinVenmoUsername = (prefs.venmoUsername || '').trim();
  };

  onMount(() => {
    prefillCreateFromCookies();
    prefillJoinFromCookies();
    syncBillHistoryFromCookie();
    syncContactsCount();
    refreshBillHistory();
    restartBillHistoryClock();
    const handleVisibilityChange = () => {
      if (document.visibilityState !== 'visible') return;
      billHistoryNowMs = Date.now();
      restartBillHistoryClock();
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => {
      clearBillHistoryRefreshTimer();
      if (billHistoryClockTimer) clearInterval(billHistoryClockTimer);
      billHistoryClockTimer = null;
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      setDocumentModalLock(false);
    };
  });

  $: landingModalOpen = showCreate || showJoin || showContactsModal;
  $: setDocumentModalLock(landingModalOpen);

  const submitCreate = async () => {
    if (createBusy || !createName.trim() || !billName.trim()) return;
    createBusy = true;
    createError = '';
    try {
      const res = await fetch(`${apiBase}/create-room`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: createName.trim(),
          bill_name: billName.trim(),
          venmo_username: normalizeVenmoUsername(createVenmoUsername)
        })
      });
      if (!res.ok) {
        createError = 'Could not create bill. Please try again.';
        return;
      }
      const data = await res.json();
      const identity = {
        userId: data.user_id,
        name: createName.trim(),
        initials: initialsFromName(createName),
        colorSeed: `${data.color_seed || data.user_id.slice(0, 6)}`,
        venmoUsername: normalizeVenmoUsername(createVenmoUsername)
      };
      saveIdentityPrefs(identity.name, identity.venmoUsername);
      localStorage.setItem(`room:${data.room_code}:identity`, JSON.stringify(identity));
      upsertBillHistoryEntry({
        roomCode: normalizeRoomCode(data.room_code),
        billName: billName.trim(),
        joinedName: identity.name,
        joinedVenmoUsername: identity.venmoUsername,
        joinedColorSeed: identity.colorSeed,
        shareCents: null,
        currency: `${data.currency || 'USD'}`.toUpperCase(),
        targetCurrency: `${data.target_currency || data.currency || 'USD'}`.toUpperCase(),
        totalCents: null,
        convertedShareCents: null,
        convertedTotalCents: null,
        updatedAt: Date.now(),
        ttlSecondsRemaining: null,
        ttlFetchedAt: null
      });
      blurActiveElement();
      goto(`/room/${data.room_code}`);
    } catch {
      createError = 'Network error. Please try again.';
    } finally {
      createBusy = false;
    }
  };

  const submitJoin = async () => {
    if (joinBusy || !joinCode.trim() || !joinName.trim()) return;
    joinBusy = true;
    joinError = '';
    try {
      const res = await fetch(`${apiBase}/join-room`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          room_code: joinCode.toUpperCase().trim(),
          name: joinName.trim(),
          venmo_username: normalizeVenmoUsername(joinVenmoUsername)
        })
      });
      if (!res.ok) {
        joinError = 'Invalid bill code or join failed.';
        return;
      }
      const data = await res.json();
      const identity = {
        userId: data.user_id,
        name: joinName.trim(),
        initials: initialsFromName(joinName),
        colorSeed: `${data.color_seed || data.user_id.slice(0, 6)}`,
        venmoUsername: normalizeVenmoUsername(joinVenmoUsername)
      };
      saveIdentityPrefs(identity.name, identity.venmoUsername);
      localStorage.setItem(`room:${data.room_code}:identity`, JSON.stringify(identity));
      upsertBillHistoryEntry({
        roomCode: normalizeRoomCode(data.room_code),
        billName: '',
        joinedName: identity.name,
        joinedVenmoUsername: identity.venmoUsername,
        joinedColorSeed: identity.colorSeed,
        shareCents: null,
        currency: `${data.currency || 'USD'}`.toUpperCase(),
        targetCurrency: `${data.target_currency || data.currency || 'USD'}`.toUpperCase(),
        totalCents: null,
        convertedShareCents: null,
        convertedTotalCents: null,
        updatedAt: Date.now(),
        ttlSecondsRemaining: null,
        ttlFetchedAt: null
      });
      blurActiveElement();
      goto(`/room/${data.room_code}`);
    } catch {
      joinError = 'Network error. Please try again.';
    } finally {
      joinBusy = false;
    }
  };
</script>

<div class="app-screen">
  <div class="mx-auto w-full max-w-md px-5 pt-4 pb-10 space-y-2.5">
    <header class="accent-gradient glass-card landing-hero p-5 motion-rise">
      <div class="mt-1 flex justify-center">
        <img
          src="/brand/divvi-banner.svg?v=3"
          alt="Divvi"
          class="h-12 w-auto max-w-full"
        />
      </div>
      <p class="mt-2 text-sm text-white/80">
        Split receipts in real time with your group. Built for mobile, fast to use at the table.
      </p>
    </header>

    <main class="space-y-2">
      <button
        class="glass-card touch-card landing-action-card motion-rise motion-rise-delay-1 w-full p-4 text-left"
        on:click={() => {
          createError = '';
          // Landing modal fields should only be populated from cookies.
          prefillCreateFromCookies();
          showCreate = true;
        }}
      >
        <div class="flex items-start gap-3">
          <div>
            <p class="text-base font-semibold text-white">Create new bill</p>
            <p class="mt-1 text-sm text-surface-300">Start a room and share the code with friends.</p>
          </div>
        </div>
        <div class="mt-4">
          <span class="btn btn-primary w-full">Create Bill</span>
        </div>
      </button>

      <button
        class="glass-card touch-card landing-action-card motion-rise motion-rise-delay-2 w-full p-4 text-left"
        on:click={() => {
          joinError = '';
          // Landing modal fields should only be populated from cookies.
          prefillJoinFromCookies();
          showJoin = true;
        }}
      >
        <div class="flex items-start gap-3">
          <div>
            <p class="text-base font-semibold text-white">Join existing bill</p>
            <p class="mt-1 text-sm text-surface-300">Enter a code and start assigning items.</p>
          </div>
        </div>
        <div class="mt-4">
          <span class="btn btn-outline w-full">Join Bill</span>
        </div>
      </button>

      <button
        class="glass-card touch-card landing-action-card motion-rise motion-rise-delay-3 w-full p-4 text-left"
        on:click={openContactsManager}
      >
        <div class="flex items-start justify-between gap-3">
          <div>
            <p class="text-base font-semibold text-white">Contacts</p>
            <p class="mt-1 text-sm text-surface-300">Manage saved contacts from people you've split bills with.</p>
          </div>
          <span class="bill-history-chip whitespace-nowrap">{contactsCount} saved</span>
        </div>
        <div class="mt-4">
          <span class="btn btn-outline w-full">Manage Contacts</span>
        </div>
      </button>
    </main>

    <section class="glass-card bill-history-shell ui-card-lift p-4 motion-rise motion-rise-delay-4 space-y-2.5" data-clock={billHistoryNowMs}>
      <div class="bill-history-sheen" aria-hidden="true"></div>
      <div class="relative z-[1] flex items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <span class="bill-history-title-mark" aria-hidden="true"></span>
          <p class="text-base font-semibold text-white">Your bills</p>
        </div>
        <span
          class="bill-history-count-pill"
          aria-label={`${billHistory.length} saved ${billHistory.length === 1 ? 'bill' : 'bills'}`}
          title={`${billHistory.length} saved ${billHistory.length === 1 ? 'bill' : 'bills'}`}
        >
          {billHistory.length}
        </span>
      </div>

      {#if !billHistoryLoadedOnce && billHistoryChecking}
        <p class="relative z-[1] text-sm text-surface-300">Checking recent rooms...</p>
      {:else if billHistory.length === 0}
        <div class="bill-history-empty relative z-[1]">
          <p class="text-sm text-surface-200">No recent bills yet on this device.</p>
        </div>
      {:else}
        <div class="bill-history-list relative z-[1]">
          {#each billHistory as entry}
            <button
              class="bill-history-card"
              on:click={() => goto(`/room/${entry.roomCode}`)}
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold text-white">{entry.billName || `Room ${entry.roomCode}`}</p>
                  <div class="mt-1 flex flex-wrap items-center gap-1.5">
                    <span class="bill-history-chip">#{entry.roomCode}</span>
                    <span class={`bill-history-chip bill-history-expiry ${expiryToneClass(entry, billHistoryNowMs)}`}>
                      <span class="font-mono tabular-nums">{formatTimeRemainingClock(entry, billHistoryNowMs)}</span>
                    </span>
                  </div>
                </div>
                <svg class="h-4 w-4 shrink-0 text-surface-400" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M7 4l6 6-6 6" />
                </svg>
              </div>
              <div class="mt-2 grid grid-cols-2 gap-2">
                <div class="bill-history-stat">
                  <p class="bill-history-stat-label">Total</p>
                  <p class="bill-history-stat-value">{formatTotal(entry)}</p>
                  {#if hasConvertedTotals(entry)}
                    <p class="bill-history-stat-conversion">→ {formatConvertedTotal(entry)}</p>
                  {/if}
                </div>
                <div class="bill-history-stat bill-history-stat-share">
                  <p class="bill-history-stat-label">Your share</p>
                  <p class="bill-history-stat-value text-cyan-100">{formatShare(entry)}</p>
                  {#if hasConvertedShare(entry)}
                    <p class="bill-history-stat-conversion text-cyan-200">→ {formatConvertedShare(entry)}</p>
                  {/if}
                </div>
              </div>
              <div class="mt-2.5 flex items-center gap-2 text-xs text-surface-300">
                <span class="bill-history-joined-mark" style={`background:${joinedAvatarColor(entry)};`}>
                  {initialsFromName(entry.joinedName || 'Guest')}
                </span>
                <span class="truncate">
                  {entry.joinedName || 'Guest'}{entry.joinedVenmoUsername ? ` (@${entry.joinedVenmoUsername})` : ''}
                </span>
              </div>
            </button>
          {/each}
        </div>
      {/if}
    </section>
  </div>

  <ContactsModal
    bind:open={showContactsModal}
    on:contactschange={() => syncContactsCount()}
    on:close={() => syncContactsCount()}
  />

  {#if showCreate}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h2 class="text-lg font-semibold text-white modal-title">Create bill</h2>
        <p class="text-sm text-surface-300 modal-subtitle">Your name and bill name are shown to everyone in the room.</p>
        <label class="block space-y-1.5 modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Your name</span>
          <input class="input" bind:value={createName} placeholder="" autocomplete="off" />
        </label>
        <label class="block space-y-1.5 modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Bill name</span>
          <input class="input" bind:value={billName} placeholder="" autocomplete="off" />
        </label>
        <label class="block space-y-1.5 modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Venmo username (optional)</span>
          <input class="input" bind:value={createVenmoUsername} placeholder="" autocomplete="off" />
        </label>
        {#if createError}
          <p class="rounded-lg border border-red-400/35 bg-red-500/15 px-3 py-2 text-sm text-red-100 modal-error">{createError}</p>
        {/if}
        <div class="flex gap-2 modal-actions">
          <button class="btn btn-outline w-full" on:click={() => (showCreate = false)} disabled={createBusy}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={submitCreate} disabled={createBusy || !createName || !billName}>
            {createBusy ? 'Creating...' : 'Start'}
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showJoin}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h2 class="text-lg font-semibold text-white modal-title">Join bill</h2>
        <p class="text-sm text-surface-300 modal-subtitle">Room codes are case-insensitive.</p>
        <label class="block space-y-1.5 modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Bill code</span>
          <input class="input uppercase" bind:value={joinCode} placeholder="" autocomplete="off" />
        </label>
        <label class="block space-y-1.5 modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Your name</span>
          <input class="input" bind:value={joinName} placeholder="" autocomplete="off" />
        </label>
        <label class="block space-y-1.5 modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Venmo username (optional)</span>
          <input class="input" bind:value={joinVenmoUsername} placeholder="" autocomplete="off" />
        </label>
        {#if joinError}
          <p class="rounded-lg border border-red-400/35 bg-red-500/15 px-3 py-2 text-sm text-red-100 modal-error">{joinError}</p>
        {/if}
        <div class="flex gap-2 modal-actions">
          <button class="btn btn-outline w-full" on:click={() => (showJoin = false)} disabled={joinBusy}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={submitJoin} disabled={joinBusy || !joinCode || !joinName}>
            {joinBusy ? 'Joining...' : 'Join'}
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
