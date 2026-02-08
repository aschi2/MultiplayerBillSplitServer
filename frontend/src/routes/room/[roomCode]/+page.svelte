<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import Avatar from '$lib/components/Avatar.svelte';
  import { formatCurrency, initialsFromName } from '$lib/utils';
  import type { RoomDoc, ReceiptParseResult, Item, Participant } from '$lib/types';
  import { getApiBase, getWsBase } from '$lib/api';
  import { COMMON_CURRENCIES, DEFAULT_CURRENCY, EXPONENTS, SYMBOLS, FLAGS } from '$lib/currency';

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
  let receiptWarnings: string[] = [];
  let receiptError: string | null = null;
  let receiptUploading = false;
  let receiptIsAddon = false;
  let baselineTaxCents = 0;
  let baselineSubtotalCents = 0;
  let parsedTaxInput = '';
  let receiptCurrencySelection: string = DEFAULT_CURRENCY;
  let parsedTaxCents = 0;
  let receiptSubtotalCents = 0;
  let projectedTaxCents = 0;
  let receiptTaxPercent = 0;
  let projectedTaxPercent = 0;
  let showItemModal = false;
  let itemModalMode: 'new' | 'edit' = 'new';
  let itemModalId: string | null = null;
  let itemForm = {
    name: '',
    quantity: '1',
    unitPrice: '',
    linePrice: '',
    discountCents: '',
    discountPercent: ''
  };
  let showTaxTipModal = false;
  let taxInput = '';
  let tipInput = '';
  let showSummary = false;
  let summaryData: {
    gross: number;
    discount: number;
    net: number;
    tax: number;
    tip: number;
    total: number;
    converted?: {
      gross: number;
      discount: number;
      net: number;
      tax: number;
      tip: number;
      total: number;
      perPerson: { id: string; total: number }[];
      rate: number;
      asOf?: string | null;
      currency: string;
    };
    perPerson: {
      id: string;
      name: string;
      color: string;
      items: { name: string; share_cents: number }[];
      itemsTotal: number;
      taxShare: number;
      tipShare: number;
      total: number;
    }[];
  } | null = null;
  let showNameModal = false;
  let showRoomNameModal = false;
  let roomNameInput = '';
  let nameInput = '';
  let showAddPersonModal = false;
  let addPersonName = '';
  let showJoinPrompt = false;
  let joinNameInput = '';
  let joinError: string | null = null;
  let preTaxSubtotalCents = 0;
  let taxCentsPreview = 0;
  let tipCentsPreview = 0;
  let taxPercent = 0;
  let tipPercent = 0;
  let wsStatus: 'connecting' | 'connected' | 'reconnecting' | 'disconnected' = 'connecting';
  let roomCurrency: string = DEFAULT_CURRENCY;
  let targetCurrency: string = DEFAULT_CURRENCY;
  let detectedCurrency: string | null = null;
  let fxRate: number | null = null;
  let fxAsOf: string | null = null;
