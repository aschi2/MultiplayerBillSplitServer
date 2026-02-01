<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import Avatar from '$lib/components/Avatar.svelte';
  import { formatCents, initialsFromName } from '$lib/utils';
  import type { RoomDoc, ReceiptParseResult, Item, Participant } from '$lib/types';
  import { getApiBase, getWsBase } from '$lib/api';

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
  let receiptError: string | null = null;
  let receiptUploading = false;
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
  let reconnectDelay = RECONNECT_MIN;
  let resyncInterval: ReturnType<typeof setInterval> | null = null;
  let heartbeatInterval: ReturnType<typeof setInterval> | null = null;
  let connectionCheckInterval: ReturnType<typeof setInterval> | null = null;
  let lastMessageAt = Date.now();
  let lastPingAt = 0;
  let missedPongs = 0;
  let currentSeq = 0;
  let isConnecting = false;
  let forceClose = false;
  const pendingOps: any[] = [];

  const safeClose = () => {
    try {
      ws?.close();
    } catch {
      // ignore
    }
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
    const clean = input.replace(/[^a-fA-F0-9]/g, '');
    if (clean.length >= 6) return clean.slice(0, 6).toLowerCase();
    let hash = 0;
    for (let i = 0; i < input.length; i++) {
      hash = (hash * 31 + input.charCodeAt(i)) >>> 0;
    }
    return hash.toString(16).padStart(6, '0').slice(0, 6);
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
    if (connectionCheckInterval) {
      clearInterval(connectionCheckInterval);
      connectionCheckInterval = null;
    }
    if (ws) {
      try {
        ws.close();
      } catch {
        // ignore
      }
      ws = null;
    }
    forceClose = false;

    wsStatus = 'connecting';
    ws = new WebSocket(`${wsBase}/${roomCode}`);
    ws.onopen = () => {
      isConnecting = false;
      reconnectDelay = RECONNECT_MIN;
      wsStatus = 'connected';
      lastMessageAt = Date.now();
      lastPingAt = 0;
      missedPongs = 0;
      requestSnapshot();
      resyncInterval = setInterval(requestSnapshot, 60000);
      heartbeatInterval = setInterval(() => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ping' }));
          lastPingAt = Date.now();
          missedPongs = Math.min(missedPongs + 1, 100);
        }
      }, 5000);
      connectionCheckInterval = setInterval(() => {
        if (!ws) return;
        const state = ws.readyState;
        if (state !== WebSocket.OPEN || missedPongs >= 6) {
          if (reconnectTimer) clearTimeout(reconnectTimer);
          reconnectDelay = RECONNECT_MIN;
          safeClose();
        }
      }, 10000);
      while (pendingOps.length && ws && ws.readyState === WebSocket.OPEN) {
        const next = pendingOps.shift();
        ws.send(JSON.stringify({ type: 'op', op: next }));
      }
      // ensure presence marked online on fresh connection
      sendPresence(true);
    };
    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      lastMessageAt = Date.now();
      if (message.type === 'pong') {
        missedPongs = 0;
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
      isConnecting = false;
      sendPresence(false);
      if (resyncInterval) {
        clearInterval(resyncInterval);
        resyncInterval = null;
      }
      if (!forceClose) {
        scheduleReconnect();
      } else {
        wsStatus = 'disconnected';
      }
    };
    ws.onerror = () => {
      isConnecting = false;
      if (!forceClose) scheduleReconnect();
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

    // per-person accumulators
    const perPerson = new Map<
      string,
      { name: string; color: string; items: { name: string; share_cents: number }[]; itemsTotal: number }
    >();

    itemsArr.forEach((it) => {
      const assignees = Object.entries(it.assigned || {}).filter(([, on]) => on).map(([uid]) => uid);
      const shareCount = assignees.length || 1; // if nobody assigned, treat as unassigned to nobody; still split 1 to avoid div0
      const netLine = Math.max(0, it.line_price_cents - it.discount_cents * (it.quantity || 1));
      const share = Math.round(netLine / shareCount);
      assignees.forEach((uid) => {
        const p = participants[uid];
        const entry = perPerson.get(uid) || {
          name: p?.name || uid,
          color: colorHex(p?.colorSeed),
          items: [],
          itemsTotal: 0
        };
        entry.items.push({ name: it.name, share_cents: share });
        entry.itemsTotal += share;
        perPerson.set(uid, entry);
      });
    });

    const totalItems = Array.from(perPerson.values()).reduce((s, p) => s + p.itemsTotal, 0) || 1;

    const detailed = Array.from(perPerson.entries()).map(([uid, person]) => {
      const ratio = person.itemsTotal / totalItems;
      const taxShare = Math.round(tax * ratio);
      const tipShare = Math.round(tip * ratio);
      const totalShare = person.itemsTotal + taxShare + tipShare;
      return { id: uid, ...person, taxShare, tipShare, total: totalShare };
    });

    const total = net + tax + tip;
    return { gross, discount, net, tax, tip, total, perPerson: detailed };
  };

  const submitReceipt = async (event: Event) => {
    const target = event.target as HTMLInputElement;
    if (!target.files?.[0]) return;
    receiptError = null;
    receiptUploading = true;
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
      warningBanner = result.warnings?.length > 0;
      const agg = new Map<
        string,
        { name: string; qty: number; unit: number; discount: number; discountPct: number; line: number }
      >();
      result.items.forEach((item) => {
        const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
        let unit = item.unit_price_cents != null ? item.unit_price_cents / 100 : 0;
        let line = item.line_price_cents != null ? item.line_price_cents / 100 : 0;
        if (!unit && line && qty > 0) unit = line / qty;
        if (!line && unit && qty > 0) line = unit * qty;
        const disc = item.discount_cents != null ? item.discount_cents / 100 : 0;
        const discPct = item.discount_percent != null ? item.discount_percent : unit ? (disc / unit) * 100 : 0;
        const key = `${(item.name || 'Item').trim().toLowerCase()}|${unit.toFixed(2)}|${disc.toFixed(2)}`;
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
        unitPrice: item.unit ? item.unit.toFixed(2) : '',
        linePrice: item.line ? item.line.toFixed(2) : '',
        discountCents: item.discount ? item.discount.toFixed(2) : '',
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

      const fmt = (v: number) => (Number.isFinite(v) && v !== 0 ? v.toFixed(2) : '');
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
    const toCents = (val: string) => {
      const num = Number.parseFloat(val);
      if (!Number.isFinite(num)) return 0;
      return Math.round(num * 100);
    };
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
    if (receiptResult?.tax_cents != null && ws && room) {
      const tax = receiptResult.tax_cents;
      const tip = room.tip_cents || 0;
      const payload = { tax_cents: tax, tip_cents: tip };
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

    const fmt = (v: number) => (Number.isFinite(v) && v !== 0 ? v.toFixed(2) : '');
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

  const toCents = (val: string) => Math.round((Number.parseFloat(val || '0') || 0) * 100);
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
  $: taxCentsPreview = toCents(
    taxInput || (room?.tax_cents ? (room.tax_cents / 100).toFixed(2) : '0')
  );
  $: tipCentsPreview = toCents(
    tipInput || (room?.tip_cents ? (room.tip_cents / 100).toFixed(2) : '0')
  );
  $: taxPercent = preTaxSubtotalCents > 0 ? (taxCentsPreview / preTaxSubtotalCents) * 100 : 0;
  $: tipPercent = preTaxSubtotalCents > 0 ? (tipCentsPreview / preTaxSubtotalCents) * 100 : 0;

  const setTipPercent = (pct: number) => {
    const base = preTaxSubtotalCents || 0;
    const tip = Math.round((base * pct) / 100);
    tipInput = (tip / 100).toFixed(2);
    tipCentsPreview = tip;
    tipPercent = base > 0 ? (tip / base) * 100 : 0;
  };
</script>

<div class="min-h-screen bg-surface-900 text-surface-50 pb-24">
  {#if wsStatus !== 'connected'}
    <div class="px-6 pt-3">
      <div class="flex items-center gap-3 rounded-xl border border-warning-500/40 bg-warning-500/10 text-warning-100 text-xs px-3 py-2">
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
        <p class="text-sm text-white/70">Room</p>
        <h1 class="text-2xl font-semibold text-white">{room?.name || 'Shared Bill'}</h1>
        <div class="flex items-center justify-center gap-3 mt-2 text-sm text-white/80 flex-wrap">
          <span class="rounded-full bg-black/20 px-3 py-1 font-mono">Room code: {roomCode?.toUpperCase()}</span>
          {#if qrUrl}
            <img src={qrUrl} alt="Join QR code" class="w-16 h-16 rounded-lg border border-white/20 bg-white/5" />
          {/if}
        </div>
      </div>
      <div class="flex flex-col items-center justify-center order-2 md:order-3 md:w-48">
        <Avatar initials={identity.initials} color={colorHex(identity.colorSeed)} size={60} />
        <div class="text-sm text-white mt-2">{identity.name || 'You'}</div>
        <div class="flex items-center justify-center gap-2 mt-3 w-full">
          <button class="action-btn action-btn-surface action-btn-compact text-xs sm:text-sm flex-1" on:click={() => { nameInput = identity.name; showNameModal = true; }}>
            ✏️<span class="inline ml-1">Change name</span>
          </button>
        </div>
        <div class="flex items-center justify-center gap-2 mt-2 w-full">
          <button class="action-btn action-btn-surface action-btn-compact text-xs sm:text-sm flex-1" on:click={() => { roomNameInput = room?.name || ''; showRoomNameModal = true; }}>
            🏷️<span class="inline ml-1">Rename room</span>
          </button>
          <button class="action-btn action-btn-primary action-btn-compact text-xs sm:text-sm flex-1" on:click={() => { addPersonName = ''; showAddPersonModal = true; }}>
            ➕<span class="inline ml-1">Add person</span>
          </button>
        </div>
      </div>
    </div>
    <div class="flex gap-3 overflow-x-auto pt-2 justify-center">
      {#if room}
        {#each participants as participant}
          <div class="flex flex-col items-center text-xs relative w-14 shrink-0">
            <Avatar initials={participant.initials} color={colorHex(participant.colorSeed)} size={36} />
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
                🗑
              </button>
            {/if}
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
    {#if receiptError}
      <div class="rounded-xl bg-error-500/20 text-error-200 px-4 py-3 text-sm border border-error-500/40">
        {receiptError}
      </div>
    {/if}

    <section class="space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Items</h2>
        {#if room && Object.keys(room.items).length === 0}
          <label class={`action-btn action-btn-surface action-btn-compact ${receiptUploading ? 'opacity-60 pointer-events-none' : ''}`}>
            📷<span class="hidden sm:inline ml-1">{receiptUploading ? 'Uploading...' : 'Upload receipt'}</span>
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
              <p class="text-sm text-surface-200 whitespace-nowrap">{formatCents(item.line_price_cents)}</p>
              {#if item.discount_cents}
                <p class="text-xs text-surface-300">
                  Discount: {formatCents(item.discount_cents)} · Net: {formatCents(item.line_price_cents - item.discount_cents)}
                </p>
              {/if}
              <p class="text-xs text-surface-400 flex flex-wrap items-center gap-1">
                Assigned:
                {#if room}
                  {#each Object.entries(item.assigned || {}) as [uid, on]}
                    {#if on}
                      <span class="badge" style={`background:${colorHex(room.participants?.[uid]?.colorSeed)}; color:white;`}>
                        {room.participants?.[uid]?.name || uid}
                      </span>
                    {/if}
                  {/each}
                {/if}
              </p>
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
                  👥<span class="hidden sm:inline ml-1">Assign</span>
                </button>
                <button
                  class="action-btn action-btn-primary action-btn-compact"
                  title={item.assigned?.[identity.userId] ? 'Unassign me' : 'Assign to me'}
                  type="button"
                  on:click|stopPropagation={() => toggleAssign(item.id, identity.userId)}
                >
                  {item.assigned?.[identity.userId] ? '🚫' : '✅'}
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
                  ✏️<span class="hidden sm:inline ml-1">Edit</span>
                </button>
                <button
                  class="action-btn action-btn-surface action-btn-compact"
                  title="Copy"
                  type="button"
                  on:click|stopPropagation={() => duplicateItem(item)}
                >
                  📄<span class="hidden sm:inline ml-1">Copy</span>
                </button>
                <button
                  class="action-btn action-btn-danger action-btn-compact"
                  title="Delete"
                  type="button"
                  on:click|stopPropagation={() => removeItem(item.id)}
                >
                  🗑<span class="hidden sm:inline ml-1">Delete</span>
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
    <button class="btn btn-outline" on:click={() => { showTaxTipModal = true; taxInput = room?.tax_cents ? (room.tax_cents/100).toFixed(2) : ''; tipInput = room?.tip_cents ? (room.tip_cents/100).toFixed(2) : ''; }}>Tax/Tip</button>
    <button class="btn btn-outline" on:click={() => { summaryData = computeSummary(); showSummary = true; }}>Summary</button>
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
                <Avatar initials={participant.initials} color={colorHex(participant.colorSeed)} size={32} />
                <span>{participant.name}</span>
              </div>
              {#if room?.items?.[activeItemId]}
                {#if room.items[activeItemId].assigned?.[participant.id]}
                  <span class="text-xs px-2 py-1 rounded-full bg-primary-500/20 text-primary-100 border border-primary-400/40">🚫 Unassign</span>
                {:else}
                  <span class="text-xs px-2 py-1 rounded-full bg-surface-800 text-surface-200 border border-surface-700">✅ Assign</span>
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
        {#each editableItems as item, index}
          <div class="border border-surface-800 rounded-xl p-4 space-y-3">
            <label class="block space-y-1">
              <span class="text-xs text-surface-300">Name</span>
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
               <span class="text-xs text-surface-300">Unit price ($)</span>
               <input
                 class="input w-full"
                 type="number"
                 min="0"
                 step="0.01"
                 bind:value={editableItems[index].unitPrice}
                 on:input={() => recalcDerived(index, 'unitPrice')}
               />
             </label>
           </div>
            <div class="grid grid-cols-2 gap-3">
              <label class="block space-y-1">
                <span class="text-xs text-surface-300">Gross line ($)</span>
                <input
                  class="input w-full"
                  type="number"
                  min="0"
                  step="0.01"
                  bind:value={editableItems[index].linePrice}
                  on:input={() => recalcDerived(index, 'linePrice')}
                />
              </label>
              <label class="block space-y-1">
                <span class="text-xs text-surface-300">Discount ($ per unit)</span>
                <input
                  class="input w-full"
                  type="number"
                  min="0"
                  step="0.01"
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
          </div>
        {/each}
        <button class="btn btn-primary w-full" on:click={confirmReceipt}>Import Items</button>
        <button class="btn btn-outline w-full" on:click={() => (showReceiptReview = false)}>Cancel</button>
      </div>
    </div>
  {/if}

  {#if showItemModal}
    <div class="fixed inset-0 bg-black/40 flex items-end justify-center">
      <div class="glass-card w-full rounded-t-3xl p-6 space-y-4 text-white">
        <h3 class="text-lg font-semibold">{itemModalMode === 'edit' ? 'Edit item' : 'Add item'}</h3>
        <label class="block">
          <span class="text-sm text-surface-200">Item name</span>
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
            <span class="text-sm text-surface-200">Unit price ($)</span>
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
              <span class="text-sm text-surface-200">Line price ($)</span>
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
            <span class="text-sm text-surface-200">Discount ($)</span>
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
          Items subtotal: {preTaxSubtotalCents ? formatCents(preTaxSubtotalCents) : '$0.00'}
        </p>
        <label class="block space-y-1">
          <span class="text-sm text-surface-200">Tax ($)</span>
          <input class="input w-full" bind:value={taxInput} inputmode="decimal" />
          <p class="text-xs text-surface-400">
            ≈ {(taxPercent || 0).toFixed(2)}% of items subtotal
          </p>
        </label>
        <label class="block space-y-1">
          <span class="text-sm text-surface-200">Tip ($)</span>
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
              const tax = Math.round((Number.parseFloat(taxInput || '0') || 0) * 100);
              const tip = Math.round((Number.parseFloat(tipInput || '0') || 0) * 100);
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
        <h3 class="text-lg font-semibold">Summary</h3>
        <div class="text-sm text-surface-200 space-y-1">
          <p>Gross: {formatCents(summaryData.gross)}</p>
          <p>Discounts: -{formatCents(summaryData.discount)}</p>
          <p>Net: {formatCents(summaryData.net)}</p>
          <p>Tax: {formatCents(summaryData.tax)}</p>
          <p>Tip: {formatCents(summaryData.tip)}</p>
          <p class="font-semibold text-white">Total: {formatCents(summaryData.total)}</p>
        </div>
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
                    <span>{formatCents(item.share_cents)}</span>
                  </div>
                {/each}
                <div class="flex justify-between font-semibold text-white">
                  <span>Items subtotal</span>
                  <span>{formatCents(person.itemsTotal)}</span>
                </div>
                <div class="flex justify-between">
                  <span>Tax share</span>
                  <span>{formatCents(person.taxShare)}</span>
                </div>
                <div class="flex justify-between">
                  <span>Tip share</span>
                  <span>{formatCents(person.tipShare)}</span>
                </div>
                <div class="flex justify-between font-semibold text-white">
                  <span>Total</span>
                  <span>{formatCents(person.total)}</span>
                </div>
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
        <h3 class="text-lg font-semibold">Rename room</h3>
        <input class="input w-full" bind:value={roomNameInput} placeholder="Room name" />
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