let editableItems: {
    name: string;
    quantity: string;
    unitPrice: string;
    linePrice: string;
    discountCents: string;
    discountPercent: string;
  }[] = [];
  let items: Item[] = [];
  let participants: Participant[] = [];
  let initialsCounts: Record<string, number> = {};
  let initialsBadges: Record<string, string> = {};
  $: participantAssignments = (() => {
    const map: Record<string, boolean> = {};
    items.forEach((item) => {
      Object.entries(item.assigned || {}).forEach(([uid, on]) => {
        if (on) map[uid] = true;
      });
    });
    return map;
  })();
  let participantAssignments: Record<string, boolean> = {};
  let shareLink = '';
  let qrUrl = '';
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
const RECONNECT_MIN = 500;
const RECONNECT_MAX = 5000;
const PING_INTERVAL = 5000;
const PONG_TIMEOUT = 10000;
let reconnectDelay = RECONNECT_MIN;
let resyncInterval: ReturnType<typeof setInterval> | null = null;
let heartbeatInterval: ReturnType<typeof setInterval> | null = null;
let pongTimer: ReturnType<typeof setTimeout> | null = null;
let lastMessageAt = Date.now();
let currentSeq = 0;
let isConnecting = false;
let forceClose = false;
let wsGeneration = 0;
const pendingOps: any[] = [];

  const safeClose = () => {
    try {
      ws?.close();
    } catch {
      // ignore
    }
  };

  const beginReconnect = () => {
    if (wsStatus === 'reconnecting') return;
    wsStatus = 'reconnecting';
    reconnectDelay = RECONNECT_MIN;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(connectWS, reconnectDelay);
  };

  const sendOp = (op: any) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'op', op }));
    } else {
      pendingOps.push(op);
      if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
        connectWS();
      }
    }
  };

  const apiBase = getApiBase();
  const wsBase = getWsBase();

  const convertToJpeg = async (file: File) => {
    if (file.type !== 'image/heic' && file.type !== 'image/heif') {
      return file;
    }
    const url = URL.createObjectURL(file);
    try {
      const image = new Image();
      await new Promise<void>((resolve, reject) => {
        image.onload = () => resolve();
        image.onerror = () => reject(new Error('Failed to load image'));
        image.src = url;
      });
      const canvas = document.createElement('canvas');
      canvas.width = image.naturalWidth || image.width;
      canvas.height = image.naturalHeight || image.height;
      const ctx = canvas.getContext('2d');
      if (!ctx) return file;
      ctx.drawImage(image, 0, 0);
      const blob = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob(resolve, 'image/jpeg', 0.9)
      );
      if (!blob) return file;
      const name = file.name.replace(/\.\w+$/, '') || 'receipt';
      return new File([blob], `${name}.jpg`, { type: 'image/jpeg' });
    } finally {
      URL.revokeObjectURL(url);
    }
  };

  const decodePayload = (payload: any) => (typeof payload === 'string' ? JSON.parse(payload) : payload);

  const colorHex = (seed?: string) => {
    if (!seed) return '#94a3b8';
    return seed.startsWith('#') ? seed : `#${seed}`;
  };

  const hexSeed = (input: string) => {
    // FNV-1a for stable, well-spread 6-hex seeds
    let hash = 0x811c9dc5;
    for (let i = 0; i < input.length; i++) {
      hash ^= input.charCodeAt(i);
      hash = Math.imul(hash, 0x01000193);
    }
    return (hash >>> 0).toString(16).padStart(6, '0').slice(0, 6);
  };

  const exponentFor = (code: string) => EXPONENTS[code] ?? 2;
  const factorFor = (code: string) => Math.pow(10, exponentFor(code));
  const symbolFor = (code: string) => SYMBOLS[code] ?? '';
  const flagFor = (code: string) => FLAGS[code] ?? '🏳️';
  const formatAmount = (amount: number, code = roomCurrency) =>
    formatCurrency(amount, code, symbolFor(code), exponentFor(code));

  const changeCurrency = (code: string) => {
    if (!code) return;
    const upper = code.toUpperCase();
    roomCurrency = upper;
    room = room ? { ...room, currency: upper } : room;
    const payload = { currency: upper };
    sendOp({ kind: 'set_room_name', payload });
    applyLocalOp({ kind: 'set_room_name', payload, timestamp: Date.now() });
  };

  const changeTargetCurrency = (code: string) => {
    if (!code) return;
    const upper = code.toUpperCase();
    targetCurrency = upper;
    room = room ? { ...room, target_currency: upper } : room;
    const payload = { target_currency: upper };
    sendOp({ kind: 'set_room_name', payload });
    applyLocalOp({ kind: 'set_room_name', payload, timestamp: Date.now() });
    fxRate = null;
    fxAsOf = null;
  };

  const ensureFxRate = async () => {
    if (targetCurrency === roomCurrency) {
      fxRate = 1;
      fxAsOf = null;
      return 1;
    }
    if (fxRate) return fxRate;
    const res = await fetch(`${apiBase}/fx?base=${roomCurrency}&target=${targetCurrency}`);
    if (!res.ok) throw new Error('Rate unavailable');
    const payload = await res.json();
    fxRate = Number(payload.rate);
    if (payload.as_of) {
      const ts =
        typeof payload.as_of === 'number'
          ? payload.as_of * 1000
          : Number.isFinite(Date.parse(payload.as_of))
            ? Date.parse(payload.as_of)
            : NaN;
      fxAsOf = Number.isFinite(ts) ? new Date(ts).toISOString() : null;
    } else {
      fxAsOf = null;
    }
    return fxRate;
  };

  const toCentsInput = (value: string, code = roomCurrency) => {
    const num = Number.parseFloat(value || '');
    if (!Number.isFinite(num)) return 0;
    return Math.max(0, Math.round(num * factorFor(code)));
  };

  const parseByCurrency = (value: string, code = roomCurrency) => {
    const exp = exponentFor(code);
    const factor = Math.pow(10, exp);
    const num = Number.parseFloat(value || '');
    if (!Number.isFinite(num)) return 0;
    return Math.max(0, Math.round(num * factor));
  };

  const subtotalFromEditable = (list: typeof editableItems) => {
    if (!list?.length) return 0;
    return list.reduce((sum, item) => {
      const qty = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
      const code = receiptCurrencySelection || roomCurrency;
      const unit = toCentsInput(item.unitPrice, code);
      const line = toCentsInput(item.linePrice, code);
      const gross = line || unit * qty;
      const discountPct = Number.parseFloat(item.discountPercent || '0') || 0;
      const discountCentsPerUnit =
        toCentsInput(item.discountCents, code) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
      const net = Math.max(0, gross - discountCentsPerUnit * qty);
      return sum + net;
    }, 0);
  };

  const discountedUnitAndNetFromEditable = (item: typeof editableItems[number]) => {
    const code = receiptCurrencySelection || roomCurrency;
    const qty = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
    const unit = toCentsInput(item.unitPrice, code);
    const line = toCentsInput(item.linePrice, code);
    const gross = line || unit * qty;
    const discountPct = Number.parseFloat(item.discountPercent || '0') || 0;
    const discountCentsPerUnit =
      toCentsInput(item.discountCents, code) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
    const net = Math.max(0, gross - discountCentsPerUnit * qty);
    const netUnit = Math.max(0, unit - discountCentsPerUnit);
    return { netUnit, netTotal: net };
  };

  const discountedUnitAndNetFromItemForm = () => {
    const qty = Math.max(1, Number.parseInt(itemForm.quantity || '1', 10) || 1);
    const unit = toCentsInput(itemForm.unitPrice, roomCurrency);
    const line = toCentsInput(itemForm.linePrice, roomCurrency);
    const gross = line || unit * qty;
    const discountPct = Number.parseFloat(itemForm.discountPercent || '0') || 0;
    const discountCentsPerUnit =
      toCentsInput(itemForm.discountCents, roomCurrency) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
    const netTotal = Math.max(0, gross - discountCentsPerUnit * qty);
    const netUnit = qty > 0 ? Math.max(0, Math.round(netTotal / qty)) : 0;
    return { netUnit, netTotal };
  };

  const removeEditableItem = (index: number) => {
    if (!receiptResult) return;
    const list = [...editableItems];
    const removed = list[index];
    if (!removed) return;
    const prevSubtotal = receiptSubtotalCents || subtotalFromEditable(list);
    list.splice(index, 1);
    editableItems = list;
    const removedNet = discountedUnitAndNetFromEditable(removed).netTotal;
    const remainingSubtotal = subtotalFromEditable(list);
    if (prevSubtotal > 0 && parsedTaxInput !== null && parsedTaxInput !== undefined) {
      const currentTax = parsedTaxCents;
      const adjustment = Math.round((removedNet / prevSubtotal) * currentTax);
      const newTax = Math.max(0, currentTax - adjustment);
      const code = receiptCurrencySelection || roomCurrency;
      const exp = exponentFor(code);
      const factor = factorFor(code);
      parsedTaxInput = (newTax / factor).toFixed(exp);
    }
    receiptSubtotalCents = remainingSubtotal;
  };

  const fallbackSeed = (id: string, name?: string) => {
    const base = id || name || 'seed';
    return hexSeed(base);
  };

  const normalizeParticipant = (p: any): Participant => {
    const id = p.id || p.ID || '';
    const name = p.name || p.Name || '';
    const colorSeed = p.colorSeed || p.ColorSeed || p.color_seed || p.Color_seed || fallbackSeed(id, name);
    return {
      id,
      name,
      initials: p.initials || p.Initials || initialsFromName(name),
      colorSeed,
      present: p.present ?? p.Present ?? false
    };
  };

  const applyLocalOp = (op: any) => {
    if (!room) return;
    const payload = decodePayload(op.payload);
    const next: RoomDoc = {
      ...room,
      items: { ...room.items },
      participants: { ...room.participants }
    };
    switch (op.kind) {
      case 'set_item': {
        const item = payload.item as Item;
        if (!item.assigned) item.assigned = {};
        next.items[item.id] = item;
        break;
      }
      case 'remove_item': {
        const id = payload.id || payload.item_id;
        if (id && next.items[id]) {
          delete next.items[id];
        }
        break;
      }
      case 'assign_item': {
        const { item_id, user_id, on } = payload;
        const it = next.items[item_id];
        if (it) {
          it.assigned = it.assigned || {};
          it.assigned[user_id] = on;
        }
        break;
      }
      case 'set_tax_tip': {
        if (typeof payload.tax_cents === 'number') next.tax_cents = payload.tax_cents;
        if (typeof payload.tip_cents === 'number') next.tip_cents = payload.tip_cents;
        break;
      }
      case 'remove_participant': {
        const id = payload.id || payload.participant_id;
        if (id && next.participants[id]) {
          delete next.participants[id];
        }
        break;
      }
      case 'set_participant': {
        const p = normalizeParticipant(payload.participant);
        if (p?.id) {
          next.participants[p.id] = p;
          if (p.id === identity.userId) {
            identity = {
              ...identity,
              name: p.name || identity.name,
              initials: p.initials || initialsFromName(p.name || identity.name),
              colorSeed: p.colorSeed || identity.colorSeed
            };
            localStorage.setItem(`room:${roomCode}:identity`, JSON.stringify(identity));
          }
        }
        break;
      }
      case 'set_room_name': {
        if (payload?.name) {
          next.name = payload.name;
        }
        if (payload?.currency) {
          roomCurrency = String(payload.currency).toUpperCase();
          next.currency = roomCurrency;
        }
        if (payload?.target_currency) {
          targetCurrency = String(payload.target_currency).toUpperCase();
          next.target_currency = targetCurrency;
        }
        break;
      }
    }
    room = next;
  };

  const requestSnapshot = () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'resync', last_seq: 0 }));
  };

  const scheduleReconnect = () => {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(() => {
      connectWS();
      reconnectDelay = Math.min(Math.round(reconnectDelay * 1.5), RECONNECT_MAX);
    }, reconnectDelay);
    wsStatus = 'reconnecting';
  };

  const connectWS = () => {
    if (isConnecting) return;
    isConnecting = true;
    const gen = ++wsGeneration;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (resyncInterval) {
      clearInterval(resyncInterval);
      resyncInterval = null;
    }
    if (heartbeatInterval) {
      clearInterval(heartbeatInterval);
      heartbeatInterval = null;
    }
    safeClose();
    ws = null;
    forceClose = false;

    wsStatus = 'connecting';
    ws = new WebSocket(`${wsBase}/${roomCode}`);
    ws.onopen = () => {
      if (gen !== wsGeneration) return;
      isConnecting = false;
      reconnectDelay = RECONNECT_MIN;
      wsStatus = 'connected';
      lastMessageAt = Date.now();
      if (pongTimer) {
        clearTimeout(pongTimer);
        pongTimer = null;
      }
      requestSnapshot();
      resyncInterval = setInterval(requestSnapshot, 60000);
      heartbeatInterval = setInterval(() => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ping' }));
          if (pongTimer) clearTimeout(pongTimer);
          pongTimer = setTimeout(() => {
            if (gen === wsGeneration) {
              safeClose();
            }
          }, PONG_TIMEOUT);
        }
      }, PING_INTERVAL);
      while (pendingOps.length && ws && ws.readyState === WebSocket.OPEN) {
        const next = pendingOps.shift();
        ws.send(JSON.stringify({ type: 'op', op: next }));
      }
      // ensure presence marked online on fresh connection
      sendPresence(true);
    };
    ws.onmessage = (event) => {
      if (gen !== wsGeneration) return;
      const message = JSON.parse(event.data);
      lastMessageAt = Date.now();
      if (message.type === 'pong') {
        if (pongTimer) {
          clearTimeout(pongTimer);
          pongTimer = null;
        }
        return;
      }
      if (message.type === 'snapshot') {
      if (typeof message.seq === 'number' && message.seq < currentSeq) {
        return;
      }
      currentSeq = typeof message.seq === 'number' ? message.seq : currentSeq;
      room = {
        ...message.doc,
        participants: Object.fromEntries(
          Object.entries(message.doc.participants || {}).map(([id, p]) => [
            id,
            normalizeParticipant(p)
          ])
        )
      };
      roomCurrency = (room.currency || DEFAULT_CURRENCY).toUpperCase();
      targetCurrency = (room.target_currency || roomCurrency || DEFAULT_CURRENCY).toUpperCase();
      const self = identity.userId && room.participants?.[identity.userId];
      if (self) {
        identity = {
          ...identity,
          name: self.name || identity.name,
          initials: self.initials || initialsFromName(self.name || identity.name),
          colorSeed: self.colorSeed || identity.colorSeed
        };
        localStorage.setItem(`room:${roomCode}:identity`, JSON.stringify(identity));
      }
      // ensure our connection is tracked and marked present after snapshot is in hand
      if (identity.userId && identity.name) {
        sendParticipantUpdate(identity.userId, identity.name, true);
      }
      }
      if (message.type === 'op') {
        if (typeof message.seq === 'number' && message.seq < currentSeq) {
          return;
        }
        if (typeof message.seq === 'number') currentSeq = message.seq;
        applyLocalOp(message.op);
      }
    };
    ws.onclose = () => {
      if (gen !== wsGeneration) return;
      isConnecting = false;
      sendPresence(false);
      if (resyncInterval) {
        clearInterval(resyncInterval);
        resyncInterval = null;
      }
      if (!forceClose) {
        wsStatus = 'reconnecting';
        scheduleReconnect();
      } else {
        wsStatus = 'disconnected';
      }
    };
    ws.onerror = () => {
      if (gen !== wsGeneration) return;
      isConnecting = false;
      if (!forceClose) {
        wsStatus = 'reconnecting';
        scheduleReconnect();
      }
    };
    window.addEventListener('beforeunload', () => sendPresence(false));
  };

  const toggleAssign = (itemId: string, userId: string) => {
    if (!room) return;
    const item = room.items[itemId];
    const currently = item.assigned?.[userId] ?? false;
    sendOp({
      kind: 'assign_item',
      actor_id: identity.userId,
      payload: { item_id: itemId, user_id: userId, on: !currently }
    });
  };

  const computeSummary = () => {
    if (!room) return null;
    const participants = room.participants || {};
    const itemsArr = Object.values(room.items || {}) as Item[];

    const gross = itemsArr.reduce((sum, it) => sum + it.line_price_cents, 0);
    const discount = itemsArr.reduce((sum, it) => sum + it.discount_cents * (it.quantity || 1), 0);
    const net = gross - discount;
    const tax = room.tax_cents || 0;
    const tip = room.tip_cents || 0;

    // per-person accumulators (float cents before final rounding)
    const perPerson = new Map<
      string,
      {
        name: string;
        color: string;
        items: { name: string; share_cents: number }[];
        itemsTotal: number; // finalized int cents
        itemsAcc: number; // float cents before rounding
      }
    >();

    const splitProportional = (total: number, weights: Record<string, number>) => {
      const entries = Object.entries(weights).filter(([, w]) => w > 0);
      const sumW = entries.reduce((s, [, w]) => s + w, 0);
      if (total <= 0 || sumW <= 0 || !entries.length) return {} as Record<string, number>;
      const bases: Record<string, number> = {};
      const remainders: { id: string; frac: number }[] = [];
      let used = 0;
      entries.forEach(([id, w]) => {
        const exact = (total * w) / sumW;
        const base = Math.floor(exact);
        bases[id] = base;
        used += base;
        remainders.push({ id, frac: exact - base });
      });
      let rem = total - used;
      remainders.sort((a, b) => b.frac - a.frac || a.id.localeCompare(b.id));
      for (let i = 0; i < rem; i++) {
        bases[remainders[i].id] += 1;
      }
      return bases;
    };

    // accumulate exact shares in float cents
    itemsArr.forEach((it) => {
      const assignees = Object.entries(it.assigned || {}).filter(([, on]) => on).map(([uid]) => uid);
      if (assignees.length === 0) return;
      const netLine = Math.max(0, it.line_price_cents - it.discount_cents * (it.quantity || 1));
      const shareExact = netLine / assignees.length;
      assignees.forEach((uid) => {
        const p = participants[uid];
        const entry =
          perPerson.get(uid) ||
          ({
            name: p?.name || uid,
            color: colorHex(p?.colorSeed),
            items: [],
            itemsTotal: 0,
            itemsAcc: 0
          } as any);
        entry.itemsAcc += shareExact;
        perPerson.set(uid, entry);
      });
      // store per-item approximate shares for display if needed
      const rem = netLine - shareExact * assignees.length;
      assignees.forEach((uid) => {
        const entry = perPerson.get(uid);
        if (!entry) return;
        // push rounded for display only; final balancing happens after
        entry.items.push({ name: it.name, share_cents: Math.round(shareExact) });
        perPerson.set(uid, entry);
      });
    });

    // finalize item shares with largest-remainder method across people
    const floorTotals: Record<string, number> = {};
    const remainders: { id: string; frac: number }[] = [];
    perPerson.forEach((person, uid) => {
      const exact = person.itemsAcc;
      const floorVal = Math.floor(exact);
      floorTotals[uid] = floorVal;
      remainders.push({ id: uid, frac: exact - floorVal });
    });
    let used = Object.values(floorTotals).reduce((s, v) => s + v, 0);
    let remCents = net - used;
    remainders.sort((a, b) => b.frac - a.frac || a.id.localeCompare(b.id));
    for (let i = 0; i < remCents; i++) {
      const target = remainders[i % remainders.length]?.id;
      if (target) floorTotals[target] += 1;
    }
    perPerson.forEach((person, uid) => {
      person.itemsTotal = floorTotals[uid] || 0;
      perPerson.set(uid, person);
    });

    const totalItems = Array.from(perPerson.values()).reduce((s, p) => s + p.itemsTotal, 0) || 0;

    const weights: Record<string, number> = {};
    perPerson.forEach((person, uid) => {
      weights[uid] = person.itemsTotal;
    });

    const taxSplits = splitProportional(tax, weights);
    const tipSplits = splitProportional(tip, weights);

    const detailed = Array.from(perPerson.entries()).map(([uid, person]) => {
      const taxShare = taxSplits[uid] || 0;
      const tipShare = tipSplits[uid] || 0;
      const totalShare = person.itemsTotal + taxShare + tipShare;
      return { id: uid, ...person, taxShare, tipShare, total: totalShare };
    });

    const total = net + tax + tip;
    return { gross, discount, net, tax, tip, total, perPerson: detailed };
  };

  const buildSummary = async () => {
    const base = computeSummary();
    if (!base) {
      summaryData = null;
      return;
    }
    summaryData = base;
    if (targetCurrency === roomCurrency) {
      return;
    }
    try {
      const rate = await ensureFxRate();
      const srcExp = exponentFor(roomCurrency);
      const tgtExp = exponentFor(targetCurrency);
      const srcFactor = Math.pow(10, srcExp);
      const tgtFactor = Math.pow(10, tgtExp);
      const roundHalfEven = (value: number) => {
        const floor = Math.floor(value);
        const frac = value - floor;
        if (frac > 0.5 + 1e-9) return floor + 1;
        if (Math.abs(frac-0.5) <= 1e-9) return floor%2 === 0 ? floor : floor + 1;
        return floor;
      };
      const convertMinor = (amount: number) => {
        const scaled = (amount / srcFactor) * rate * tgtFactor;
        return roundHalfEven(scaled);
      };
      const converted = {
        gross: convertMinor(base.gross),
        discount: convertMinor(base.discount),
        net: convertMinor(base.net),
        tax: convertMinor(base.tax),
        tip: convertMinor(base.tip),
        total: convertMinor(base.total),
        perPerson: base.perPerson.map((p) => ({
          id: p.id,
          total: convertMinor(p.total)
        })),
        rate,
        asOf: fxAsOf,
        currency: targetCurrency
      };
      summaryData = { ...base, converted };
    } catch (err) {
      console.error('fx error', err);
      fxRate = null;
      fxAsOf = null;
    }
  };

  const submitReceipt = async (event: Event) => {
    const target = event.target as HTMLInputElement;
    if (!target.files?.[0]) return;
    receiptError = null;
    receiptUploading = true;
    baselineTaxCents = room?.tax_cents || 0;
    baselineSubtotalCents = items.reduce((sum, it) => {
      const qty = it.quantity || 1;
      const gross = Number(it.line_price_cents || 0);
      const discount = Number(it.discount_cents || 0) * qty;
      const net = Math.max(0, gross - discount);
      return sum + net;
    }, 0);
    receiptIsAddon = (room && Object.keys(room.items || {}).length > 0) || baselineSubtotalCents > 0;
    parsedTaxInput = '';
    try {
      const file = await convertToJpeg(target.files[0]);
      const form = new FormData();
      form.append('file', file);
      const res = await fetch(`${apiBase}/receipt/parse`, { method: 'POST', body: form });
      if (!res.ok) {
        let message = `Receipt upload failed (${res.status})`;
        try {
          const payload = await res.json();
          if (payload?.error) message = payload.error;
        } catch {
          // ignore JSON parse errors
        }
        receiptError = message;
        return;
      }
      const result = (await res.json()) as ReceiptParseResult;
      receiptResult = result;
      detectedCurrency = result?.currency ? result.currency.toUpperCase() : null;
      receiptCurrencySelection = (detectedCurrency || roomCurrency || DEFAULT_CURRENCY).toUpperCase();
      const parseFactor = factorFor(receiptCurrencySelection);
      const parseExp = exponentFor(receiptCurrencySelection);
      parsedTaxInput =
        result?.tax_cents != null ? (Number(result.tax_cents) / parseFactor).toFixed(parseExp) : '';
      receiptWarnings = Array.isArray(result.warnings) ? result.warnings.filter(Boolean) : [];
      // Warnings mean "review recommended", not necessarily a broken import.
      warningBanner = receiptWarnings.length > 0 || (typeof result.confidence === 'number' && result.confidence < 0.6);
      const agg = new Map<
        string,
        { name: string; qty: number; unit: number; discount: number; discountPct: number; line: number }
      >();
      result.items.forEach((item) => {
        const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
        // Receipt parse values are in minor units for the detected currency.
        let unit = item.unit_price_cents != null ? item.unit_price_cents / parseFactor : 0;
        let line = item.line_price_cents != null ? item.line_price_cents / parseFactor : 0;
        if (!unit && line && qty > 0) unit = line / qty;
        if (!line && unit && qty > 0) line = unit * qty;
        const disc = item.discount_cents != null ? item.discount_cents / parseFactor : 0;
        const discPct = item.discount_percent != null ? item.discount_percent : unit ? (disc / unit) * 100 : 0;
        const key = `${(item.name || 'Item').trim().toLowerCase()}|${unit.toFixed(parseExp)}|${disc.toFixed(parseExp)}`;
        const existing = agg.get(key);
        if (existing) {
          existing.qty += qty;
          existing.line += line;
        } else {
          agg.set(key, { name: item.name ?? '', qty, unit, discount: disc, discountPct: discPct, line });
        }
      });
      editableItems = Array.from(agg.values()).map((item) => ({
        name: item.name,
        quantity: String(item.qty),
        unitPrice: item.unit ? item.unit.toFixed(parseExp) : '',
        linePrice: item.line ? item.line.toFixed(parseExp) : '',
        discountCents: item.discount ? item.discount.toFixed(parseExp) : '',
        discountPercent: item.discountPct ? item.discountPct.toFixed(2) : ''
      }));
      showReceiptReview = true;
    } catch (err) {
      receiptError = err instanceof Error ? err.message : 'Receipt upload failed';
    } finally {
      receiptUploading = false;
      target.value = '';
    }
  };

  const recalcDerived = (index: number, changed: 'quantity' | 'unitPrice' | 'linePrice' | 'discountCents' | 'discountPercent') => {
    editableItems = editableItems.map((item, i) => {
      if (i !== index) return item;

      const qtyVal = Number.parseFloat(item.quantity || '');
      const hasQty = Number.isFinite(qtyVal) && qtyVal > 0;
      const quantity = hasQty ? qtyVal : null;

      const exp = exponentFor(roomCurrency);
      const factor = factorFor(roomCurrency);
      let unitPrice = Number.parseFloat(item.unitPrice || '');
      let linePrice = Number.parseFloat(item.linePrice || '');
      let discountCents = Number.parseFloat(item.discountCents || ''); // per-unit
      let discountPercent = Number.parseFloat(item.discountPercent || '');

      if (changed === 'quantity' && quantity !== null) {
        if (unitPrice) {
          linePrice = hasQty ? unitPrice * quantity : linePrice;
        } else if (linePrice && hasQty) {
          unitPrice = linePrice / quantity;
        }
      } else if (changed === 'unitPrice' && hasQty && unitPrice) {
        linePrice = unitPrice * quantity;
      } else if (changed === 'linePrice' && hasQty && linePrice) {
        unitPrice = linePrice / quantity;
      } else if (!linePrice && hasQty && unitPrice) {
        linePrice = unitPrice * quantity;
      } else if (!unitPrice && hasQty && linePrice) {
        unitPrice = linePrice / quantity;
      }

      // discounts are per-unit; do not adjust linePrice (keep gross)
      if (unitPrice) {
        if (changed === 'discountPercent' && discountPercent) {
          discountCents = unitPrice * (discountPercent / 100);
        } else if (changed === 'discountCents' && discountCents) {
          discountPercent = (discountCents / unitPrice) * 100;
        } else if (discountPercent && !discountCents) {
          discountCents = unitPrice * (discountPercent / 100);
        } else if (discountCents && !discountPercent) {
          discountPercent = (discountCents / unitPrice) * 100;
        }
      }

      const fmt = (v: number) => (Number.isFinite(v) && v !== 0 ? v.toFixed(exp) : '');
      const next = { ...item };
      if (quantity !== null) next.quantity = String(quantity); // leave as-is if user cleared
      if (changed !== 'unitPrice') next.unitPrice = fmt(unitPrice);
      if (changed !== 'linePrice') next.linePrice = fmt(linePrice);
      if (changed !== 'discountCents') next.discountCents = fmt(discountCents);
      if (changed !== 'discountPercent') next.discountPercent = fmt(discountPercent);
      return next;
    });
  };

  const confirmReceipt = () => {
    if (!receiptResult || !ws) return;
    const toCents = (val: string) => parseByCurrency(val, receiptCurrencySelection || roomCurrency);
    const aggregated = new Map<
      string,
      { name: string; qty: number; unit: number; disc: number; discPct: number }
    >();

    editableItems.forEach((item) => {
      const quantity = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
      let unitPriceCents = toCents(item.unitPrice);
      let linePriceCents = toCents(item.linePrice);
      if (!unitPriceCents && linePriceCents && quantity > 0) {
        unitPriceCents = Math.round(linePriceCents / quantity);
      }
      let discountCents = toCents(item.discountCents); // per-unit
      const discountPercent = Number.parseFloat(item.discountPercent || '0') || 0;
      if (!discountCents && discountPercent && unitPriceCents) {
        discountCents = Math.round(unitPriceCents * (discountPercent / 100));
      }
        const key = `${(item.name || 'Item').trim().toLowerCase()}|${unitPriceCents}|${linePriceCents}|${discountCents}|${discountPercent}`;
        const existing = aggregated.get(key);
        if (existing) {
          existing.qty += quantity;
          existing.line = (existing.line || 0) + (linePriceCents || unitPriceCents * quantity);
        } else {
          aggregated.set(key, {
            name: item.name || 'Item',
            qty: quantity,
            unit: unitPriceCents,
            line: linePriceCents || unitPriceCents * quantity,
            disc: discountCents,
            discPct: discountPercent
          });
        }
      });

    Array.from(aggregated.values()).forEach((agg, idx) => {
      // show aggregated qty in review, but fan out into single-quantity items when importing
      const unitLine = Math.round((agg.line || agg.unit * agg.qty) / agg.qty) || agg.unit;
      for (let i = 0; i < agg.qty; i++) {
        const itemId = `${Date.now()}-${idx}-${i}-${Math.random().toString(36).slice(2, 6)}`;
        const opPayload = {
          item: {
            id: itemId,
            name: agg.name,
            quantity: 1,
            unit_price_cents: agg.unit,
            line_price_cents: unitLine,
            discount_cents: agg.disc,
            discount_percent: agg.discPct,
            assigned: {}
          }
        };
        ws?.send(
          JSON.stringify({
            type: 'op',
            op: {
              kind: 'set_item',
              actor_id: identity.userId,
              payload: opPayload
            }
          })
        );
      applyLocalOp({ kind: 'set_item', payload: opPayload });
      }
    });
    showReceiptReview = false;
    const taxDelta = parsedTaxCents;
    if (ws && room) {
      if (receiptCurrencySelection && receiptCurrencySelection !== roomCurrency) {
        changeCurrency(receiptCurrencySelection);
      }
      const currentTax = room.tax_cents || 0;
      const newTax = receiptIsAddon ? currentTax + taxDelta : taxDelta;
      const payload = { tax_cents: newTax, tip_cents: room.tip_cents || 0 };
      ws.send(
        JSON.stringify({
          type: 'op',
          op: { kind: 'set_tax_tip', actor_id: identity.userId, payload }
        })
      );
      applyLocalOp({ kind: 'set_tax_tip', payload, timestamp: Date.now() });
    }
    receiptResult = null;
    editableItems = [];
    parsedTaxInput = '';
    receiptWarnings = [];
    warningBanner = false;
    detectedCurrency = null;
    receiptCurrencySelection = (roomCurrency || DEFAULT_CURRENCY).toUpperCase();
  };

  const parseCents = (value: string) => {
    const cleaned = value.replace(/[^\d.-]/g, '');
    if (!cleaned) return 0;
    const numeric = Number(cleaned);
    if (!Number.isFinite(numeric)) return 0;
    return Math.round(numeric * 100);
  };

  const recalcItemForm = (changed: 'quantity' | 'unitPrice' | 'linePrice' | 'discountCents' | 'discountPercent') => {
    const qtyVal = Number.parseFloat(itemForm.quantity || '');
    const hasQty = Number.isFinite(qtyVal) && qtyVal > 0;
    const quantity = hasQty ? qtyVal : null;

    let unitPrice = Number.parseFloat(itemForm.unitPrice || '');
    let linePrice = Number.parseFloat(itemForm.linePrice || '');
    const discountCentsRaw = itemForm.discountCents || '';
    const discountPercentRaw = itemForm.discountPercent || '';
    let discountCents = Number.parseFloat(discountCentsRaw); // per-unit
    let discountPercent = Number.parseFloat(discountPercentRaw);

    const wantsClearCents =
      changed === 'discountCents' && (discountCentsRaw.trim() === '' || Number(discountCentsRaw) === 0);
    const wantsClearPercent =
      changed === 'discountPercent' && (discountPercentRaw.trim() === '' || Number(discountPercentRaw) === 0);

    if (wantsClearCents) {
      discountCents = 0;
      discountPercent = 0;
    } else if (wantsClearPercent) {
      discountPercent = 0;
      if (discountCentsRaw.trim() === '' || Number(discountCentsRaw) === 0) discountCents = 0;
    }

    if (changed === 'quantity' && quantity !== null) {
      if (unitPrice) {
        linePrice = unitPrice * quantity; // keep gross unit
      } else if (linePrice) {
        unitPrice = linePrice / quantity;
      }
    } else if (changed === 'unitPrice' && hasQty && unitPrice) {
      linePrice = unitPrice * quantity!;
    } else if (changed === 'linePrice' && hasQty && linePrice) {
      unitPrice = linePrice / quantity!;
    } else if (!linePrice && hasQty && unitPrice) {
      linePrice = unitPrice * quantity!;
    } else if (!unitPrice && hasQty && linePrice) {
      unitPrice = linePrice / quantity!;
    }

    if (unitPrice) {
      if (changed === 'discountPercent' && discountPercent) {
        discountCents = unitPrice * (discountPercent / 100);
      } else if (changed === 'discountCents' && discountCents) {
        discountPercent = unitPrice ? (discountCents / unitPrice) * 100 : discountPercent;
      } else if (discountPercent && !discountCents && !wantsClearPercent) {
        discountCents = unitPrice * (discountPercent / 100);
      } else if (discountCents && !discountPercent && !wantsClearCents) {
        discountPercent = unitPrice ? (discountCents / unitPrice) * 100 : discountPercent;
      }
    }

      const fmt = (v: number) => (Number.isFinite(v) && v !== 0 ? v.toFixed(exp) : '');
    const next = { ...itemForm };
    if (quantity !== null) next.quantity = String(quantity); // allow user to clear
    if (changed !== 'unitPrice') next.unitPrice = fmt(unitPrice);
    if (changed !== 'linePrice') next.linePrice = fmt(linePrice);
    if (changed !== 'discountCents') next.discountCents = fmt(discountCents);
    if (changed !== 'discountPercent') next.discountPercent = fmt(discountPercent);
    itemForm = next;
  };

  const openNewItemModal = () => {
    itemModalMode = 'new';
    itemModalId = null;
    itemForm = { name: '', quantity: '1', unitPrice: '', linePrice: '', discountCents: '', discountPercent: '' };
    showItemModal = true;
  };

  const openEditItemModal = (item: Item) => {
    itemModalMode = 'edit';
    itemModalId = item.id;
    itemForm = {
      name: item.name,
      quantity: String(item.quantity || 1),
      unitPrice: item.unit_price_cents ? (item.unit_price_cents / 100).toFixed(2) : '',
      linePrice: item.line_price_cents ? (item.line_price_cents / 100).toFixed(2) : '',
      discountCents: item.discount_cents ? (item.discount_cents / 100).toFixed(2) : '',
      discountPercent: item.discount_percent ? item.discount_percent.toFixed(2) : ''
    };
    showItemModal = true;
  };

  const submitItemModal = () => {
    if (!itemForm.name.trim()) return;
    const quantity = itemModalMode === 'new' ? Math.max(1, Number.parseFloat(itemForm.quantity || '1') || 1) : 1;
    const unitPriceCents = parseCents(itemForm.unitPrice);
    const discountCentsRaw = (itemForm.discountCents || '').trim();
    const discountPercentRaw = (itemForm.discountPercent || '').trim();

    let discountCents = 0;
    let discountPercent = 0;
    if (discountCentsRaw !== '' && discountPercentRaw !== '') {
      discountCents = parseCents(discountCentsRaw);
      discountPercent = Number.parseFloat(discountPercentRaw) || 0;
    } else if (discountCentsRaw !== '') {
      discountCents = parseCents(discountCentsRaw);
      discountPercent = unitPriceCents ? (discountCents / unitPriceCents) * 100 : 0;
    } else if (discountPercentRaw !== '') {
      discountPercent = Number.parseFloat(discountPercentRaw) || 0;
      discountCents = unitPriceCents ? Math.round(unitPriceCents * (discountPercent / 100)) : 0;
    }

    let linePriceCents = parseCents(itemForm.linePrice);
    if (!linePriceCents && unitPriceCents) {
      linePriceCents = unitPriceCents * quantity; // gross
    }
    const netLine = linePriceCents || unitPriceCents * quantity;
    const sendItem = (id: string) => {
      const assigned =
        itemModalMode === 'edit' && itemModalId && room?.items?.[itemModalId]?.assigned
          ? { ...room.items[itemModalId].assigned }
          : {};
      const payload = {
        item: {
          id,
          name: itemForm.name.trim(),
          quantity: 1,
          unit_price_cents: unitPriceCents || netLine,
          line_price_cents: linePriceCents ? Math.round(linePriceCents / quantity) : unitPriceCents,
          discount_cents: discountCents,
          discount_percent: discountPercent,
          assigned
        }
      };
      sendOp({ kind: 'set_item', actor_id: identity.userId, payload });
      applyLocalOp({ kind: 'set_item', payload });
    };

    if (itemModalMode === 'edit' && itemModalId) {
      sendItem(itemModalId);
    } else {
      for (let i = 0; i < quantity; i++) {
        sendItem(`${Date.now()}-${i}-${Math.random().toString(36).slice(2, 6)}`);
      }
    }
    showItemModal = false;
    // ensure we converge with server after batch sends
    requestSnapshot();
  };

  const duplicateItem = (item: Item) => {
    openEditItemModal(item);
    itemModalMode = 'new';
    itemModalId = null;
  };

  const sendParticipantUpdate = (id: string, name: string, present: boolean) => {
    if (!name.trim()) return;

    const normalizedName = name.trim();
    const nameKey = normalizedName.toLowerCase();
    const existingMatch = Object.entries(room?.participants || {}).find(
      ([, p]) => (p as Participant).name.trim().toLowerCase() === nameKey
    );
    const targetId = existingMatch ? existingMatch[0] : id;
    const colorSeed =
      room?.participants?.[targetId]?.colorSeed ||
      hexSeed(targetId) ||
      fallbackSeed(targetId, normalizedName);

    const participant = {
      id: targetId,
      name: normalizedName,
      initials: initialsFromName(normalizedName),
      color_seed: colorSeed,
      colorSeed,
      present
    };

    sendOp({
      kind: 'set_participant',
      actor_id: identity.userId || targetId,
      payload: { participant }
    });

    applyLocalOp({ kind: 'set_participant', payload: { participant } });

    // Only update local identity when we're modifying ourselves.
    if (identity.userId === targetId) {
      identity = {
        ...identity,
        name: participant.name,
        initials: participant.initials,
        colorSeed: participant.colorSeed
      };
      localStorage.setItem(`room:${roomCode}:identity`, JSON.stringify(identity));
    }
  };

  const sendPresence = (present: boolean) => {
    const stored = localStorage.getItem(`room:${roomCode}:identity`);
    if (!stored) return;
    const id = JSON.parse(stored);
    sendParticipantUpdate(id.userId, id.name, present);
  };

  const removeParticipant = (id: string) => {
    if (participantAssignments[id]) return;
    sendOp({
      kind: 'remove_participant',
      actor_id: identity.userId,
      payload: { id }
    });
    applyLocalOp({ kind: 'remove_participant', payload: { id } });
  };

  const removeItem = (itemId: string) => {
    sendOp({
      kind: 'remove_item',
      actor_id: identity.userId,
      payload: { id: itemId }
    });
    applyLocalOp({
      kind: 'remove_item',
      payload: { id: itemId }
    });
  };

  const sendRoomNameUpdate = (name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    const payload = { name: trimmed };
    sendOp({
      kind: 'set_room_name',
      actor_id: identity.userId,
      payload
    });
    applyLocalOp({ kind: 'set_room_name', payload, timestamp: Date.now() });
  };

  const joinRoomWithName = async () => {
    if (!joinNameInput.trim()) {
      joinError = 'Please enter a name to join.';
      return;
    }
    joinError = null;
    try {
      const res = await fetch(`${apiBase}/join-room`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ room_code: roomCode.toUpperCase(), name: joinNameInput.trim() })
      });
      if (!res.ok) {
        joinError = `Join failed (${res.status})`;
        return;
      }
      const data = await res.json();
      identity = {
        userId: data.user_id,
        name: joinNameInput.trim(),
        initials: initialsFromName(joinNameInput),
        colorSeed: hexSeed(data.user_id)
      };
      localStorage.setItem(`room:${data.room_code}:identity`, JSON.stringify(identity));
      showJoinPrompt = false;
      connectWS();
    } catch (err) {
      joinError = err instanceof Error ? err.message : 'Join failed';
    }
  };

  onMount(() => {
    const stored = localStorage.getItem(`room:${roomCode}:identity`);
    if (stored) {
      identity = JSON.parse(stored);
      connectWS();
    } else {
      showJoinPrompt = true;
    }
    if (browser) {
      shareLink = `${window.location.origin}/room/${roomCode}`;
      qrUrl = `https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=${encodeURIComponent(shareLink)}`;
      const kickReconnect = () => {
        reconnectDelay = RECONNECT_MIN;
        scheduleReconnect();
      };
      window.addEventListener('online', kickReconnect);
      window.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') kickReconnect();
      });
    }
  });

  $: items = room ? (Object.values(room.items) as Item[]) : [];
  $: participants = room ? (Object.values(room.participants) as Participant[]) : [];
  $: initialsCounts = participants.reduce((acc, p) => {
    const init = p.initials || initialsFromName(p.name);
    acc[init] = (acc[init] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);
  $: initialsBadges = (() => {
    const seen: Record<string, number> = {};
    const badges: Record<string, string> = {};
    participants.forEach((p) => {
      const init = p.initials || initialsFromName(p.name);
      seen[init] = (seen[init] || 0) + 1;
      if ((initialsCounts[init] || 0) > 1) {
        badges[p.id] = String(seen[init]);
      }
    });
    return badges;
  })();
  $: receiptSubtotalCents = subtotalFromEditable(editableItems);
  $: parsedTaxCents = parseByCurrency(parsedTaxInput, receiptCurrencySelection || roomCurrency);
  $: projectedTaxCents =
    receiptCurrencySelection && roomCurrency && receiptCurrencySelection !== roomCurrency
      ? NaN
      : baselineTaxCents + parsedTaxCents;
  $: receiptTaxPercent =
    receiptSubtotalCents > 0 ? (parsedTaxCents / receiptSubtotalCents) * 100 : 0;
  $: projectedTaxPercent =
    !Number.isFinite(projectedTaxCents)
      ? 0
      : baselineSubtotalCents + receiptSubtotalCents > 0
      ? (projectedTaxCents / (baselineSubtotalCents + receiptSubtotalCents)) * 100
      : 0;

  const toCents = (val: string, code = roomCurrency) => {
    const factor = factorFor(code);
    return Math.round((Number.parseFloat(val || '0') || 0) * factor);
  };
  const preTaxSubtotal = (list: Item[]) => {
    if (!list?.length) return 0;
    const subtotal = list.reduce((sum, it) => {
      const qty = it.quantity || 1;
      const gross = Number(it.line_price_cents || 0);
      const discount = Number(it.discount_cents || 0) * qty;
      const net = Math.max(0, gross - discount);
      return sum + net;
    }, 0);
    return Number.isFinite(subtotal) ? Math.max(0, Math.round(subtotal)) : 0;
  };
  $: preTaxSubtotalCents = preTaxSubtotal(items);
  $: (() => {
    const exp = exponentFor(roomCurrency);
    const factor = factorFor(roomCurrency);
    taxCentsPreview = toCents(
      taxInput || (room?.tax_cents ? (room.tax_cents / factor).toFixed(exp) : '0'),
      roomCurrency
    );
    tipCentsPreview = toCents(
      tipInput || (room?.tip_cents ? (room.tip_cents / factor).toFixed(exp) : '0'),
      roomCurrency
    );
  })();
  $: taxPercent = preTaxSubtotalCents > 0 ? (taxCentsPreview / preTaxSubtotalCents) * 100 : 0;
  $: tipPercent = preTaxSubtotalCents > 0 ? (tipCentsPreview / preTaxSubtotalCents) * 100 : 0;

  const setTipPercent = (pct: number) => {
    const base = preTaxSubtotalCents || 0;
    const tip = Math.round((base * pct) / 100);
    const exp = exponentFor(roomCurrency);
    const factor = factorFor(roomCurrency);
    tipInput = (tip / factor).toFixed(exp);
    tipCentsPreview = tip;
    tipPercent = base > 0 ? (tip / base) * 100 : 0;
  };
</script>

<div class="min-h-screen bg-surface-900 text-surface-50 pb-24 relative">
  {#if wsStatus !== 'connected'}
    <div class="absolute top-3 inset-x-0 flex justify-center pointer-events-none z-50 px-4">
      <div class="flex items-center gap-3 rounded-xl border border-warning-500/60 bg-warning-900/95 text-warning-50 text-xs px-3 py-2 shadow-xl pointer-events-auto">
        <span>⚠️</span>
        <span class="flex-1">
          {wsStatus === 'reconnecting' ? 'Reconnecting to room…' : 'Connecting…'}
        </span>
        <button
          class="action-btn action-btn-surface action-btn-compact"
          type="button"
          on:click={() => {
            if (reconnectTimer) clearTimeout(reconnectTimer);
            reconnectDelay = RECONNECT_MIN;
            connectWS();
          }}
        >
          Retry
        </button>
      </div>
    </div>
  {/if}
  <header class="px-6 pt-6 pb-4 space-y-3">
    <div class="accent-gradient rounded-3xl p-5 shadow-2xl flex flex-col md:flex-row md:items-center md:justify-center gap-4">
      <div class="text-center flex-1 order-1">
        <p class="text-sm text-white/70">Restaurant</p>
        <h1 class="text-2xl font-semibold text-white">{room?.name || 'Shared Bill'}</h1>
        <div class="flex items-center justify-center gap-3 mt-2 text-sm text-white/80 flex-wrap">
          <span class="rounded-full bg-black/20 px-3 py-1 font-mono">Bill code: {roomCode?.toUpperCase()}</span>
          {#if qrUrl}
            <img src={qrUrl} alt="Join QR code" class="w-16 h-16 rounded-lg border border-white/20 bg-white/5" />
          {/if}
        </div>
      </div>
      <div class="flex flex-col items-center justify-center order-2 md:order-3 md:w-48">
        <Avatar
          initials={identity.initials}
          color={colorHex(identity.colorSeed)}
          size={60}
          badge={initialsBadges[identity.userId] ? String(initialsBadges[identity.userId]) : undefined}
          title={identity.name}
        />
        <div class="text-sm text-white mt-2">{identity.name || 'You'}</div>
        <div class="flex items-center justify-center gap-2 mt-3 w-full">
          <button class="action-btn action-btn-surface action-btn-compact text-xs sm:text-sm flex-1" on:click={() => { nameInput = identity.name; showNameModal = true; }}>
            <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#eab308">
              <path d="M12 20h9" />
              <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5Z" />
            </svg>
            <span class="inline ml-1">Change name</span>
          </button>
        </div>
        <div class="flex items-center justify-center gap-2 mt-2 w-full">
          <button class="action-btn action-btn-surface action-btn-compact text-xs sm:text-sm flex-1" on:click={() => { roomNameInput = room?.name || ''; showRoomNameModal = true; }}>
            <svg class="inline-block align-middle" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round">
              <path d="M20 12l-8-8H6a2 2 0 0 0-2 2v6l8 8a2 2 0 0 0 2.8 0l5.2-5.2A2 2 0 0 0 20 12Z" />
              <path d="M7.5 7.5h.01" />
            </svg>
            <span class="inline-block ml-1 whitespace-normal leading-tight text-left">
              <span class="block">Rename</span>
              <span class="block">restaurant</span>
            </span>
          </button>
          <button class="action-btn action-btn-primary action-btn-compact text-xs sm:text-sm flex-1" on:click={() => { addPersonName = ''; showAddPersonModal = true; }}>
            <svg class="inline-block align-middle" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#22c55e">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            <span class="inline-block ml-1 whitespace-normal leading-tight text-left">
              <span class="block">Add</span>
              <span class="block">person</span>
            </span>
          </button>
        </div>
      </div>
    </div>
    <div class="flex flex-col md:flex-row gap-3 items-center justify-center mt-2">
      <div class="flex items-center gap-2">
        <span class="text-sm text-white/70">Bill currency:</span>
        <select
          class="input bg-white/5 border border-white/15 rounded-lg px-3 py-2 text-white text-sm"
          bind:value={roomCurrency}
          on:change={(e) => changeCurrency((e.target as HTMLSelectElement).value)}
        >
          {#each COMMON_CURRENCIES as c}
            <option value={c.code}>{c.flag} {c.code} {c.symbol}</option>
          {/each}
        </select>
      </div>
    </div>
    <div class="flex gap-3 overflow-x-auto pt-2 justify-center">
      {#if room}
        {#each participants as participant}
          <div class="flex flex-col items-center text-xs relative w-14 shrink-0">
            <Avatar
              initials={participant.initials}
              color={colorHex(participant.colorSeed)}
              size={36}
              badge={initialsBadges[participant.id] ? String(initialsBadges[participant.id]) : undefined}
              title={participant.name}
            />
            <span
              class="absolute -right-1 -top-1 w-3 h-3 rounded-full border border-surface-900"
              style={`background:${participant.present ? '#22c55e' : '#6b7280'};`}
              title={participant.present ? 'Present' : 'Not present'}
            ></span>
            {#if !participant.present && !participantAssignments[participant.id]}
              <button
                class="action-btn action-btn-danger action-btn-compact mt-1 w-full"
                title="Remove"
                on:click={() => removeParticipant(participant.id)}
              >
                <svg class="inline-block align-middle" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#ef4444">
                  <path d="M3 6h18" />
                  <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
                  <path d="M10 11v6" />
                  <path d="M14 11v6" />
                </svg>
              </button>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  </header>

  <main class="px-6 space-y-4">
    {#if warningBanner}
      <div class="rounded-xl bg-warning-500/20 text-warning-200 px-4 py-3 text-sm border border-warning-500/40 flex items-center justify-between gap-3">
        <div class="min-w-0">
          Receipt parsed with {receiptWarnings.length} note{receiptWarnings.length === 1 ? '' : 's'}—review recommended.
        </div>
        <button class="btn btn-outline shrink-0" type="button" on:click={() => (showReceiptReview = true)}>
          Review
        </button>
      </div>
    {/if}
    {#if receiptError}
      <div class="rounded-xl bg-error-500/20 text-error-200 px-4 py-3 text-sm border border-error-500/40">
        {receiptError}
      </div>
    {/if}

    <section class="space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Items</h2>
        {#if room}
          <label
            class={`action-btn action-btn-surface action-btn-compact ${receiptUploading ? 'opacity-60 pointer-events-none' : ''}`}
          >
            <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round">
              <path d="M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2Z" />
              <path d="M9 7l1.5-2.5h3L15 7" />
              <circle cx="12" cy="13" r="3.5" />
            </svg>
            <span class="ml-1">
              {receiptUploading
                ? 'Uploading...'
                : items.length > 0
                  ? 'Add receipt'
                  : 'Upload receipt'}
            </span>
            <input type="file" class="hidden" accept="image/*" on:change={submitReceipt} />
          </label>
        {/if}
      </div>
      {#if room}
        {#each items as item}
          <div class="glass-card rounded-2xl p-4 flex items-center justify-between w-full text-left">
            <div
              class="flex-1 pr-4 min-w-0"
              role="button"
              tabindex="0"
              on:click={() => toggleAssign(item.id, identity.userId)}
              on:keydown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  toggleAssign(item.id, identity.userId);
                }
              }}
            >
              <p
                class="font-semibold text-white leading-tight break-words truncate"
                style="font-size: clamp(13px, 4vw, 16px);"
                title={item.name}
              >
                {item.name}
              </p>
              <p class="text-sm text-surface-200 whitespace-nowrap">{formatAmount(item.line_price_cents)}</p>
              {#if item.discount_cents}
                <p class="text-xs text-surface-300">
                  Discount: {formatAmount(item.discount_cents)} · Net: {formatAmount(item.line_price_cents - item.discount_cents)}
                </p>
              {/if}
              <div class="text-xs text-surface-400 flex flex-wrap items-center gap-2">
                <span>Assigned:</span>
                <div class="flex items-center gap-1 flex-wrap">
                  {#if room}
                    {#each Object.entries(item.assigned || {}) as [uid, on]}
                      {#if on}
                        <Avatar
                          initials={room.participants?.[uid]?.initials || initialsFromName(room.participants?.[uid]?.name || uid)}
                          color={colorHex(room.participants?.[uid]?.colorSeed)}
                          badge={initialsBadges[uid] ? String(initialsBadges[uid]) : undefined}
                          title={room.participants?.[uid]?.name}
                          size={22}
                        />
                      {/if}
                    {/each}
                  {/if}
                </div>
              </div>
            </div>
            <div class="flex flex-col gap-2 mt-2 items-end flex-shrink-0 w-auto ml-2">
              <div class="flex gap-2 flex-wrap justify-end">
                <button
                  class="action-btn action-btn-surface action-btn-compact"
                  title="Assign participants"
                  type="button"
                  on:click|stopPropagation={() => {
                    showAssign = true;
                    activeItemId = item.id;
                  }}
                >
                  <svg class="inline-block align-middle" width="18" height="18" viewBox="0 0 34 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                    <circle cx="9" cy="7" r="4" />
                    <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
                    <path d="M16 3.13a4 4 0 1 1 0 7.75" />
                    <g transform="translate(9,-1)" stroke="#22c55e" stroke-width="2.35">
                      <line x1="18" y1="6" x2="18" y2="14" />
                      <line x1="14" y1="10" x2="22" y2="10" />
                    </g>
                  </svg>
                  <span class="hidden sm:inline ml-1">Assign</span>
                </button>
                <button
                  class={`action-btn ${item.assigned?.[identity.userId] ? 'action-btn-danger' : 'action-btn-primary'} action-btn-compact`}
                  title={item.assigned?.[identity.userId] ? 'Unassign me' : 'Assign to me'}
                  type="button"
                  on:click|stopPropagation={() => toggleAssign(item.id, identity.userId)}
                >
                  {#if item.assigned?.[identity.userId]}
                    <svg
                      class="inline-block align-middle"
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.75"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      style="color:#ef4444"
                    >
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                  {:else}
                    <svg
                      class="inline-block align-middle"
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.75"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      style="color:#22c55e"
                    >
                      <line x1="12" y1="5" x2="12" y2="19" />
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                  {/if}
                  <span class="hidden sm:inline ml-1">{item.assigned?.[identity.userId] ? 'Unassign' : 'Me'}</span>
                </button>
              </div>
              <div class="flex gap-2 flex-wrap justify-end">
                <button
                  class="action-btn action-btn-surface action-btn-compact"
                  title="Edit"
                  type="button"
                  on:click|stopPropagation={() => openEditItemModal(item)}
                >
                  <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#eab308">
                    <path d="M12 20h9" />
                    <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5Z" />
                  </svg>
                  <span class="hidden sm:inline ml-1">Edit</span>
                </button>
                <button
                  class="action-btn action-btn-surface action-btn-compact"
                  title="Copy"
                  type="button"
                  on:click|stopPropagation={() => duplicateItem(item)}
                >
                  <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round">
                    <rect x="9" y="9" width="11" height="11" rx="2" />
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                  </svg>
                  <span class="hidden sm:inline ml-1">Copy</span>
                </button>
                <button
                  class="action-btn action-btn-danger action-btn-compact"
                  title="Delete"
                  type="button"
                  on:click|stopPropagation={() => removeItem(item.id)}
                >
                  <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#ef4444">
                    <path d="M3 6h18" />
                    <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
                    <path d="M10 11v6" />
                    <path d="M14 11v6" />
                  </svg>
                  <span class="hidden sm:inline ml-1">Delete</span>
                </button>
              </div>
            </div>
          </div>
        {/each}
      {/if}
    </section>
  </main>

  <div class="fixed bottom-0 inset-x-0 bg-surface-900/90 border-t border-surface-800 px-6 py-3 flex gap-2 backdrop-blur">
    <button class="btn btn-primary flex-1" on:click={openNewItemModal}>Add Item</button>
    <button class="btn btn-outline flex-1" on:click={() => { showTaxTipModal = true; const factor = factorFor(roomCurrency); const exp = exponentFor(roomCurrency); taxInput = room?.tax_cents ? (room.tax_cents/factor).toFixed(exp) : ''; tipInput = room?.tip_cents ? (room.tip_cents/factor).toFixed(exp) : ''; }}>Tax/Tip</button>
    <button class="btn btn-outline flex-1" on:click={async () => { await buildSummary(); showSummary = true; }}>Summary</button>
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
                <Avatar
                  initials={participant.initials}
                  color={colorHex(participant.colorSeed)}
                  size={32}
                  badge={initialsBadges[participant.id] ? String(initialsBadges[participant.id]) : undefined}
                  title={participant.name}
                />
                <span>{participant.name}</span>
              </div>
              {#if room?.items?.[activeItemId]}
                {#if room.items[activeItemId].assigned?.[participant.id]}
                  <span class="text-xs px-2 py-1 rounded-full bg-error-500/20 text-error-100 border border-error-400/40 flex items-center gap-1">
                    <svg
                      width="14"
                      height="14"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.75"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      style="color:#ef4444"
                    >
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                    Unassign
                  </span>
                {:else}
                  <span class="text-xs px-2 py-1 rounded-full bg-primary-500/20 text-primary-100 border border-primary-400/40 flex items-center gap-1">
                    <svg
                      width="14"
                      height="14"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.75"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      style="color:#22c55e"
                    >
                      <line x1="12" y1="5" x2="12" y2="19" />
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                    Assign
                  </span>
                {/if}
              {:else}
                <span class="text-sm text-surface-500">Tap to toggle</span>
              {/if}
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
        <p class="text-sm text-surface-200">Edit aggregated lines before importing. Prices are gross (pre-discount).</p>
        <div class="flex items-center justify-between gap-3 flex-wrap rounded-xl border border-white/10 bg-white/5 px-3 py-2">
          <div class="text-sm text-white/80">Receipt Currency</div>
          <select
            class="input bg-white/5 border border-white/15 rounded-lg px-3 py-2 text-white text-sm"
            bind:value={receiptCurrencySelection}
            on:change={(e) => {
              const next = ((e.target as HTMLSelectElement).value || '').toUpperCase();
              receiptCurrencySelection = next || receiptCurrencySelection;
              // keep parsed tax display aligned to the selected currency exponent
              const exp = exponentFor(receiptCurrencySelection);
              parsedTaxInput = parsedTaxInput ? Number.parseFloat(parsedTaxInput).toFixed(exp) : parsedTaxInput;
            }}
          >
            {#each COMMON_CURRENCIES as c}
              <option value={c.code}>{c.flag} {c.code} {c.symbol}</option>
            {/each}
          </select>
          {#if detectedCurrency && detectedCurrency !== receiptCurrencySelection}
            <button class="btn btn-outline" type="button" on:click={() => (receiptCurrencySelection = detectedCurrency!)}>
              Use detected {detectedCurrency}
            </button>
          {/if}
        </div>
        {#if receiptWarnings.length > 0}
          <div class="rounded-xl border border-warning-500/40 bg-warning-500/10 text-warning-200 p-3 text-sm space-y-1">
            <div class="font-semibold">Notes from parser</div>
            <ul class="list-disc list-inside space-y-1">
              {#each receiptWarnings as w}
                <li>{w}</li>
              {/each}
            </ul>
          </div>
        {/if}
        {#each editableItems as item, index}
          <div class="border border-surface-800 rounded-xl p-4 space-y-3">
            <label class="block space-y-1">
              <span class="text-xs text-surface-300">Item Name</span>
              <input class="input w-full" bind:value={editableItems[index].name} />
            </label>
            <div class="grid grid-cols-2 gap-3">
              <label class="block space-y-1">
                <span class="text-xs text-surface-300">Quantity</span>
                <input
                  class="input w-full"
                 type="number"
                 min="1"
                 step="1"
                 bind:value={editableItems[index].quantity}
                 on:input={() => recalcDerived(index, 'quantity')}
               />
             </label>
             <label class="block space-y-1">
                <span class="text-xs text-surface-300">Unit Price ({symbolFor(receiptCurrencySelection) || receiptCurrencySelection})</span>
                <input
                  class="input w-full"
                  type="number"
                  min="0"
                 step={1 / factorFor(receiptCurrencySelection)}
                  bind:value={editableItems[index].unitPrice}
                  on:input={() => recalcDerived(index, 'unitPrice')}
                />
              </label>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <label class="block space-y-1">
                <span class="text-xs text-surface-300">Total ({symbolFor(receiptCurrencySelection) || receiptCurrencySelection})</span>
                <input
                  class="input w-full"
                  type="number"
                  min="0"
                  step={1 / factorFor(receiptCurrencySelection)}
                  bind:value={editableItems[index].linePrice}
                  on:input={() => recalcDerived(index, 'linePrice')}
                />
              </label>
              <label class="block space-y-1">
                <span class="text-xs text-surface-300">Discount ({symbolFor(receiptCurrencySelection) || receiptCurrencySelection} per unit)</span>
                <input
                  class="input w-full"
                  type="number"
                  min="0"
                  step={1 / factorFor(receiptCurrencySelection)}
                  bind:value={editableItems[index].discountCents}
                  on:input={() => recalcDerived(index, 'discountCents')}
                />
              </label>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <label class="block space-y-1">
                <span class="text-xs text-surface-300">Discount (%)</span>
                <input
                  class="input w-full"
                  type="number"
                  min="0"
                  step="0.01"
                  bind:value={editableItems[index].discountPercent}
                  on:input={() => recalcDerived(index, 'discountPercent')}
                />
              </label>
            </div>
            {#if editableItems[index]}
              {#if true}
                <div class="rounded-lg bg-surface-800/70 border border-surface-700 px-3 py-2 text-sm flex flex-col gap-1">
                  {#if editableItems[index]}
                    <div class="flex items-center justify-between">
                      <span class="text-surface-300">Discounted unit</span>
                      <span class="font-semibold text-white">
                        {formatAmount(
                          discountedUnitAndNetFromEditable(editableItems[index]).netUnit,
                          receiptCurrencySelection
                        )}
                      </span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span class="text-surface-300">Total after discount</span>
                      <span class="font-semibold text-white">
                        {formatAmount(
                          discountedUnitAndNetFromEditable(editableItems[index]).netTotal,
                          receiptCurrencySelection
                        )}
                      </span>
                    </div>
                  {/if}
                </div>
              {/if}
            {/if}
            <div class="flex justify-end">
              <button
                class="action-btn action-btn-danger action-btn-compact"
                on:click={() => removeEditableItem(index)}
                type="button"
              >
                <svg class="inline-block align-middle" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#ef4444">
                  <path d="M3 6h18" />
                  <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
                  <path d="M10 11v6" />
                  <path d="M14 11v6" />
                </svg>
                <span class="ml-1">Remove</span>
              </button>
            </div>
          </div>
        {/each}
        <div class="border border-surface-800 rounded-xl p-4 space-y-3">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">This receipt tax</p>
              <p class="text-xs text-surface-300">
                Receipt subtotal: {formatAmount(receiptSubtotalCents, receiptCurrencySelection)} · Tax %
                {receiptSubtotalCents > 0 ? ` ${receiptTaxPercent.toFixed(2)}%` : ' --%'}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-surface-300">
                {symbolFor(receiptCurrencySelection) || receiptCurrencySelection}
              </span>
              <input
                class="input w-28 text-right"
                inputmode="decimal"
                bind:value={parsedTaxInput}
                placeholder="0.00"
              />
            </div>
          </div>
          {#if receiptIsAddon}
            <div class="mt-3 flex items-start justify-between gap-3">
              <div>
                <p class="text-sm font-semibold">Projected total tax</p>
                <p class="text-xs text-surface-300">
                  {#if Number.isFinite(projectedTaxCents)}
                    {formatAmount(baselineTaxCents)} + {formatAmount(parsedTaxCents, receiptCurrencySelection)} =
                    {formatAmount(projectedTaxCents)}
                  {:else}
                    (Shown after import when bill currency matches receipt currency)
                  {/if}
                </p>
              </div>
              <div class="text-right text-sm">
                <div class="font-semibold">
                  {#if Number.isFinite(projectedTaxCents)}
                    {formatAmount(projectedTaxCents)}
                  {:else}
                    --
                  {/if}
                </div>
                <div class="text-xs text-surface-300">
                  Total tax %:
                  {Number.isFinite(projectedTaxCents) && baselineSubtotalCents + receiptSubtotalCents > 0
                    ? `${projectedTaxPercent.toFixed(2)}%`
                    : '--%'}
                </div>
              </div>
            </div>
          {:else}
            <div class="mt-2 text-xs text-surface-300">
              Tax % of this receipt:
              {receiptSubtotalCents > 0 ? `${receiptTaxPercent.toFixed(2)}%` : '--%'}
            </div>
          {/if}
        </div>
        <button class="btn btn-primary w-full" on:click={confirmReceipt}>Import items & tax</button>
        <button
          class="btn btn-outline w-full"
          on:click={() => {
            showReceiptReview = false;
            receiptWarnings = [];
            warningBanner = false;
            receiptCurrencySelection = (roomCurrency || DEFAULT_CURRENCY).toUpperCase();
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  {/if}

  {#if showItemModal}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">{itemModalMode === 'edit' ? 'Edit item' : 'Add item'}</h3>
        <label class="block">
          <span class="text-sm text-surface-200">Item Name</span>
          <input class="input w-full" bind:value={itemForm.name} placeholder="Cheeseburger" />
        </label>
        {#if itemModalMode === 'new'}
          <label class="block">
            <span class="text-sm text-surface-200">Quantity</span>
            <input
              class="input w-full"
              bind:value={itemForm.quantity}
              placeholder="1"
              inputmode="numeric"
              on:input={() => recalcItemForm('quantity')}
            />
          </label>
        {/if}
        <div class="grid grid-cols-2 gap-3">
          <label class="block">
            <span class="text-sm text-surface-200">Unit Price ({symbolFor(roomCurrency) || roomCurrency})</span>
            <input
              class="input w-full"
              bind:value={itemForm.unitPrice}
              placeholder="9.99"
              inputmode="decimal"
              on:input={() => recalcItemForm('unitPrice')}
            />
          </label>
          {#if itemModalMode === 'new'}
            <label class="block">
              <span class="text-sm text-surface-200">Total ({symbolFor(roomCurrency) || roomCurrency})</span>
              <input
                class="input w-full"
                bind:value={itemForm.linePrice}
                placeholder="19.98"
                inputmode="decimal"
                on:input={() => recalcItemForm('linePrice')}
              />
            </label>
          {/if}
        </div>
        <div class="grid grid-cols-2 gap-3">
          <label class="block">
            <span class="text-sm text-surface-200">Discount ({symbolFor(roomCurrency) || roomCurrency})</span>
            <input
              class="input w-full"
              bind:value={itemForm.discountCents}
              placeholder="2.00"
              inputmode="decimal"
              on:input={() => recalcItemForm('discountCents')}
            />
          </label>
          <label class="block">
            <span class="text-sm text-surface-200">Discount (%)</span>
            <input
              class="input w-full"
              bind:value={itemForm.discountPercent}
              placeholder="10"
              inputmode="decimal"
              on:input={() => recalcItemForm('discountPercent')}
            />
          </label>
        </div>
        <div class="rounded-lg bg-surface-800/70 border border-surface-700 px-3 py-2 text-sm flex items-center justify-between">
          <div class="flex flex-col gap-1 w-full">
            <div class="flex items-center justify-between">
              <span class="text-surface-300">Discounted unit</span>
              <span class="font-semibold text-white">{formatAmount(discountedUnitAndNetFromItemForm().netUnit)}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-surface-300">Total after discount</span>
              <span class="font-semibold text-white">{formatAmount(discountedUnitAndNetFromItemForm().netTotal)}</span>
            </div>
          </div>
        </div>
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showItemModal = false)}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={submitItemModal} disabled={!itemForm.name.trim()}>
            {itemModalMode === 'edit' ? 'Save' : 'Add'}
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showTaxTipModal}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Tax / Tip</h3>
        <p class="text-xs text-surface-300">
          Items subtotal: {formatAmount(preTaxSubtotalCents || 0, roomCurrency)}
        </p>
        <label class="block space-y-1">
          <span class="text-sm text-surface-200">
            Tax ({symbolFor(roomCurrency) || roomCurrency})
          </span>
          <input class="input w-full" bind:value={taxInput} inputmode="decimal" />
          <p class="text-xs text-surface-400">
            ≈ {(taxPercent || 0).toFixed(2)}% of items subtotal
          </p>
        </label>
        <label class="block space-y-1">
          <span class="text-sm text-surface-200">
            Tip ({symbolFor(roomCurrency) || roomCurrency})
          </span>
          <input class="input w-full" bind:value={tipInput} inputmode="decimal" />
          <p class="text-xs text-surface-400">
            ≈ {(tipPercent || 0).toFixed(2)}% of items subtotal
          </p>
        </label>
        <div class="flex flex-wrap gap-2">
          {#each [15, 18, 20] as pct}
            <button
              class="action-btn action-btn-surface action-btn-compact"
              type="button"
              on:click={() => setTipPercent(pct)}
            >
              {pct}%
            </button>
          {/each}
        </div>
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showTaxTipModal = false)}>Cancel</button>
          <button
            class="btn btn-primary w-full"
            on:click={() => {
              const factor = factorFor(roomCurrency);
              const tax = Math.round((Number.parseFloat(taxInput || '0') || 0) * factor);
              const tip = Math.round((Number.parseFloat(tipInput || '0') || 0) * factor);
              const payload = { tax_cents: tax, tip_cents: tip };
              sendOp({ kind: 'set_tax_tip', actor_id: identity.userId, payload });
              applyLocalOp({ kind: 'set_tax_tip', payload });
              showTaxTipModal = false;
            }}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showSummary && summaryData}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white max-h-[80vh] overflow-y-auto">
        <div class="flex items-center justify-between gap-3 flex-wrap">
          <h3 class="text-lg font-semibold">Summary</h3>
          <div class="flex items-center gap-2">
            <span class="text-sm text-white/70">Summary in:</span>
            <select
              class="input bg-white/5 border border-white/15 rounded-lg px-3 py-2 text-white text-sm"
              bind:value={targetCurrency}
              on:change={async (e) => {
                changeTargetCurrency((e.target as HTMLSelectElement).value);
                await buildSummary();
              }}
            >
              {#each COMMON_CURRENCIES as c}
                <option value={c.code}>{c.flag} {c.code} {c.symbol}</option>
              {/each}
            </select>
          </div>
        </div>
        <div class="text-sm text-surface-200 space-y-1">
          <p>Gross: {formatAmount(summaryData.gross)}</p>
          <p>Discounts: -{formatAmount(summaryData.discount)}</p>
          <p>Net: {formatAmount(summaryData.net)}</p>
          <p>Tax: {formatAmount(summaryData.tax)}</p>
          <p>Tip: {formatAmount(summaryData.tip)}</p>
          <p class="font-semibold text-white">Total: {formatAmount(summaryData.total)}</p>
        </div>
        {#if summaryData.converted}
          <div class="text-sm text-primary-200 space-y-1 border border-primary-500/40 rounded-xl p-3 bg-primary-500/10">
            <div class="flex justify-between items-center">
              <span class="font-semibold">Converted to {summaryData.converted.currency}</span>
              <span class="text-xs opacity-80">rate {summaryData.converted.rate?.toFixed(4)}{summaryData.converted.asOf ? ` · as of ${summaryData.converted.asOf}` : ''}</span>
            </div>
            <p>Gross: {formatCurrency(summaryData.converted.gross, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</p>
            <p>Discounts: -{formatCurrency(summaryData.converted.discount, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</p>
            <p>Net: {formatCurrency(summaryData.converted.net, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</p>
            <p>Tax: {formatCurrency(summaryData.converted.tax, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</p>
            <p>Tip: {formatCurrency(summaryData.converted.tip, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</p>
            <p class="font-semibold text-white">Total: {formatCurrency(summaryData.converted.total, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</p>
          </div>
        {/if}
        <div class="space-y-3">
          {#each summaryData.perPerson as person}
            <div class="border border-surface-800 rounded-xl p-3 space-y-2">
              <div class="flex items-center gap-2">
                <span class="w-3 h-3 rounded-full" style={`background:${person.color};`}></span>
                <p class="font-semibold">{person.name}</p>
              </div>
              <div class="space-y-1 text-sm text-surface-200">
                {#each person.items as item}
                  <div class="flex justify-between">
                    <span>{item.name}</span>
                    <span>{formatAmount(item.share_cents)}</span>
                  </div>
                {/each}
                <div class="flex justify-between font-semibold text-white">
                  <span>Items subtotal</span>
                  <span>{formatAmount(person.itemsTotal)}</span>
                </div>
                <div class="flex justify-between">
                  <span>Tax share</span>
                  <span>{formatAmount(person.taxShare)}</span>
                </div>
                <div class="flex justify-between">
                  <span>Tip share</span>
                  <span>{formatAmount(person.tipShare)}</span>
                </div>
                <div class="flex justify-between font-semibold text-white">
                  <span>Total</span>
                  <span>{formatAmount(person.total)}</span>
                </div>
                {#if summaryData.converted}
                  {#each summaryData.converted.perPerson.filter((p) => p.id === person.id) as conv}
                    <div class="flex justify-between text-primary-200 font-semibold">
                      <span>Total ({summaryData.converted.currency})</span>
                      <span>{formatCurrency(conv.total, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                    </div>
                  {/each}
                {/if}
              </div>
            </div>
          {/each}
        </div>
        <button class="btn btn-primary w-full" on:click={() => (showSummary = false)}>Close</button>
      </div>
    </div>
  {/if}

  {#if showNameModal}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Change name</h3>
        <input class="input w-full" bind:value={nameInput} placeholder="Your name" />
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showNameModal = false)}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={() => { sendParticipantUpdate(identity.userId, nameInput, true); identity.name = nameInput.trim(); identity.initials = nameInput.trim().slice(0,2).toUpperCase(); localStorage.setItem(`room:${roomCode}:identity`, JSON.stringify(identity)); showNameModal = false; }}>
            Save
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showRoomNameModal}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Rename restaurant</h3>
        <input class="input w-full" bind:value={roomNameInput} placeholder="Restaurant name" />
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showRoomNameModal = false)}>Cancel</button>
          <button
            class="btn btn-primary w-full"
            on:click={() => {
              sendRoomNameUpdate(roomNameInput);
              showRoomNameModal = false;
            }}
            disabled={!roomNameInput.trim()}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showAddPersonModal}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">Add person</h3>
        <input class="input w-full" bind:value={addPersonName} placeholder="Name" />
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => (showAddPersonModal = false)}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={() => {
            const name = addPersonName.trim();
            if (!name) return;
            const existing = Object.entries(room?.participants || {}).find(([, p]) => (p as Participant).name.trim().toLowerCase() === name.toLowerCase());
            const id = existing ? existing[0] : `guest-${Date.now()}`;
            sendParticipantUpdate(id, name, false);
            showAddPersonModal = false;
          }}>Add</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showJoinPrompt}
    <div class="fixed inset-0 bg-black/60 flex items-end justify-center z-50">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold text-center">Join this room</h3>
        <p class="text-center text-surface-200 text-sm">Enter your name to join room {roomCode?.toUpperCase()}.</p>
        <input class="input w-full" bind:value={joinNameInput} placeholder="Your name" />
        {#if joinError}
          <div class="text-error-300 text-sm">{joinError}</div>
        {/if}
        <div class="flex gap-3">
          <button class="btn btn-outline w-full" on:click={() => history.back()}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={joinRoomWithName} disabled={!joinNameInput.trim()}>Join</button>
        </div>
      </div>
    </div>
  {/if}
</div>
