<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { browser } from '$app/environment';
  import Avatar from '$lib/components/Avatar.svelte';
  import ItemEditorFields from '$lib/components/ItemEditorFields.svelte';
  import ReceiptCropModal from '$lib/components/ReceiptCropModal.svelte';
  import ContactsModal from '$lib/components/ContactsModal.svelte';
  import { upsertBillHistoryEntry } from '$lib/billHistory';
  import { formatCurrency, initialsFromName } from '$lib/utils';
  import { loadIdentityPrefs, saveIdentityPrefs } from '$lib/identityPrefs';
  import {
    loadContacts,
    trackRecentPeople,
    touchContact,
    migrateFromFriendGroups,
    type Contact
  } from '$lib/contacts';
  import type {
    RoomDoc,
    ReceiptParseResult,
    ReceiptItem,
    ReceiptAddon,
    Item,
    Participant,
    ItemMeta,
    ItemAddonMeta
  } from '$lib/types';
  import { getApiBase, getWsBase } from '$lib/api';
  import { COMMON_CURRENCIES, DEFAULT_CURRENCY, EXPONENTS, SYMBOLS, FLAGS } from '$lib/currency';

  export let data;
  let roomCode = data.roomCode as string;
  let ws: WebSocket | null = null;
  let room: RoomDoc | null = null;
  let identity = { userId: '', name: '', initials: '', colorSeed: '', venmoUsername: '' };
  let showAssign = false;
  let activeItemId: string | null = null;
  let receiptResult: ReceiptParseResult | null = null;
  let showReceiptReview = false;
  let showReceiptItemEditModal = false;
  let showReceiptItemAddonsModal = false;
  let receiptEditingIndex: number | null = null;
  let receiptItemAddonsIndex: number | null = null;
  let showReceiptAttachModal = false;
  let receiptAttachSourceIndex: number | null = null;
  let warningBanner = false;
  let receiptWarnings: string[] = [];
  let receiptError: string | null = null;
  let receiptUploading = false;
  let receiptTryingAgain = false;
  let receiptRetryStatus: string | null = null;
  let receiptFileInputEl: HTMLInputElement | null = null;
  let receiptLastUploadedFile: File | null = null;
  let receiptLastUploadedCropped = false;
  let showReceiptCropModal = false;
  let receiptCropSourceFile: File | null = null;
  let parsedTaxInput = '';
  let parsedTipInput = '';
  let parsedBillDiscountInput = '';
  let parsedBillChargesInput = '';
  let receiptCurrencySelection: string = DEFAULT_CURRENCY;
  let parsedTaxCents = 0;
  let parsedTipCents = 0;
  let parsedBillDiscountCents = 0;
  let parsedBillChargesCents = 0;
  let receiptSubtotalCents = 0;
  let receiptGrossSubtotalCents = 0;
  let receiptImportedTotalCents = 0;
  let receiptTaxPercent = 0;
  let receiptTipPercent = 0;
  let showItemModal = false;
  let showItemAddonsModal = false;
  let showRepeatedGroupSheet = false;
  let repeatedGroupSheetKey: string | null = null;
  let itemModalMode: 'new' | 'edit' | 'group' = 'new';
  let itemModalId: string | null = null;
  let itemModalGroupKey: string | null = null;
  type ItemFormAddon = {
    name: string;
    price: string;
  };
  type ItemFormState = {
    name: string;
    quantity: string;
    unitPrice: string;
    linePrice: string;
    discountCents: string;
    discountPercent: string;
    addons: ItemFormAddon[];
  };
  type RepeatedGroup = {
    key: string;
    baseName: string;
    items: Item[];
    addons: ItemAddonMeta[];
    sortOrder: number;
    totalLinePriceCents: number;
    totalDiscountCents: number;
    totalNetPriceCents: number;
  };
  type ItemListEntry =
    | { kind: 'item'; sortOrder: number; item: Item }
    | { kind: 'group'; sortOrder: number; group: RepeatedGroup };
  let itemForm: ItemFormState = {
    name: '',
    quantity: '1',
    unitPrice: '0',
    linePrice: '0',
    discountCents: '0',
    discountPercent: '0',
    addons: []
  };
  let itemFormDiscountMode: 'amount' | 'percent' = 'amount';
  let itemFormTotalInputMode: 'auto' | 'manual' = 'auto';
  let itemFormAddonTotalPreviewCents = 0;
  let itemFormAddonExtendedTotalPreviewCents = 0;
  let itemFormPricingPreview = {
    qty: 1,
    unit: 0,
    baseTotal: 0,
    addonPerItem: 0,
    addonTotal: 0,
    grossTotal: 0
  };
  let itemFormNetPreview = { netUnit: 0, netTotal: 0 };
  let showBillSettingsModal = false;
  let taxInput = '';
  let tipInput = '';
  let billDiscountInput = '';
  let billChargesInput = '';
  let showSummary = false;
  let summaryExporting = false;
  let summaryExportError: string | null = null;
  let summaryUnsentOnly = false;
  let summaryVenmoSentByPersonId: Record<string, { total: number; sentAt: number }> = {};
  let summaryChargeQueue: Array<{ id: string; venmoUsername: string; total: number }> = [];
  let summaryChargeQueueActive = false;
  let summaryChargeQueuePendingResume = false;
  let summaryUnsentRequestableCount = 0;
  let venmoLaunchCooldownUntil = 0;
  let summaryData: {
    gross: number;
    itemDiscount: number;
    billDiscount: number;
    discount: number;
    net: number;
    billCharges: number;
    tax: number;
    tip: number;
    totalBeforeTip: number;
    total: number;
    converted?: {
      gross: number;
      itemDiscount: number;
      billDiscount: number;
      discount: number;
      net: number;
      billCharges: number;
      tax: number;
      tip: number;
      totalBeforeTip: number;
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
      venmoUsername: string;
      items: {
        item_id: string;
        name: string;
        share_cents: number;
        fraction_numerator: number;
        fraction_denominator: number;
      }[];
      grossItemsTotal: number;
      itemsTotal: number;
      billDiscountShare: number;
      billChargesShare: number;
      taxShare: number;
      tipShare: number;
      total: number;
    }[];
  } | null = null;
  let showNameModal = false;
  let showRoomNameModal = false;
  let roomNameInput = '';
  let nameInput = '';
  let venmoInput = '';
  let showAddPersonModal = false;
  let addPersonName = '';
  let addPersonVenmoInput = '';
  let showJoinPrompt = false;
  let joinNameInput = '';
  let joinVenmoInput = '';
  let joinError: string | null = null;
  let bulkAssignMode = false;
  let bulkAssignSelectedByItemId: Record<string, boolean> = {};
  let bulkAssignTargetParticipantId = '';
  let contacts: Contact[] = [];
  let showContactsModal = false;
  let addPersonSelectedContactIds: Set<string> = new Set();
  let addPersonSearchFilter = '';
  type EditableItemReviewFlags = {
    name: boolean;
    quantity: boolean;
    unitPrice: boolean;
    linePrice: boolean;
    discount: boolean;
    addons: boolean;
    reasons: string[];
  };
  let editableItemReviewFlags: EditableItemReviewFlags[] = [];
  let showReceiptFlaggedOnly = false;
  let receiptReviewFocusIndex = 0;
  let preTaxGrossSubtotalCents = 0;
  let preTaxNetSubtotalCents = 0;
  let taxCentsPreview = 0;
  let tipCentsPreview = 0;
  let billDiscountCentsPreview = 0;
  let billChargesCentsPreview = 0;
  let billSettingsDirty = false;
  let billSettingsInitialized = false;
  let taxPercent = 0;
  let tipPercent = 0;
  let billDiscountPercent = 0;
  let billChargesPercent = 0;
  let wsStatus: 'connecting' | 'connected' | 'reconnecting' | 'disconnected' = 'connecting';
  let roomCurrency: string = DEFAULT_CURRENCY;
  let targetCurrency: string = DEFAULT_CURRENCY;
  let detectedCurrency: string | null = null;
  let fxRate: number | null = null;
  let fxAsOf: string | null = null;
  let venmoUsdRate: number | null = null;
  let venmoUsdRateBase: string = '';
  type EditableItem = {
    name: string;
    quantity: string;
    unitPrice: string;
    linePrice: string;
    discountCents: string;
    discountPercent: string;
    addons?: ItemFormAddon[];
    discountMode?: 'amount' | 'percent';
    totalInputMode?: 'auto' | 'manual';
  };
  type ReceiptReviewSnapshot = {
    receiptResult: ReceiptParseResult | null;
    editableItems: EditableItem[];
    editableItemReviewFlags: EditableItemReviewFlags[];
    receiptWarnings: string[];
    warningBanner: boolean;
    detectedCurrency: string | null;
    receiptCurrencySelection: string;
    parsedTaxInput: string;
    parsedTipInput: string;
    parsedBillDiscountInput: string;
    parsedBillChargesInput: string;
    showReceiptFlaggedOnly: boolean;
    receiptReviewFocusIndex: number;
  };
  let editableItems: EditableItem[] = [];
  let receiptRetryConsumed = false;
  let receiptOriginalSnapshot: ReceiptReviewSnapshot | null = null;
  let receiptRetrySnapshot: ReceiptReviewSnapshot | null = null;
  let receiptUsingRetryResult = false;
  let items: Item[] = [];
  let itemEntries: ItemListEntry[] = [];
  let standaloneItems: Item[] = [];
  let repeatedGroupsByKey: Record<string, RepeatedGroup> = {};
  let activeRepeatedGroup: RepeatedGroup | null = null;
  let participants: Participant[] = [];
  let roomParticipantsForJoin: Participant[] = [];
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
  let qrUrlFullscreen = '';
  let showQrFullscreen = false;
  let billCodeShareFeedback = '';
  let billCodeShareFeedbackTimer: ReturnType<typeof setTimeout> | null = null;
  let joinPrefillName = '';
  let joinPrefillVenmo = '';
  let joinPrefillLocked = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  const RECONNECT_MIN = 500;
  const RECONNECT_MAX = 5000;
  const PING_INTERVAL = 5000;
  const PONG_TIMEOUT = 10000;
  const SORT_ORDER_STEP = 1000;
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
  let roomHistoryPersistTimer: ReturnType<typeof setTimeout> | null = null;
  let anyModalOpen = false;

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

  const buildQrDataUrl = async (text: string, size: number) => {
    try {
      const { toDataURL } = await import('qrcode');
      return await toDataURL(text, {
        width: size,
        margin: 1,
        errorCorrectionLevel: 'M',
        color: {
          dark: '#111111',
          light: '#ffffff'
        }
      });
    } catch {
      return `https://api.qrserver.com/v1/create-qr-code/?size=${size}x${size}&data=${encodeURIComponent(text)}`;
    }
  };

  // Keep more detail for dense/small-text receipts (journal/POS printouts).
  // 1080 was too aggressive and could blur key item lines before parsing.
  const RECEIPT_MAX_UPLOAD_DIMENSION = 1800;
  const DISCOUNT_LINE_KEYWORDS =
    /\b(discount|coupon|promo|off|savings|rebate|member|premier|loyalty|credit|voucher|adjustment|markdown)\b|%/i;
  const BILL_DISCOUNT_KEYWORDS =
    /\b(discount|coupon|promo|savings|rebate|member|premier|loyalty|credit|voucher|adjustment|markdown|order|bill|subtotal|total)\b/i;
  const BILL_CHARGE_KEYWORDS =
    /\b(service|admin|administrative|convenience|surcharge|processing|booking|facility|platform|charge|fee)\b/i;
  const ADDON_PREFIX_KEYWORDS = /^\s*(?:\+|add(?:\s+on)?\b|extra\b|with\b|w\/\b|topping\b)/i;

  const normalizeReceiptImage = async (file: File) => {
    const isHeic = file.type === 'image/heic' || file.type === 'image/heif';
    const url = URL.createObjectURL(file);
    try {
      const image = new Image();
      await new Promise<void>((resolve, reject) => {
        image.onload = () => resolve();
        image.onerror = () => reject(new Error('Failed to load image'));
        image.src = url;
      });

      const sourceWidth = image.naturalWidth || image.width;
      const sourceHeight = image.naturalHeight || image.height;
      if (!sourceWidth || !sourceHeight) return file;

      const maxSourceDimension = Math.max(sourceWidth, sourceHeight);
      const scale =
        maxSourceDimension > RECEIPT_MAX_UPLOAD_DIMENSION
          ? RECEIPT_MAX_UPLOAD_DIMENSION / maxSourceDimension
          : 1;
      const targetWidth = Math.max(1, Math.round(sourceWidth * scale));
      const targetHeight = Math.max(1, Math.round(sourceHeight * scale));

      const shouldResize = targetWidth !== sourceWidth || targetHeight !== sourceHeight;
      if (!shouldResize && !isHeic) {
        return file;
      }

      let currentCanvas = document.createElement('canvas');
      currentCanvas.width = sourceWidth;
      currentCanvas.height = sourceHeight;
      let currentCtx = currentCanvas.getContext('2d');
      if (!currentCtx) return file;
      currentCtx.drawImage(image, 0, 0);

      // Multi-step downsampling preserves fine text detail better than a single large resize.
      while (Math.max(currentCanvas.width, currentCanvas.height) > RECEIPT_MAX_UPLOAD_DIMENSION * 2) {
        const nextCanvas = document.createElement('canvas');
        nextCanvas.width = Math.max(targetWidth, Math.round(currentCanvas.width / 2));
        nextCanvas.height = Math.max(targetHeight, Math.round(currentCanvas.height / 2));
        const nextCtx = nextCanvas.getContext('2d');
        if (!nextCtx) break;
        nextCtx.imageSmoothingEnabled = true;
        nextCtx.imageSmoothingQuality = 'high';
        nextCtx.drawImage(currentCanvas, 0, 0, currentCanvas.width, currentCanvas.height, 0, 0, nextCanvas.width, nextCanvas.height);
        currentCanvas = nextCanvas;
      }

      if (currentCanvas.width !== targetWidth || currentCanvas.height !== targetHeight) {
        const finalCanvas = document.createElement('canvas');
        finalCanvas.width = targetWidth;
        finalCanvas.height = targetHeight;
        const finalCtx = finalCanvas.getContext('2d');
        if (!finalCtx) return file;
        finalCtx.imageSmoothingEnabled = true;
        finalCtx.imageSmoothingQuality = 'high';
        finalCtx.drawImage(currentCanvas, 0, 0, currentCanvas.width, currentCanvas.height, 0, 0, targetWidth, targetHeight);
        currentCanvas = finalCanvas;
      }

      const outputType =
        isHeic
          ? 'image/jpeg'
          : file.type === 'image/png'
            ? 'image/png'
            : file.type === 'image/webp'
              ? 'image/webp'
              : 'image/jpeg';
      const outputQuality = outputType === 'image/png' ? undefined : 0.98;
      const blob = await new Promise<Blob | null>((resolve) =>
        currentCanvas.toBlob(resolve, outputType, outputQuality)
      );
      if (!blob) return file;

      const baseName = file.name.replace(/\.\w+$/, '') || 'receipt';
      const extension =
        outputType === 'image/png' ? 'png' : outputType === 'image/webp' ? 'webp' : 'jpg';
      return new File([blob], `${baseName}.${extension}`, { type: outputType });
    } finally {
      URL.revokeObjectURL(url);
    }
  };

  const decodePayload = (payload: any) => (typeof payload === 'string' ? JSON.parse(payload) : payload);

  const parsedItemAdjustmentAmount = (item: ReceiptItem) => {
    const unit = Math.abs(Math.round(item.unit_price_cents ?? 0));
    const line = Math.abs(Math.round(item.line_price_cents ?? 0));
    const discount = Math.abs(Math.round(item.discount_cents ?? 0));
    return Math.max(unit, line, discount);
  };

  const isLikelyBillLevelDiscount = (item: ReceiptItem) => {
    const text = `${item.name ?? ''} ${item.raw_text ?? ''}`.toLowerCase();
    const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
    if (qty > 1.0001) return false;
    if (!DISCOUNT_LINE_KEYWORDS.test(text) && !/\b\d{1,2}\s*%/.test(text)) return false;
    // Keep pure "%/OFF" lines item-level by default unless explicit bill-level wording exists.
    if (!BILL_DISCOUNT_KEYWORDS.test(text)) return false;
    return parsedItemAdjustmentAmount(item) > 0;
  };

  const isLikelyBillLevelCharge = (item: ReceiptItem) => {
    const text = `${item.name ?? ''} ${item.raw_text ?? ''}`.toLowerCase();
    const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
    if (qty > 1.0001) return false;
    if (!BILL_CHARGE_KEYWORDS.test(text)) return false;
    if (isLikelyBillLevelDiscount(item)) return false;
    return parsedItemAdjustmentAmount(item) > 0;
  };

  const splitBillLevelAdjustmentsFromParsedItems = (parsedItems: ReceiptItem[]) => {
    const items: ReceiptItem[] = [];
    const inferredDiscountLines: string[] = [];
    const inferredChargeLines: string[] = [];
    let inferredBillDiscountCents = 0;
    let inferredBillChargesCents = 0;

    parsedItems.forEach((item, idx) => {
      const amount = parsedItemAdjustmentAmount(item);
      if (amount <= 0) {
        items.push(item);
        return;
      }

      if (isLikelyBillLevelDiscount(item)) {
        inferredBillDiscountCents += amount;
        inferredDiscountLines.push((item.name || item.raw_text || `line ${idx + 1}`).trim());
        return;
      }

      if (isLikelyBillLevelCharge(item)) {
        inferredBillChargesCents += amount;
        inferredChargeLines.push((item.name || item.raw_text || `line ${idx + 1}`).trim());
        return;
      }

      items.push(item);
    });

    return { items, inferredBillDiscountCents, inferredBillChargesCents, inferredDiscountLines, inferredChargeLines };
  };

  const parsePercentFromDiscountText = (text: string): number | null => {
    const match = text.match(/(\d{1,3}(?:[.,]\d+)?)\s*(?:[%％]|off\b|ｏｆｆ\b)/i);
    if (!match) return null;
    const normalized = match[1].replace(',', '.');
    const pct = Number.parseFloat(normalized);
    if (!Number.isFinite(pct) || pct <= 0 || pct > 100) return null;
    return pct;
  };

  const normalizedItemNameForMatching = (value: string | null | undefined) =>
    (value || '')
      .toLowerCase()
      .replace(/\b(?:off|sale|discount|coupon|promo|member|loyalty|割引|値引)\b/g, ' ')
      .replace(/[%％\d.,\-+()]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();

  const attachAdjacentItemDiscountLines = (parsedItems: ReceiptItem[]) => {
    const items: ReceiptItem[] = [];
    const attachedLines: string[] = [];

    parsedItems.forEach((item, idx) => {
      const amount = parsedItemAdjustmentAmount(item);
      const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
      const text = `${item.name ?? ''} ${item.raw_text ?? ''}`.toLowerCase();
      const percentFromText = parsePercentFromDiscountText(text);
      const hasPositiveGross =
        (item.line_price_cents != null && Math.round(item.line_price_cents) > 0) ||
        (item.unit_price_cents != null && Math.round(item.unit_price_cents) > 0);
      const looksLikeDiscountLine =
        qty <= 1.0001 &&
        amount > 0 &&
        (DISCOUNT_LINE_KEYWORDS.test(text) || percentFromText !== null) &&
        (!hasPositiveGross || (item.discount_cents != null && Math.round(item.discount_cents) > 0));

      if (!looksLikeDiscountLine || items.length === 0) {
        items.push(item);
        return;
      }

      const parent = items[items.length - 1];
      if (!parent) {
        items.push(item);
        return;
      }

      const parentQty = parent.quantity && parent.quantity > 0 ? parent.quantity : 1;
      const parentGross =
        parent.line_price_cents != null
          ? Math.max(0, Math.round(parent.line_price_cents))
          : parent.unit_price_cents != null
            ? Math.max(0, Math.round(parent.unit_price_cents * parentQty))
            : 0;
      if (parentGross <= 0 || amount > parentGross) {
        items.push(item);
        return;
      }

      let shouldAttach = false;
      if (percentFromText !== null) {
        const expected = Math.round((parentGross * percentFromText) / 100);
        const tolerance = Math.max(2, Math.round(expected * 0.06));
        shouldAttach = Math.abs(expected - amount) <= tolerance;
      }
      if (!shouldAttach) {
        const parentName = normalizedItemNameForMatching(parent.name || parent.raw_text || '');
        const discountName = normalizedItemNameForMatching(item.name || item.raw_text || '');
        if (parentName && discountName && (discountName.includes(parentName) || parentName.includes(discountName))) {
          const likelyMax = Math.round(parentGross * 0.8);
          shouldAttach = amount > 0 && amount <= likelyMax;
        }
      }
      if (!shouldAttach) {
        items.push(item);
        return;
      }

      const existingDiscountPerUnit = Math.max(0, Math.round(parent.discount_cents ?? 0));
      const addPerUnit = Math.max(0, Math.round(amount / parentQty));
      const nextDiscountPerUnit = existingDiscountPerUnit + addPerUnit;
      parent.discount_cents = nextDiscountPerUnit;
      if (parent.unit_price_cents && parent.unit_price_cents > 0) {
        parent.discount_percent = (nextDiscountPerUnit / parent.unit_price_cents) * 100;
      }

      attachedLines.push(
        `${(item.name || item.raw_text || `line ${idx + 1}`).trim()} -> ${(parent.name || parent.raw_text || `line ${idx}`).trim()}`
      );
    });

    return { items, attachedLines };
  };

  const normalizeAddonLabel = (value: string | null | undefined) =>
    normalizeReceiptLineLabel(
      (value || '').replace(/^\s*(?:\+|add(?:\s+on)?\b|extra\b|with\b|w\/\b)\s*/i, '')
    );

  const normalizeReceiptLineLabel = (value: string | null | undefined) => {
    let out = (value || '').replace(/\s+/g, ' ').trim();
    if (!out) return '';
    while (true) {
      const next = out
        .replace(/[-:;,.|]+$/g, '')
        .replace(/\s*[$€£¥₩₹]+\s*$/g, '')
        .replace(/[-:;,.|]+$/g, '')
        .trim();
      if (next === out) return next;
      out = next;
    }
  };

  const parsedLineAmountCents = (item: ReceiptItem) => {
    const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
    if (item.line_price_cents != null) return Math.max(0, Math.round(item.line_price_cents));
    if (item.unit_price_cents != null) return Math.max(0, Math.round(item.unit_price_cents * qty));
    return 0;
  };

  const isLikelyStandaloneAddonLine = (item: ReceiptItem) => {
    const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
    if (qty > 1.0001) return false;
    const baseText = `${item.name ?? ''}`.trim();
    const rawText = `${item.raw_text ?? ''}`.trim();
    const text = `${baseText} ${rawText}`.trim().toLowerCase();
    if (!text) return false;
    if (DISCOUNT_LINE_KEYWORDS.test(text) || BILL_CHARGE_KEYWORDS.test(text)) return false;
    if (isLikelyBillLevelDiscount(item) || isLikelyBillLevelCharge(item)) return false;
    if (item.discount_cents != null && Math.round(item.discount_cents) > 0) return false;
    if (!ADDON_PREFIX_KEYWORDS.test(baseText) && !ADDON_PREFIX_KEYWORDS.test(rawText)) return false;
    const amount = parsedLineAmountCents(item);
    if (amount > 0) return true;
    return item.unit_price_cents == null && item.line_price_cents == null;
  };

  const foldStandaloneAddonsIntoItems = (parsedItems: ReceiptItem[]) => {
    const items: ReceiptItem[] = [];
    const foldedLines: string[] = [];

    parsedItems.forEach((item, idx) => {
      const normalizedItem: ReceiptItem = {
        ...item,
        addons: Array.isArray(item.addons) ? [...item.addons] : []
      };

      if (!items.length || !isLikelyStandaloneAddonLine(normalizedItem)) {
        items.push(normalizedItem);
        return;
      }

      const parent = items[items.length - 1];
      if (!parent) {
        items.push(normalizedItem);
        return;
      }

      const addonLabel = normalizeAddonLabel(normalizedItem.name || normalizedItem.raw_text || '');
      const addonPrice = parsedLineAmountCents(normalizedItem);
      const addonRawText = (normalizedItem.raw_text || normalizedItem.name || '').trim() || null;
      const addonEntry: ReceiptAddon = {
        name: addonLabel || (normalizedItem.name || 'Addon').trim(),
        price_cents: addonPrice > 0 ? addonPrice : null,
        raw_text: addonRawText
      };
      parent.addons = [...(parent.addons || []), addonEntry];

      if (addonPrice > 0) {
        const parentQty = parent.quantity && parent.quantity > 0 ? parent.quantity : 1;
        const parentLine =
          parent.line_price_cents != null
            ? Math.max(0, Math.round(parent.line_price_cents))
            : parent.unit_price_cents != null
              ? Math.max(0, Math.round(parent.unit_price_cents * parentQty))
              : 0;
        const nextLine = parentLine + addonPrice;
        parent.line_price_cents = nextLine;
        if (parentQty > 0) {
          parent.unit_price_cents = Math.max(0, Math.round(nextLine / parentQty));
        }
      }

      foldedLines.push(
        `${addonLabel || normalizedItem.name || `line ${idx + 1}`} -> ${(parent.name || parent.raw_text || 'previous item').trim()}`
      );
    });

    return { items, foldedLines };
  };

  const splitNameAndAddonLabels = (name: string) => {
    const parts = (name || '')
      .split(/\s+\+\s+/)
      .map((part) => part.trim())
      .filter(Boolean);
    if (!parts.length) return { baseName: '', addonLabels: [] as string[] };
    if (parts.length === 1) return { baseName: parts[0], addonLabels: [] as string[] };
    return { baseName: parts[0], addonLabels: parts.slice(1) };
  };

  const numberedGeneratedItemName = (baseName: string, index: number, total: number) =>
    total > 1 ? `${baseName} #${index + 1}` : baseName;

  const stripGeneratedItemNumberSuffix = (name: string) => name.replace(/\s+#\d+\s*$/i, '').trim();

  const formAddonsFromItemMeta = (item: Item): ItemFormAddon[] => {
    const exp = exponentFor(roomCurrency);
    const factor = factorFor(roomCurrency);
    return itemAddonMeta(item)
      .map((addon) => {
        const name = normalizeAddonLabel(String(addon?.name || ''));
        const priceCents = Number.isFinite(Number(addon?.price_cents))
          ? Math.max(0, Math.round(Number(addon.price_cents)))
          : 0;
        return {
          name,
          price: priceCents > 0 ? (priceCents / factor).toFixed(exp) : ''
        };
      })
      .filter((addon) => addon.name || addon.price);
  };

  const normalizedFormAddons = (addons: ItemFormAddon[], code = roomCurrency) =>
    (addons || [])
      .map((addon) => {
        const name = normalizeAddonLabel(addon?.name || '');
        const priceCents = parseByCurrency(addon?.price || '', code);
        return {
          name: name || (priceCents > 0 ? 'Addon' : ''),
          price_cents: priceCents
        };
      })
      .filter((addon) => addon.name || addon.price_cents > 0);

  const addonsSignature = (addons: Array<{ name: string; price_cents: number }>) =>
    addons
      .map((addon) => `${(addon?.name || '').trim().toLowerCase()}:${Math.max(0, addon?.price_cents || 0)}`)
      .join('|');

  const itemDisplayParts = (item: Item) => {
    const parsed = splitNameAndAddonLabels(item?.name || '');
    const baseName = parsed.baseName || item?.name || 'Item';
    const metaAddons = itemAddonMeta(item);
    const fallbackAddons = parsed.addonLabels.map((label) => ({ name: label, price_cents: 0 }));
    return {
      baseName,
      addons: metaAddons.length > 0 ? metaAddons : fallbackAddons
    };
  };

  const repeatedGroupBaseName = (item: Item) =>
    normalizeReceiptLineLabel(stripGeneratedItemNumberSuffix(itemDisplayParts(item).baseName)) || 'Item';

  const itemNetPriceCents = (item: Item) =>
    Math.max(
      0,
      Math.round(
        Number(item?.line_price_cents || 0) -
          Math.max(0, Number(item?.discount_cents || 0) * Math.max(1, Number(item?.quantity || 1)))
      )
    );

  const repeatedGroupSignature = (item: Item) => {
    if (repeatedGroupExcluded(item)) return null;
    const baseName = repeatedGroupBaseName(item).trim().toLowerCase();
    if (!baseName) return null;
    return [
      baseName,
      itemNetPriceCents(item),
      Math.max(0, Math.round(Number(item?.discount_cents || 0))),
      addonsSignature(itemDisplayParts(item).addons)
    ].join('|');
  };

  const buildRepeatedGroups = (sortedItems: Item[]) => {
    const grouped = new Map<string, Item[]>();
    sortedItems.forEach((item) => {
      const key = repeatedGroupSignature(item);
      if (!key) return;
      const next = grouped.get(key) || [];
      next.push(item);
      grouped.set(key, next);
    });

    const groups: Record<string, RepeatedGroup> = {};
    grouped.forEach((groupItems, key) => {
      if (groupItems.length < 2) return;
      const orderedItems = sortItemsByOrder(groupItems);
      const first = orderedItems[0];
      groups[key] = {
        key,
        baseName: repeatedGroupBaseName(first),
        items: orderedItems,
        addons: itemDisplayParts(first).addons,
        sortOrder: itemSortOrderOr(first, Number.MAX_SAFE_INTEGER),
        totalLinePriceCents: orderedItems.reduce((sum, item) => sum + Math.max(0, Number(item.line_price_cents || 0)), 0),
        totalDiscountCents: orderedItems.reduce(
          (sum, item) => sum + Math.max(0, Number(item.discount_cents || 0) * Math.max(1, Number(item.quantity || 1))),
          0
        ),
        totalNetPriceCents: orderedItems.reduce((sum, item) => sum + itemNetPriceCents(item), 0)
      };
    });

    return groups;
  };

  const editableItemDisplayParts = (item: EditableItem) => {
    const parsed = splitNameAndAddonLabels(item?.name || '');
    const baseName = normalizeReceiptLineLabel(parsed.baseName || item?.name || '') || 'Item';
    const formAddons = normalizedFormAddons(item?.addons || [], receiptCurrencySelection);
    const fallbackAddons = parsed.addonLabels.map((label) => ({
      name: normalizeAddonLabel(label),
      price_cents: 0
    }));
    return {
      baseName,
      addons: formAddons.length > 0 ? formAddons : fallbackAddons.filter((addon) => addon.name || addon.price_cents > 0)
    };
  };

  const updateItemFormAddon = (index: number, field: 'name' | 'price', value: string) => {
    const addons = (itemForm.addons || []).map((addon, idx) =>
      idx === index ? { ...addon, [field]: value } : addon
    );
    itemForm = { ...itemForm, addons };
  };

  const addItemFormAddon = () => {
    itemForm = { ...itemForm, addons: [...(itemForm.addons || []), { name: '', price: '' }] };
  };

  const removeItemFormAddon = (index: number) => {
    itemForm = {
      ...itemForm,
      addons: (itemForm.addons || []).filter((_, idx) => idx !== index)
    };
  };

  $: itemFormAddonTotalPreviewCents = (itemForm.addons || []).reduce(
    (sum, addon) => sum + parseByCurrency(addon?.price || '', roomCurrency),
    0
  );
  $: itemFormAddonExtendedTotalPreviewCents = itemFormAddonExtendedTotalCents(
    itemForm,
    itemFormAddonTotalPreviewCents
  );
  $: itemFormPricingPreview = pricingPreviewFromItemForm(itemForm, itemFormAddonTotalPreviewCents);
  $: itemFormNetPreview = discountedUnitAndNetFromItemForm(itemForm, itemFormAddonTotalPreviewCents);

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

  const normalizeVenmoUsername = (value: string | null | undefined) =>
    (value || '')
      .trim()
      .replace(/^@+/, '')
      .replace(/\s+/g, '');

  const rememberIdentityPrefs = (name: string, venmoUsername: string | null | undefined) => {
    const normalizedName = (name || '').trim();
    const normalizedVenmo = normalizeVenmoUsername(venmoUsername || '');
    saveIdentityPrefs(normalizedName, normalizedVenmo);
    joinPrefillName = normalizedName;
    joinPrefillVenmo = normalizedVenmo;
  };

  const emptyEditableItemReviewFlags = (): EditableItemReviewFlags => ({
    name: false,
    quantity: false,
    unitPrice: false,
    linePrice: false,
    discount: false,
    addons: false,
    reasons: []
  });

  const reviewFlagsFromParsedItem = (item: ReceiptItem): EditableItemReviewFlags => {
    const flags = emptyEditableItemReviewFlags();
    const name = `${item?.name || ''}`.trim();
    const rawText = `${item?.raw_text || ''}`.trim();
    const qty = Number(item?.quantity ?? 0);
    const hasQty = Number.isFinite(qty) && qty > 0;
    const unit = Number(item?.unit_price_cents ?? NaN);
    const line = Number(item?.line_price_cents ?? NaN);
    const hasUnit = Number.isFinite(unit) && unit >= 0;
    const hasLine = Number.isFinite(line) && line >= 0;
    const discountCents = Number(item?.discount_cents ?? NaN);
    const discountPct = Number(item?.discount_percent ?? NaN);
    const hasDiscountCents = Number.isFinite(discountCents) && discountCents > 0;
    const hasDiscountPct = Number.isFinite(discountPct) && discountPct > 0;
    const addons = Array.isArray(item?.addons) ? item.addons : [];

    if (!name || /^item\b/i.test(name)) {
      flags.name = true;
      flags.reasons.push('Name may be incomplete');
    }

    if (!rawText) {
      flags.name = true;
      flags.reasons.push('Missing source line text');
    }

    if (!hasQty || Math.abs(qty - Math.round(qty)) > 0.0001) {
      flags.quantity = true;
      flags.reasons.push('Quantity may need review');
    }

    if (!hasUnit && !hasLine) {
      flags.unitPrice = true;
      flags.linePrice = true;
      flags.reasons.push('Missing price value');
    } else if (hasUnit && hasLine && hasQty) {
      const expectedLine = Math.round(Math.max(0, unit) * Math.max(1, Math.round(qty)));
      const tolerance = Math.max(2, Math.round(expectedLine * 0.06));
      if (Math.abs(expectedLine - Math.round(Math.max(0, line))) > tolerance) {
        flags.unitPrice = true;
        flags.linePrice = true;
        flags.reasons.push('Unit × quantity does not match total');
      }
    }

    if (hasDiscountPct && !hasDiscountCents && !hasUnit) {
      flags.discount = true;
      flags.reasons.push('Discount percent may not map cleanly');
    }

    if (hasDiscountCents && hasUnit && discountCents > unit) {
      flags.discount = true;
      flags.reasons.push('Discount is larger than unit price');
    }

    if (addons.length > 0) {
      const hasAddonGap = addons.some((addon) => {
        const addonName = `${addon?.name || ''}`.trim();
        const addonPrice = Number(addon?.price_cents ?? NaN);
        return !addonName || !Number.isFinite(addonPrice) || addonPrice < 0;
      });
      if (hasAddonGap) {
        flags.addons = true;
        flags.reasons.push('Add-on details may be incomplete');
      }
    }

    const uniqueReasons = Array.from(new Set(flags.reasons)).slice(0, 3);
    return { ...flags, reasons: uniqueReasons };
  };

  const mergeEditableItemReviewFlags = (
    left: EditableItemReviewFlags,
    right: EditableItemReviewFlags
  ): EditableItemReviewFlags => ({
    name: left.name || right.name,
    quantity: left.quantity || right.quantity,
    unitPrice: left.unitPrice || right.unitPrice,
    linePrice: left.linePrice || right.linePrice,
    discount: left.discount || right.discount,
    addons: left.addons || right.addons,
    reasons: Array.from(new Set([...(left.reasons || []), ...(right.reasons || [])])).slice(0, 4)
  });

  const editableItemReviewAt = (index: number): EditableItemReviewFlags =>
    editableItemReviewFlags[index] || emptyEditableItemReviewFlags();

  const reviewFlagsNeedReview = (flags: EditableItemReviewFlags) =>
    flags.name ||
    flags.quantity ||
    flags.unitPrice ||
    flags.linePrice ||
    flags.discount ||
    flags.addons ||
    flags.reasons.length > 0;

  const editableItemNeedsReview = (index: number) => {
    const flags = editableItemReviewAt(index);
    return reviewFlagsNeedReview(flags);
  };

  let receiptFlaggedIndices: number[] = [];
  let receiptVisibleIndices: number[] = [];
  $: receiptFlaggedIndices = editableItemReviewFlags
    .map((_, index) => index)
    .filter((index) => editableItemNeedsReview(index));
  $: if (showReceiptFlaggedOnly && receiptFlaggedIndices.length === 0) {
    showReceiptFlaggedOnly = false;
  }
  $: receiptVisibleIndices = showReceiptFlaggedOnly ? receiptFlaggedIndices : editableItems.map((_, index) => index);

  const scrollReceiptReviewItemIntoView = (index: number) => {
    if (!browser || index < 0) return;
    queueMicrotask(() => {
      const target = document.getElementById(`receipt-review-item-${index}`);
      target?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    });
  };

  const setReceiptEditingIndex = (index: number | null) => {
    receiptEditingIndex = index;
    showReceiptItemEditModal = index !== null;
    if (index === null) {
      showReceiptItemAddonsModal = false;
      receiptItemAddonsIndex = null;
      closeReceiptAttachModal();
      return;
    }
    if (receiptItemAddonsIndex !== null && receiptItemAddonsIndex !== index) {
      showReceiptItemAddonsModal = false;
      receiptItemAddonsIndex = null;
    }
    if (receiptAttachSourceIndex !== null && receiptAttachSourceIndex !== index) {
      closeReceiptAttachModal();
    }
  };

  const openReceiptItemEditor = (index: number) => {
    if (!editableItems[index]) return;
    setReceiptEditingIndex(index);
  };

  const closeReceiptItemEditor = () => {
    setReceiptEditingIndex(null);
  };

  const focusNextFlaggedReceiptItem = () => {
    if (!receiptFlaggedIndices.length) return;
    showReceiptFlaggedOnly = false;
    receiptReviewFocusIndex = (receiptReviewFocusIndex + 1) % receiptFlaggedIndices.length;
    const targetIndex = receiptFlaggedIndices[receiptReviewFocusIndex];
    scrollReceiptReviewItemIntoView(targetIndex);
    openReceiptItemEditor(targetIndex);
  };

  const toggleContactSelection = (contactId: string) => {
    const next = new Set(addPersonSelectedContactIds);
    if (next.has(contactId)) next.delete(contactId);
    else next.add(contactId);
    addPersonSelectedContactIds = next;
  };

  const addSelectedContactsToRoom = () => {
    if (!room) return;
    addPersonSelectedContactIds.forEach((contactId) => {
      const contact = contacts.find((c) => c.id === contactId);
      if (!contact) return;
      const name = contact.name.trim();
      if (!name) return;
      const existing = Object.entries(room?.participants || {}).find(
        ([, p]) => (p as Participant).name.trim().toLowerCase() === name.toLowerCase()
      );
      const id = existing ? existing[0] : `guest-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
      sendParticipantUpdate(id, name, false, contact.venmoUsername || '');
      touchContact(contactId);
    });
    showAddPersonModal = false;
    addPersonSelectedContactIds = new Set();
    addPersonSearchFilter = '';
  };

  const addManualPersonToRoom = () => {
    const name = addPersonName.trim();
    if (!name) return;
    const existing = Object.entries(room?.participants || {}).find(
      ([, p]) => (p as Participant).name.trim().toLowerCase() === name.toLowerCase()
    );
    const id = existing ? existing[0] : `guest-${Date.now()}`;
    sendParticipantUpdate(id, name, false, addPersonVenmoInput);
    showAddPersonModal = false;
  };

  let bulkSelectedItemIds: string[] = [];
  $: bulkSelectedItemIds = Object.entries(bulkAssignSelectedByItemId)
    .filter(([, selected]) => !!selected)
    .map(([itemId]) => itemId);
  $: if (bulkAssignMode && !bulkAssignTargetParticipantId && participants.length > 0) {
    bulkAssignTargetParticipantId = participants[0].id;
  }
  $: if (bulkAssignMode) {
    const existingIds = new Set(standaloneItems.map((item) => item.id));
    const cleaned = Object.fromEntries(
      Object.entries(bulkAssignSelectedByItemId).filter(([itemId, selected]) => selected && existingIds.has(itemId))
    );
    if (Object.keys(cleaned).length !== Object.keys(bulkAssignSelectedByItemId).length) {
      bulkAssignSelectedByItemId = cleaned;
    }
  }
  const setBulkAssignMode = (next: boolean) => {
    bulkAssignMode = next;
    if (!next) {
      bulkAssignSelectedByItemId = {};
      bulkAssignTargetParticipantId = '';
    }
  };

  const toggleBulkItemSelection = (itemId: string) => {
    bulkAssignSelectedByItemId = {
      ...bulkAssignSelectedByItemId,
      [itemId]: !bulkAssignSelectedByItemId[itemId]
    };
  };

  const replaceItemAssignments = (item: Item, assigned: Record<string, boolean>) => {
    const nextItem: Item = { ...item, assigned };
    upsertItem(nextItem);
  };

  const applyBulkSplitEvenlyAcrossRoom = () => {
    if (!room || bulkSelectedItemIds.length === 0) return;
    const everyoneIds = participants.map((participant) => participant.id);
    if (!everyoneIds.length) return;
    bulkSelectedItemIds.forEach((itemId) => {
      const item = room?.items?.[itemId];
      if (!item) return;
      const assigned = Object.fromEntries(everyoneIds.map((id) => [id, true]));
      replaceItemAssignments(item, assigned);
    });
    bulkAssignSelectedByItemId = {};
  };

  const applyBulkAssignToParticipant = () => {
    if (!room || bulkSelectedItemIds.length === 0 || !bulkAssignTargetParticipantId) return;
    bulkSelectedItemIds.forEach((itemId) => {
      const item = room?.items?.[itemId];
      if (!item) return;
      const merged = { ...(item.assigned || {}), [bulkAssignTargetParticipantId]: true };
      replaceItemAssignments(item, merged);
    });
    bulkAssignSelectedByItemId = {};
  };

  const clearBulkAssignments = () => {
    if (!room || bulkSelectedItemIds.length === 0) return;
    bulkSelectedItemIds.forEach((itemId) => {
      const item = room?.items?.[itemId];
      if (!item) return;
      replaceItemAssignments(item, {});
    });
    bulkAssignSelectedByItemId = {};
  };

  const openRepeatedGroupSheet = (groupKey: string) => {
    if (!groupKey || !repeatedGroupsByKey[groupKey]) return;
    repeatedGroupSheetKey = groupKey;
    showRepeatedGroupSheet = true;
  };

  const closeRepeatedGroupSheet = () => {
    showRepeatedGroupSheet = false;
    repeatedGroupSheetKey = null;
  };

  const servingAssignedCount = (item: Item) =>
    Object.values(item?.assigned || {}).filter(Boolean).length;

  const removeRepeatedGroup = (group: RepeatedGroup) => {
    removeItems(group.items.map((item) => item.id));
    if (repeatedGroupSheetKey === group.key) {
      closeRepeatedGroupSheet();
    }
  };

  const applyRepeatedGroupOneEach = (group: RepeatedGroup) => {
    const orderedParticipants = [...participants].sort(
      (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id)
    );
    if (group.items.length !== orderedParticipants.length) return;
    group.items.forEach((item, index) => {
      const participant = orderedParticipants[index];
      if (!participant) return;
      replaceItemAssignments(item, { [participant.id]: true });
    });
  };

  const applyRepeatedGroupEveryone = (group: RepeatedGroup) => {
    const assigned = Object.fromEntries(participants.map((participant) => [participant.id, true]));
    group.items.forEach((item) => replaceItemAssignments(item, assigned));
  };

  const clearRepeatedGroupAssignments = (group: RepeatedGroup) => {
    group.items.forEach((item) => replaceItemAssignments(item, {}));
  };

  const pullServingOutAsItem = (group: RepeatedGroup, item: Item) => {
    const servingIndex = Math.max(
      0,
      group.items.findIndex((candidate) => candidate.id === item.id)
    );
    const baseName = repeatedGroupBaseName(item);
    upsertItem(
      withMergedItemMeta(
        {
          ...item,
          name: numberedGeneratedItemName(baseName, servingIndex, group.items.length)
        },
        { repeated_group_excluded: true }
      )
    );
  };

  const hydrateJoinPrefillFromCookies = () => {
    const prefs = loadIdentityPrefs();
    joinPrefillName = (prefs.name || '').trim();
    joinPrefillVenmo = normalizeVenmoUsername(prefs.venmoUsername || '');
  };

  const prefillJoinPromptFromCookies = () => {
    const cookieName = joinPrefillName.trim();
    const cookieVenmo = normalizeVenmoUsername(joinPrefillVenmo);
    const cookieKey = cookieName.toLowerCase();
    const matchingParticipant = cookieKey
      ? roomParticipantsForJoin.find((participant) => participant.name.trim().toLowerCase() === cookieKey)
      : null;

    if (matchingParticipant) {
      joinNameInput = matchingParticipant.name;
      joinVenmoInput = normalizeVenmoUsername(matchingParticipant.venmoUsername || cookieVenmo);
      return;
    }

    if (!joinNameInput.trim() && cookieName) {
      joinNameInput = cookieName;
    }
    if (!joinVenmoInput.trim() && cookieVenmo) {
      joinVenmoInput = cookieVenmo;
    }
  };

  const formatAmountForVenmo = (amount: number) => {
    if (amount <= 0) return '';
    const usdFactor = factorFor('USD');
    const usdExp = exponentFor('USD');
    if (roomCurrency === 'USD') {
      return (Math.max(0, amount) / usdFactor).toFixed(usdExp);
    }
    if (!venmoUsdRate || !Number.isFinite(venmoUsdRate) || venmoUsdRate <= 0) return '';
    const roomFactor = factorFor(roomCurrency);
    const usdMinor = Math.round((Math.max(0, amount) / roomFactor) * venmoUsdRate * usdFactor);
    return (usdMinor / usdFactor).toFixed(usdExp);
  };

  const isLikelyMobileDevice = () => {
    if (!browser) return false;
    const ua = navigator.userAgent || '';
    return /iPhone|iPad|iPod|Android/i.test(ua);
  };

  const acquireVenmoLaunchCooldown = () => {
    const now = Date.now();
    if (now < venmoLaunchCooldownUntil) return false;
    venmoLaunchCooldownUntil = now + 1200;
    return true;
  };

  const venmoChargeQuery = (username: string | null | undefined, amount: number, note: string) => {
    const normalized = normalizeVenmoUsername(username);
    const usdAmount = formatAmountForVenmo(amount);
    if (!normalized || !usdAmount) return '';
    return [
      `txn=charge`,
      `recipients=${encodeURIComponent(normalized)}`,
      `amount=${encodeURIComponent(usdAmount)}`,
      `note=${encodeURIComponent(note || 'Bill split')}`
    ].join('&');
  };

  const venmoChargeUrl = (username: string | null | undefined, amount: number, note: string) => {
    const query = venmoChargeQuery(username, amount, note);
    if (!query) return '';
    return `https://venmo.com/?${query}`;
  };

  const venmoChargeAppUrl = (username: string | null | undefined, amount: number, note: string) => {
    const query = venmoChargeQuery(username, amount, note);
    if (!query) return '';
    return `venmo://paycharge?${query}`;
  };

  const openVenmoCharge = (username: string | null | undefined, amount: number, note: string) => {
    const webUrl = venmoChargeUrl(username, amount, note);
    if (!webUrl) return false;
    if (!browser) return false;
    if (!acquireVenmoLaunchCooldown()) return false;
    const appUrl = venmoChargeAppUrl(username, amount, note);
    const isMobile = isLikelyMobileDevice();
    if (!isMobile || !appUrl) {
      const opened = window.open(webUrl, '_blank', 'noopener,noreferrer');
      return !!opened;
    }
    // Mobile: do not auto-fallback to web URL because that can create duplicate charge pages.
    window.location.assign(appUrl);
    return true;
  };

  const venmoNoteWithDate = (billName: string | null | undefined) => {
    const trimmed = `${billName || ''}`.trim();
    const name = (trimmed || 'Bill split').replace(/\+/g, ' ').replace(/\s+/g, ' ').trim();
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    return `${name} ${mm}/${dd}/${yyyy}`;
  };

  const setBillCodeShareStatus = (value: string) => {
    billCodeShareFeedback = value;
    if (billCodeShareFeedbackTimer) clearTimeout(billCodeShareFeedbackTimer);
    billCodeShareFeedbackTimer = setTimeout(() => {
      billCodeShareFeedback = '';
      billCodeShareFeedbackTimer = null;
    }, 1800);
  };

  const copyRoomLinkToClipboard = async () => {
    if (!browser || !shareLink) return false;
    try {
      await navigator.clipboard.writeText(shareLink);
      return true;
    } catch {
      const fallbackInput = document.createElement('textarea');
      fallbackInput.value = shareLink;
      fallbackInput.setAttribute('readonly', 'true');
      fallbackInput.style.position = 'fixed';
      fallbackInput.style.opacity = '0';
      fallbackInput.style.pointerEvents = 'none';
      document.body.appendChild(fallbackInput);
      fallbackInput.focus();
      fallbackInput.select();
      let copied = false;
      try {
        copied = document.execCommand('copy');
      } catch {
        copied = false;
      }
      fallbackInput.remove();
      return copied;
    }
  };

  const shareRoomFromBillCode = async () => {
    if (!browser || !shareLink) return;
    const roomName = `${room?.name || ''}`.trim() || 'Shared Bill';
    const upperCode = roomCode?.toUpperCase() || '';
    const nav = navigator as Navigator & {
      share?: (data: ShareData) => Promise<void>;
    };
    if (typeof nav.share === 'function') {
      try {
        await nav.share({
          title: `${roomName} · Divvi`,
          text: `Join my bill: ${roomName} (${upperCode})`,
          url: shareLink
        });
        setBillCodeShareStatus('Shared');
        return;
      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') return;
      }
    }
    const copied = await copyRoomLinkToClipboard();
    setBillCodeShareStatus(copied ? 'Copied link' : 'Share unavailable');
  };

  const changeCurrency = (code: string) => {
    if (!code) return;
    const upper = code.toUpperCase();
    roomCurrency = upper;
    room = room ? { ...room, currency: upper } : room;
    const payload = { currency: upper };
    sendOp({ kind: 'set_room_name', payload });
    applyLocalOp({ kind: 'set_room_name', payload, timestamp: Date.now() });
    venmoUsdRate = null;
    venmoUsdRateBase = '';
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

  const ensureVenmoUsdRate = async () => {
    if (roomCurrency === 'USD') {
      venmoUsdRate = 1;
      venmoUsdRateBase = 'USD';
      return 1;
    }
    if (venmoUsdRate && venmoUsdRateBase === roomCurrency) return venmoUsdRate;
    const res = await fetch(`${apiBase}/fx?base=${roomCurrency}&target=USD`);
    if (!res.ok) throw new Error('USD rate unavailable');
    const payload = await res.json();
    const rate = Number(payload.rate);
    if (!Number.isFinite(rate) || rate <= 0) throw new Error('Invalid USD rate');
    venmoUsdRate = rate;
    venmoUsdRateBase = roomCurrency;
    return rate;
  };

  const numericInputToString = (value: string | number | null | undefined) => {
    if (value === null || value === undefined) return '';
    return `${value}`.trim();
  };

  const toCentsInput = (value: string | number | null | undefined, code = roomCurrency) => {
    const raw = numericInputToString(value);
    const num = Number.parseFloat(raw);
    if (!Number.isFinite(num)) return 0;
    return Math.max(0, Math.round(num * factorFor(code)));
  };

  const parseByCurrency = (value: string | number | null | undefined, code = roomCurrency) => {
    const raw = numericInputToString(value);
    const exp = exponentFor(code);
    const factor = Math.pow(10, exp);
    const num = Number.parseFloat(raw);
    if (!Number.isFinite(num)) return 0;
    return Math.max(0, Math.round(num * factor));
  };

  const inputValueFromMinorUnits = (amountMinorUnits: number, code = roomCurrency) => {
    const exp = exponentFor(code);
    const factor = factorFor(code);
    const cents = Math.max(0, Math.round(Number(amountMinorUnits) || 0));
    return (cents / factor).toFixed(exp);
  };

  const normalizeInputByCurrency = (value: string | number | null | undefined, code = roomCurrency) => {
    const trimmed = numericInputToString(value);
    if (!trimmed) return '';
    const exp = exponentFor(code);
    const factor = factorFor(code);
    const minor = parseByCurrency(trimmed, code);
    return (minor / factor).toFixed(exp);
  };

  const normalizeRequiredInputByCurrency = (value: string | number | null | undefined, code = roomCurrency) => {
    const normalized = normalizeInputByCurrency(value, code);
    return normalized || zeroPlaceholderByCurrency(code);
  };

  const zeroPlaceholderByCurrency = (code = roomCurrency) => {
    const exp = exponentFor(code);
    return exp > 0 ? `0.${'0'.repeat(exp)}` : '0';
  };

  const itemNetSubtotalFromEditable = (list: typeof editableItems) => {
    if (!list?.length) return 0;
    return list.reduce((sum, item) => {
      const qty = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
      const code = receiptCurrencySelection || roomCurrency;
      const unit = toCentsInput(item.unitPrice, code);
      const line = toCentsInput(item.linePrice, code);
      const addonPerItem = (item.addons || []).reduce(
        (addonSum, addon) => addonSum + toCentsInput(addon?.price || '', code),
        0
      );
      const gross = (line || unit * qty) + addonPerItem * qty;
      const discountPct = Number.parseFloat(item.discountPercent || '0') || 0;
      const discountCentsPerUnit =
        toCentsInput(item.discountCents, code) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
      const net = Math.max(0, gross - discountCentsPerUnit * qty);
      return sum + net;
    }, 0);
  };

  const itemGrossSubtotalFromEditable = (list: typeof editableItems) => {
    if (!list?.length) return 0;
    return list.reduce((sum, item) => {
      const qty = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
      const code = receiptCurrencySelection || roomCurrency;
      const unit = toCentsInput(item.unitPrice, code);
      const line = toCentsInput(item.linePrice, code);
      const addonPerItem = (item.addons || []).reduce(
        (addonSum, addon) => addonSum + toCentsInput(addon?.price || '', code),
        0
      );
      const gross = Math.max(0, (line || unit * qty) + addonPerItem * qty);
      return sum + gross;
    }, 0);
  };

  const subtotalFromEditable = (list: typeof editableItems) =>
    Math.max(0, itemNetSubtotalFromEditable(list) - parsedBillDiscountCents);

  const discountedUnitAndNetFromEditable = (item: typeof editableItems[number]) => {
    const code = receiptCurrencySelection || roomCurrency;
    const qty = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
    const unit = toCentsInput(item.unitPrice, code);
    const line = toCentsInput(item.linePrice, code);
    const addonPerItem = (item.addons || []).reduce(
      (addonSum, addon) => addonSum + toCentsInput(addon?.price || '', code),
      0
    );
    const gross = (line || unit * qty) + addonPerItem * qty;
    const discountPct = Number.parseFloat(item.discountPercent || '0') || 0;
    const discountCentsPerUnit =
      toCentsInput(item.discountCents, code) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
    const net = Math.max(0, gross - discountCentsPerUnit * qty);
    const netUnit = qty > 0 ? Math.max(0, Math.round(net / qty)) : 0;
    return { netUnit, netTotal: net };
  };

  const discountedUnitAndNetFromItemForm = (form: ItemFormState, addonPerItemCents: number) => {
    const qty = Math.max(1, Number.parseInt(form.quantity || '1', 10) || 1);
    const unit = toCentsInput(form.unitPrice, roomCurrency);
    const line = toCentsInput(form.linePrice, roomCurrency);
    const gross = (line || unit * qty) + addonPerItemCents * qty;
    const discountPct = Number.parseFloat(form.discountPercent || '0') || 0;
    const discountCentsPerUnit =
      toCentsInput(form.discountCents, roomCurrency) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
    const netTotal = Math.max(0, gross - discountCentsPerUnit * qty);
    const netUnit = qty > 0 ? Math.max(0, Math.round(netTotal / qty)) : 0;
    return { netUnit, netTotal };
  };

  const pricingPreviewFromItemForm = (form: ItemFormState, addonPerItemCents: number) => {
    const qty = Math.max(1, Number.parseInt(form.quantity || '1', 10) || 1);
    const unit = toCentsInput(form.unitPrice, roomCurrency);
    const explicitTotal = toCentsInput(form.linePrice, roomCurrency);
    const baseTotal = explicitTotal || (unit ? unit * qty : 0);
    const addonPerItem = addonPerItemCents;
    const addonTotal = addonPerItem * qty;
    const grossTotal = Math.max(0, baseTotal + addonTotal);
    return { qty, unit, baseTotal, addonPerItem, addonTotal, grossTotal };
  };

  const removeEditableItem = (index: number, options: { rebalanceTaxAndTip?: boolean } = {}) => {
    if (!receiptResult) return;
    const rebalanceTaxAndTip = options.rebalanceTaxAndTip !== false;
    const list = [...editableItems];
    const removed = list[index];
    if (!removed) return;
    if (showReceiptItemAddonsModal && receiptItemAddonsIndex !== null) {
      if (receiptItemAddonsIndex === index) {
        showReceiptItemAddonsModal = false;
        receiptItemAddonsIndex = null;
      } else if (receiptItemAddonsIndex > index) {
        receiptItemAddonsIndex -= 1;
      }
    }
    if (showReceiptAttachModal && receiptAttachSourceIndex !== null) {
      if (receiptAttachSourceIndex === index) {
        showReceiptAttachModal = false;
        receiptAttachSourceIndex = null;
      } else if (receiptAttachSourceIndex > index) {
        receiptAttachSourceIndex -= 1;
      }
    }
    if (receiptEditingIndex !== null) {
      if (receiptEditingIndex === index) {
        closeReceiptItemEditor();
      } else if (receiptEditingIndex > index) {
        receiptEditingIndex -= 1;
      }
    }
    const prevItemsSubtotal = itemNetSubtotalFromEditable(list);
    const prevItemsGrossSubtotal = itemGrossSubtotalFromEditable(list);
    list.splice(index, 1);
    editableItems = list;
    editableItemReviewFlags = editableItemReviewFlags.filter((_, idx) => idx !== index);
    const remainingSubtotal = Math.max(0, itemNetSubtotalFromEditable(list) - parsedBillDiscountCents);
    const remainingGrossSubtotal = itemGrossSubtotalFromEditable(list);
    if (rebalanceTaxAndTip) {
      const removedNet = discountedUnitAndNetFromEditable(removed).netTotal;
      const removedGross = pricingPreviewFromEditable(removed).grossTotal;
      if (prevItemsSubtotal > 0 && parsedTaxInput !== null && parsedTaxInput !== undefined) {
        const currentTax = parsedTaxCents;
        const adjustment = Math.round((removedNet / prevItemsSubtotal) * currentTax);
        const newTax = Math.max(0, currentTax - adjustment);
        const code = receiptCurrencySelection || roomCurrency;
        const exp = exponentFor(code);
        const factor = factorFor(code);
        parsedTaxInput = (newTax / factor).toFixed(exp);
      }
      if (prevItemsGrossSubtotal > 0 && parsedTipInput !== null && parsedTipInput !== undefined) {
        const currentTip = parsedTipCents;
        const adjustment = Math.round((removedGross / prevItemsGrossSubtotal) * currentTip);
        const newTip = Math.max(0, currentTip - adjustment);
        const code = receiptCurrencySelection || roomCurrency;
        const exp = exponentFor(code);
        const factor = factorFor(code);
        parsedTipInput = (newTip / factor).toFixed(exp);
      }
    }
    receiptSubtotalCents = remainingSubtotal;
    receiptGrossSubtotalCents = remainingGrossSubtotal;
  };

  const openReceiptAttachModal = (index: number) => {
    if (!editableItems[index] || editableItems.length < 2) return;
    openReceiptItemEditor(index);
    if (showReceiptAttachModal && receiptAttachSourceIndex === index) {
      closeReceiptAttachModal();
      return;
    }
    showReceiptItemAddonsModal = false;
    receiptItemAddonsIndex = null;
    receiptAttachSourceIndex = index;
    showReceiptAttachModal = true;
  };

  const closeReceiptAttachModal = () => {
    showReceiptAttachModal = false;
    receiptAttachSourceIndex = null;
  };

  const attachEditableItemAsAddon = (sourceIndex: number, targetIndex: number) => {
    if (sourceIndex === targetIndex) return;
    const source = editableItems[sourceIndex];
    const target = editableItems[targetIndex];
    if (!source || !target) return;
    const code = receiptCurrencySelection || roomCurrency;
    const targetQty = Math.max(1, Number.parseInt(target.quantity || '1', 10) || 1);
    const sourceGrossCents = pricingPreviewFromEditable(source).grossTotal;
    const perItemAddonCents = Math.max(0, Math.round(sourceGrossCents / targetQty));
    const sourceQty = Math.max(1, Number.parseInt(source.quantity || '1', 10) || 1);
    const sourceNameBase = normalizeAddonLabel(source.name || '') || source.name?.trim() || 'Modifier';
    const sourceAddonNames = (source.addons || [])
      .map((addon) => normalizeAddonLabel(addon?.name || ''))
      .filter(Boolean);
    const sourceName = sourceQty > 1 ? `${sourceNameBase} (${sourceQty}x)` : sourceNameBase;
    const addonName = sourceAddonNames.length > 0 ? `${sourceName}: ${sourceAddonNames.join(', ')}` : sourceName;
    const addonPrice = inputValueFromMinorUnits(perItemAddonCents, code);

    const sourceFlags = editableItemReviewAt(sourceIndex);
    const targetFlags = editableItemReviewAt(targetIndex);

    const nextItems = editableItems.map((item, idx) =>
      idx === targetIndex
        ? {
            ...item,
            addons: [...(item.addons || []), { name: addonName, price: addonPrice }]
          }
        : item
    );
    const nextFlags = editableItemReviewFlags.map((flags, idx) =>
      idx === targetIndex ? mergeEditableItemReviewFlags(targetFlags, sourceFlags) : flags
    );

    nextItems.splice(sourceIndex, 1);
    nextFlags.splice(sourceIndex, 1);
    editableItems = nextItems;
    editableItemReviewFlags = nextFlags;
    closeReceiptAttachModal();
    setReceiptEditingIndex(targetIndex > sourceIndex ? targetIndex - 1 : targetIndex);

    if (showReceiptItemAddonsModal && receiptItemAddonsIndex !== null) {
      if (receiptItemAddonsIndex === sourceIndex) {
        showReceiptItemAddonsModal = false;
        receiptItemAddonsIndex = null;
      } else if (receiptItemAddonsIndex > sourceIndex) {
        receiptItemAddonsIndex -= 1;
      }
    }
  };

  const openReceiptItemAddonsModal = (index: number) => {
    if (!editableItems[index]) return;
    openReceiptItemEditor(index);
    if (showReceiptItemAddonsModal && receiptItemAddonsIndex === index) {
      showReceiptItemAddonsModal = false;
      receiptItemAddonsIndex = null;
      return;
    }
    closeReceiptAttachModal();
    editableItems = editableItems.map((item, idx) =>
      idx === index ? { ...item, addons: item.addons || [] } : item
    );
    receiptItemAddonsIndex = index;
    showReceiptItemAddonsModal = true;
  };

  const updateEditableItemAddon = (
    itemIndex: number,
    addonIndex: number,
    field: 'name' | 'price',
    value: string
  ) => {
    editableItems = editableItems.map((item, idx) => {
      if (idx !== itemIndex) return item;
      const addons = (item.addons || []).map((addon, aIdx) =>
        aIdx === addonIndex ? { ...addon, [field]: value } : addon
      );
      return { ...item, addons };
    });
  };

  const addEditableItemAddon = (itemIndex: number) => {
    editableItems = editableItems.map((item, idx) =>
      idx === itemIndex ? { ...item, addons: [...(item.addons || []), { name: '', price: '' }] } : item
    );
  };

  const removeEditableItemAddon = (itemIndex: number, addonIndex: number) => {
    editableItems = editableItems.map((item, idx) => {
      if (idx !== itemIndex) return item;
      return { ...item, addons: (item.addons || []).filter((_, aIdx) => aIdx !== addonIndex) };
    });
  };

  const promoteEditableItemAddonToItem = (itemIndex: number, addonIndex: number) => {
    const parent = editableItems[itemIndex];
    const addon = parent?.addons?.[addonIndex];
    if (!parent || !addon) return;
    const code = receiptCurrencySelection || roomCurrency;
    const qty = Math.max(1, Number.parseInt(parent.quantity || '1', 10) || 1);
    const addonUnitCents = toCentsInput(addon.price || '', code);
    const addonLineCents = addonUnitCents * qty;
    const newItem = {
      name: normalizeAddonLabel(addon.name || '') || (addon.name || '').trim() || 'Item',
      quantity: String(qty),
      unitPrice: inputValueFromMinorUnits(addonUnitCents, code),
      linePrice: inputValueFromMinorUnits(addonLineCents, code),
      discountCents: '',
      discountPercent: '',
      addons: [] as ItemFormAddon[],
      discountMode: 'amount' as const,
      totalInputMode: 'auto' as const
    };

    const nextItems = editableItems.map((item, idx) => {
      if (idx !== itemIndex) return item;
      return { ...item, addons: (item.addons || []).filter((_, aIdx) => aIdx !== addonIndex) };
    });
    const insertAt = itemIndex + 1;
    nextItems.splice(insertAt, 0, newItem);
    editableItems = nextItems;
    if (receiptEditingIndex !== null && receiptEditingIndex >= insertAt) {
      receiptEditingIndex += 1;
    }

    const nextFlags = [...editableItemReviewFlags];
    nextFlags.splice(insertAt, 0, emptyEditableItemReviewFlags());
    editableItemReviewFlags = nextFlags;
  };

  const editableItemAddonTotalCents = (itemIndex: number) => {
    const code = receiptCurrencySelection || roomCurrency;
    return (editableItems[itemIndex]?.addons || []).reduce(
      (sum, addon) => sum + parseByCurrency(addon?.price || '', code),
      0
    );
  };

  const editableItemAddonExtendedTotalCents = (itemIndex: number) => {
    const qty = Math.max(1, Number.parseInt(editableItems[itemIndex]?.quantity || '1', 10) || 1);
    return editableItemAddonTotalCents(itemIndex) * qty;
  };

  const itemFormAddonExtendedTotalCents = (form: ItemFormState, addonPerItemCents: number) => {
    const qty = Math.max(1, Number.parseInt(form.quantity || '1', 10) || 1);
    return addonPerItemCents * qty;
  };

  const fallbackSeed = (id: string, name?: string) => {
    const base = id || name || 'seed';
    return hexSeed(base);
  };

  const normalizeParticipant = (p: any): Participant => {
    const id = p.id || p.ID || '';
    const name = p.name || p.Name || '';
    const colorSeed = p.colorSeed || p.ColorSeed || p.color_seed || p.Color_seed || fallbackSeed(id, name);
    const venmoUsername = normalizeVenmoUsername(
      p.venmoUsername || p.VenmoUsername || p.venmo_username || p.Venmo_username || ''
    );
    return {
      id,
      name,
      initials: p.initials || p.Initials || initialsFromName(name),
      colorSeed,
      venmoUsername,
      present: p.present ?? p.Present ?? false,
      finished: p.finished ?? p.Finished ?? false
    };
  };

  const normalizeSortOrder = (value: unknown) => {
    const numeric = Number(value);
    return Number.isFinite(numeric) ? Math.round(numeric) : null;
  };

  const normalizeItem = (item: any): Item => {
    const normalized: Item = {
      ...item,
      id: item?.id || item?.ID || '',
      name: item?.name || item?.Name || '',
      quantity: Math.max(1, Math.round(Number(item?.quantity ?? item?.Quantity ?? 1) || 1)),
      unit_price_cents: Math.max(0, Math.round(Number(item?.unit_price_cents ?? item?.UnitPriceCents ?? 0) || 0)),
      line_price_cents: Math.max(0, Math.round(Number(item?.line_price_cents ?? item?.LinePriceCents ?? 0) || 0)),
      discount_cents: Math.max(0, Math.round(Number(item?.discount_cents ?? item?.DiscountCents ?? 0) || 0)),
      discount_percent: Math.max(0, Number(item?.discount_percent ?? item?.DiscountPercent ?? 0) || 0),
      assigned: typeof item?.assigned === 'object' && item.assigned ? { ...item.assigned } : {},
      raw_text: item?.raw_text ?? item?.RawText ?? '',
      sort_order: normalizeSortOrder(item?.sort_order ?? item?.sortOrder ?? item?.SortOrder),
      meta: item?.meta && typeof item.meta === 'object' ? { ...item.meta } : undefined
    };
    return normalized;
  };

  const itemSortOrderOr = (item: Pick<Item, 'sort_order'>, fallback: number) => {
    const normalized = normalizeSortOrder(item?.sort_order);
    return normalized === null ? fallback : normalized;
  };

  const itemSortOrder = (item: Pick<Item, 'sort_order'>) => itemSortOrderOr(item, Number.MAX_SAFE_INTEGER);

  const sortItemsByOrder = (list: Item[]) =>
    [...(list || [])].sort((left, right) => {
      const leftOrder = itemSortOrder(left);
      const rightOrder = itemSortOrder(right);
      if (leftOrder !== rightOrder) return leftOrder - rightOrder;
      return (left.id || '').localeCompare(right.id || '');
    });

  const nextStandaloneSortOrder = (list: Item[]) => {
    const maxOrder = (list || []).reduce((max, item) => Math.max(max, itemSortOrderOr(item, -SORT_ORDER_STEP)), -SORT_ORDER_STEP);
    if (maxOrder < 0) return SORT_ORDER_STEP;
    return Math.floor(maxOrder / SORT_ORDER_STEP + 1) * SORT_ORDER_STEP;
  };

  const itemAddonMeta = (item: Item): ItemAddonMeta[] => {
    const raw = Array.isArray(item?.meta?.addons) ? item.meta.addons : [];
    return raw
      .map((addon: any) => ({
        name: normalizeAddonLabel(String(addon?.name || '')),
        price_cents: Number.isFinite(Number(addon?.price_cents))
          ? Math.max(0, Math.round(Number(addon.price_cents)))
          : 0,
        raw_text: addon?.raw_text ?? null
      }))
      .filter((addon) => addon.name || addon.price_cents > 0);
  };

  const repeatedGroupExcluded = (item: Item) => Boolean(item?.meta?.repeated_group_excluded);

  const withMergedItemMeta = (item: Item, patch: Partial<ItemMeta>) => {
    const nextMeta: ItemMeta = { ...(item.meta || {}) };
    Object.entries(patch).forEach(([key, value]) => {
      if (value === undefined || value === null || value === false) {
        delete nextMeta[key];
      } else {
        (nextMeta as any)[key] = value;
      }
    });
    return {
      ...item,
      meta: Object.keys(nextMeta).length > 0 ? nextMeta : undefined
    };
  };

  const setDocumentModalLock = (locked: boolean) => {
    if (!browser) return;
    document.documentElement.classList.toggle('modal-open', locked);
    document.body.classList.toggle('modal-open', locked);
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
        const incomingItem = normalizeItem(payload.item);
        const existingItem = next.items[incomingItem.id] ? normalizeItem(next.items[incomingItem.id]) : null;
        const item =
          normalizeSortOrder(incomingItem.sort_order) === null && existingItem
            ? { ...incomingItem, sort_order: existingItem.sort_order ?? null }
            : incomingItem;
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
        if (typeof payload.bill_discount_cents === 'number') next.bill_discount_cents = payload.bill_discount_cents;
        if (typeof payload.bill_charges_cents === 'number') next.bill_charges_cents = payload.bill_charges_cents;
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
              colorSeed: p.colorSeed || identity.colorSeed,
              venmoUsername: p.venmoUsername || identity.venmoUsername || ''
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

  const upsertItem = (item: Item) => {
    const normalizedItem = normalizeItem(item);
    const payload = { item: normalizedItem };
    sendOp({ kind: 'set_item', actor_id: identity.userId, payload });
    applyLocalOp({ kind: 'set_item', payload });
  };

  const removeItems = (itemIds: string[]) => {
    Array.from(new Set(itemIds)).forEach((itemId) => {
      if (!itemId) return;
      sendOp({
        kind: 'remove_item',
        actor_id: identity.userId,
        payload: { id: itemId }
      });
      applyLocalOp({
        kind: 'remove_item',
        payload: { id: itemId }
      });
    });
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
      const normalizedItems = Object.fromEntries(
        Object.entries(message.doc.items || {}).map(([id, item]) => [
          id,
          normalizeItem(item)
        ])
      );
      room = {
        ...message.doc,
        items: normalizedItems,
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
          colorSeed: self.colorSeed || identity.colorSeed,
          venmoUsername: self.venmoUsername || identity.venmoUsername || ''
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

  const splitEvenCents = (total: number, participantIds: string[]) => {
    const ids = Array.from(new Set(participantIds)).sort((a, b) => a.localeCompare(b));
    if (total <= 0 || ids.length === 0) return {} as Record<string, number>;
    const base = Math.floor(total / ids.length);
    let remainder = total - base * ids.length;
    const result: Record<string, number> = {};
    ids.forEach((id) => {
      result[id] = base + (remainder > 0 ? 1 : 0);
      if (remainder > 0) remainder -= 1;
    });
    return result;
  };

  const computeSummary = () => {
    if (!room) return null;
    const participants = room.participants || {};
    const itemsArr = Object.values(room.items || {}) as Item[];

    const gross = itemsArr.reduce((sum, it) => sum + Math.max(0, Number(it.line_price_cents || 0)), 0);
    const itemDiscount = itemsArr.reduce(
      (sum, it) =>
        sum +
        Math.min(
          Math.max(0, Number(it.line_price_cents || 0)),
          Math.max(0, Number(it.discount_cents || 0) * (it.quantity || 1))
        ),
      0
    );
    const billDiscount = Math.min(Math.max(0, room.bill_discount_cents || 0), Math.max(0, gross - itemDiscount));
    const discount = itemDiscount + billDiscount;
    const net = Math.max(0, gross - discount);
    const billCharges = Math.max(0, room.bill_charges_cents || 0);
    const tax = room.tax_cents || 0;
    const tip = room.tip_cents || 0;

    const perPerson = new Map<
      string,
      {
        name: string;
        color: string;
        venmoUsername: string;
        items: {
          item_id: string;
          name: string;
          share_cents: number;
          fraction_numerator: number;
          fraction_denominator: number;
        }[];
        grossItemsTotal: number;
        itemsTotal: number;
        billDiscountShare: number;
        billChargesShare: number;
        taxShare: number;
        tipShare: number;
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

    itemsArr.forEach((it) => {
      const assignees = Object.entries(it.assigned || {}).filter(([, on]) => on).map(([uid]) => uid);
      if (assignees.length === 0) return;
      const grossLine = Math.max(0, Number(it.line_price_cents || 0));
      const itemDiscountLine = Math.min(
        grossLine,
        Math.max(0, Number(it.discount_cents || 0) * (it.quantity || 1))
      );
      const netLine = Math.max(0, grossLine - itemDiscountLine);
      const sortedAssignees = [...assignees].sort((a, b) => a.localeCompare(b));
      const splitGross = splitEvenCents(grossLine, sortedAssignees);
      const splitNet = splitEvenCents(netLine, sortedAssignees);
      sortedAssignees.forEach((uid) => {
        const participant = participants[uid];
        const entry =
          perPerson.get(uid) ||
          ({
            name: participant?.name || uid,
            color: colorHex(participant?.colorSeed),
            venmoUsername: normalizeVenmoUsername(participant?.venmoUsername || ''),
            items: [],
            grossItemsTotal: 0,
            itemsTotal: 0,
            billDiscountShare: 0,
            billChargesShare: 0,
            taxShare: 0,
            tipShare: 0
          } as any);
        const grossShare = Math.max(0, splitGross[uid] || 0);
        const netShare = Math.max(0, splitNet[uid] || 0);
        entry.grossItemsTotal += grossShare;
        entry.itemsTotal += netShare;
        entry.items.push({
          item_id: it.id,
          name: it.name,
          share_cents: netShare,
          fraction_numerator: 1,
          fraction_denominator: sortedAssignees.length
        });
        perPerson.set(uid, entry);
      });
    });

    const grossWeights: Record<string, number> = {};
    perPerson.forEach((person, uid) => {
      grossWeights[uid] = Math.max(0, person.grossItemsTotal || 0);
    });

    const billDiscountSplits = splitProportional(billDiscount, grossWeights);
    const billChargeSplits = splitProportional(billCharges, grossWeights);

    const taxableWeights: Record<string, number> = {};
    perPerson.forEach((person, uid) => {
      const discountShare = billDiscountSplits[uid] || 0;
      taxableWeights[uid] = Math.max(0, person.itemsTotal - discountShare);
    });

    const taxSplits = splitProportional(tax, taxableWeights);
    const tipSplits = splitProportional(tip, grossWeights);

    const detailed = Array.from(perPerson.entries()).map(([uid, person]) => {
      const billDiscountShare = billDiscountSplits[uid] || 0;
      const billChargesShare = billChargeSplits[uid] || 0;
      const taxShare = taxSplits[uid] || 0;
      const tipShare = tipSplits[uid] || 0;
      const totalShare = person.itemsTotal - billDiscountShare + billChargesShare + taxShare + tipShare;
      return {
        id: uid,
        ...person,
        items: person.items.sort((a, b) => a.name.localeCompare(b.name)),
        billDiscountShare,
        billChargesShare,
        taxShare,
        tipShare,
        total: totalShare
      };
    });

    const totalBeforeTip = net + billCharges + tax;
    const total = totalBeforeTip + tip;
    return { gross, itemDiscount, billDiscount, discount, net, billCharges, tax, tip, totalBeforeTip, total, perPerson: detailed };
  };

  const buildSummary = async () => {
    const base = computeSummary();
    if (!base) {
      summaryData = null;
      return;
    }
    summaryData = base;
    try {
      await ensureVenmoUsdRate();
    } catch {
      venmoUsdRate = null;
      venmoUsdRateBase = '';
    }
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
        itemDiscount: convertMinor(base.itemDiscount),
        billDiscount: convertMinor(base.billDiscount),
        discount: convertMinor(base.discount),
        net: convertMinor(base.net),
        billCharges: convertMinor(base.billCharges),
        tax: convertMinor(base.tax),
        tip: convertMinor(base.tip),
        totalBeforeTip: convertMinor(base.totalBeforeTip),
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

  const roundHalfEven = (value: number) => {
    const floor = Math.floor(value);
    const frac = value - floor;
    if (frac > 0.5 + 1e-9) return floor + 1;
    if (Math.abs(frac - 0.5) <= 1e-9) return floor % 2 === 0 ? floor : floor + 1;
    return floor;
  };

  const convertSummaryAmount = (baseMinor: number) => {
    if (!summaryData?.converted || targetCurrency === roomCurrency) return null;
    const rate = Number(summaryData.converted.rate || 0);
    if (!Number.isFinite(rate) || rate <= 0) return null;
    const sourceFactor = factorFor(roomCurrency);
    const targetFactor = factorFor(targetCurrency);
    const scaled = (baseMinor / sourceFactor) * rate * targetFactor;
    return roundHalfEven(scaled);
  };

  type SummaryPerson = NonNullable<typeof summaryData>['perPerson'][number];

  const summaryPersonVenmoUrl = (person: SummaryPerson) =>
    venmoChargeUrl(person.venmoUsername, person.total, venmoNoteWithDate(room?.name));

  const summaryRequestablePeople = (source: SummaryPerson[]) =>
    source.filter((person) => person.id !== identity.userId && !!summaryPersonVenmoUrl(person));

  const summaryUnsentRequestablePeople = (source: SummaryPerson[]) =>
    summaryRequestablePeople(source).filter((person) => !isSummaryRequestSent(person.id, person.total));

  const openSummaryPersonVenmoCharge = (person: SummaryPerson) => {
    const launched = openVenmoCharge(person.venmoUsername, person.total, venmoNoteWithDate(room?.name));
    if (launched) {
      setSummaryRequestSent(person.id, person.total, true);
    }
  };

  const isSummaryRequestSent = (personId: string, total: number) =>
    summaryVenmoSentByPersonId[personId]?.total === total;

  const setSummaryRequestSent = (personId: string, total: number, sent: boolean) => {
    if (!sent) {
      const next = { ...summaryVenmoSentByPersonId };
      delete next[personId];
      summaryVenmoSentByPersonId = next;
      return;
    }
    summaryVenmoSentByPersonId = {
      ...summaryVenmoSentByPersonId,
      [personId]: {
        total,
        sentAt: Date.now()
      }
    };
  };

  const advanceSummaryChargeQueue = () => {
    if (!summaryChargeQueueActive) return;
    const next = summaryChargeQueue[0];
    if (!next) {
      summaryChargeQueueActive = false;
      summaryChargeQueuePendingResume = false;
      return;
    }
    const launched = openVenmoCharge(next.venmoUsername, next.total, venmoNoteWithDate(room?.name));
    if (!launched) {
      summaryChargeQueueActive = false;
      summaryChargeQueuePendingResume = false;
      return;
    }
    setSummaryRequestSent(next.id, next.total, true);
    summaryChargeQueue = summaryChargeQueue.slice(1);
    if (summaryChargeQueue.length === 0) {
      summaryChargeQueueActive = false;
      summaryChargeQueuePendingResume = false;
      return;
    }
    if (isLikelyMobileDevice()) {
      // Continue automatically when the user returns from Venmo.
      summaryChargeQueuePendingResume = true;
      return;
    }
    setTimeout(() => advanceSummaryChargeQueue(), 350);
  };

  const startSummaryChargeAllUnsent = () => {
    if (!summaryData || summaryChargeQueueActive) return;
    const unsent = summaryUnsentRequestablePeople(summaryData.perPerson);
    if (unsent.length === 0) return;
    summaryChargeQueue = unsent.map((person) => ({
      id: person.id,
      venmoUsername: person.venmoUsername,
      total: person.total
    }));
    summaryChargeQueueActive = true;
    summaryChargeQueuePendingResume = false;
    advanceSummaryChargeQueue();
  };

  const summaryItemShareLabel = (item: {
    name: string;
    fraction_numerator: number;
    fraction_denominator: number;
  }) => {
    const denominator = Math.max(1, Math.round(item.fraction_denominator || 1));
    if (denominator <= 1) return item.name;
    return `${Math.max(1, Math.round(item.fraction_numerator || 1))}/${denominator} ${item.name}`;
  };

let summaryVisiblePeople: SummaryPerson[] = [];
$: if (!summaryData) {
  summaryUnsentRequestableCount = 0;
  summaryChargeQueue = [];
  summaryChargeQueueActive = false;
  summaryChargeQueuePendingResume = false;
  summaryVisiblePeople = [];
} else {
  const nextSent = { ...summaryVenmoSentByPersonId };
  let changed = false;
    Object.entries(nextSent).forEach(([personId, state]) => {
      const person = summaryData?.perPerson.find((candidate) => candidate.id === personId);
      if (!person || person.total !== state.total) {
        delete nextSent[personId];
        changed = true;
      }
  });
  if (changed) summaryVenmoSentByPersonId = nextSent;
  summaryUnsentRequestableCount = summaryUnsentRequestablePeople(summaryData.perPerson).length;
  summaryVisiblePeople = summaryData.perPerson.filter((person) => {
    if (!summaryUnsentOnly) return true;
    const requestable = person.id !== identity.userId && !!summaryPersonVenmoUrl(person);
    if (!requestable) return true;
    return !isSummaryRequestSent(person.id, person.total);
  });
}

  const persistRoomHistorySnapshot = () => {
    if (!browser || !room || !identity.userId) return;
    const joinedName = (identity.name || '').trim();
    if (!joinedName) return;
    const participant = room.participants?.[identity.userId];
    const joinedVenmoUsername = normalizeVenmoUsername(identity.venmoUsername || participant?.venmoUsername || '');
    const joinedColorSeed = `${participant?.colorSeed || identity.colorSeed || ''}`.trim();
    const summary = computeSummary();
    const myShare = summary?.perPerson?.find((person) => person.id === identity.userId);
    const shareCents = myShare ? Math.max(0, Math.round(myShare.total || 0)) : 0;
    const totalCents = summary ? Math.max(0, Math.round(summary.total || 0)) : 0;
    upsertBillHistoryEntry({
      roomCode: roomCode.toUpperCase(),
      billName: (room.name || '').trim(),
      joinedName,
      joinedVenmoUsername,
      joinedColorSeed,
      shareCents,
      currency: `${room.currency || roomCurrency || DEFAULT_CURRENCY}`.toUpperCase(),
      targetCurrency: `${room.target_currency || targetCurrency || roomCurrency || DEFAULT_CURRENCY}`.toUpperCase(),
      totalCents,
      convertedShareCents: null,
      convertedTotalCents: null,
      updatedAt: Date.now()
    });
  };

  const scheduleRoomHistoryPersist = () => {
    if (!browser) return;
    if (roomHistoryPersistTimer) clearTimeout(roomHistoryPersistTimer);
    roomHistoryPersistTimer = setTimeout(() => {
      roomHistoryPersistTimer = null;
      persistRoomHistorySnapshot();
    }, 250);
  };

  const exportSummaryPdf = async () => {
    if (!browser || !summaryData || !room) return;
    summaryExportError = null;
    summaryExporting = true;
    try {
      const { jsPDF } = await import('jspdf');
      const currentSummary = summaryData;
      const convertedCurrency = currentSummary.converted?.currency || targetCurrency;
      const formatConverted = (amount: number) =>
        formatCurrency(amount, convertedCurrency, symbolFor(convertedCurrency), exponentFor(convertedCurrency));
      const convertedPerPerson = new Map(
        (currentSummary.converted?.perPerson || []).map((entry) => [entry.id, entry.total] as const)
      );

      const doc = new jsPDF({ unit: 'pt', format: 'a4' });
      const pageWidth = doc.internal.pageSize.getWidth();
      const pageHeight = doc.internal.pageSize.getHeight();
      const marginX = 34;
      const marginY = 34;
      const contentWidth = pageWidth - marginX * 2;
      let y = marginY;

      type Rgb = [number, number, number];
      const palette = {
        page: [244, 248, 255] as Rgb,
        hero: [9, 21, 44] as Rgb,
        heroBorder: [25, 48, 86] as Rgb,
        heroAccent: [34, 211, 238] as Rgb,
        card: [255, 255, 255] as Rgb,
        cardBorder: [212, 226, 246] as Rgb,
        lineSoft: [225, 236, 249] as Rgb,
        textStrong: [14, 31, 56] as Rgb,
        textMuted: [79, 104, 136] as Rgb,
        textWhite: [246, 251, 255] as Rgb,
        textWhiteMuted: [168, 190, 219] as Rgb,
        cyan: [53, 217, 239] as Rgb,
        success: [11, 138, 90] as Rgb,
        danger: [171, 45, 78] as Rgb
      };

      const setText = (size: number, style: 'normal' | 'bold', color: Rgb) => {
        doc.setFont('helvetica', style);
        doc.setFontSize(size);
        doc.setTextColor(color[0], color[1], color[2]);
      };

      const drawRoundedCard = (x: number, top: number, w: number, h: number, fill: Rgb, border: Rgb, radius = 12) => {
        doc.setFillColor(fill[0], fill[1], fill[2]);
        doc.setDrawColor(border[0], border[1], border[2]);
        doc.setLineWidth(1);
        doc.roundedRect(x, top, w, h, radius, radius, 'FD');
      };

      let activeSectionLabel = 'Summary';
      const drawContinuationHeader = (label: string) => {
        const stripHeight = 24;
        drawRoundedCard(marginX, y, contentWidth, stripHeight, [236, 244, 255], [206, 223, 247], 9);
        setText(10, 'bold', palette.textMuted);
        doc.text(`${label} (continued)`, marginX + 12, y + 16);
        y += stripHeight + 12;
      };

      const newPage = (label: string) => {
        doc.addPage();
        y = marginY;
        drawContinuationHeader(label);
      };

      const ensureSpace = (needed: number, label = activeSectionLabel) => {
        activeSectionLabel = label;
        if (y + needed > pageHeight - marginY) {
          newPage(label);
        }
      };

      const drawRightText = (
        text: string,
        x: number,
        top: number,
        size: number,
        style: 'normal' | 'bold',
        color: Rgb
      ) => {
        setText(size, style, color);
        doc.text(text, x, top, { align: 'right' });
      };

      const parseHexColor = (hex: string | null | undefined, fallback: Rgb): Rgb => {
        const raw = `${hex || ''}`.trim().replace('#', '');
        const expanded = raw.length === 3 ? raw.split('').map((ch) => ch + ch).join('') : raw;
        if (!/^[0-9a-fA-F]{6}$/.test(expanded)) return fallback;
        const r = Number.parseInt(expanded.slice(0, 2), 16);
        const g = Number.parseInt(expanded.slice(2, 4), 16);
        const b = Number.parseInt(expanded.slice(4, 6), 16);
        if (![r, g, b].every((value) => Number.isFinite(value))) return fallback;
        return [r, g, b];
      };

      const now = new Date();
      doc.setFillColor(palette.page[0], palette.page[1], palette.page[2]);
      doc.rect(0, 0, pageWidth, pageHeight, 'F');

      const heroHeight = 126;
      ensureSpace(heroHeight + 16, 'Overview');
      drawRoundedCard(marginX, y, contentWidth, heroHeight, palette.hero, palette.heroBorder, 18);
      doc.setFillColor(palette.heroAccent[0], palette.heroAccent[1], palette.heroAccent[2]);
      doc.roundedRect(marginX + 1, y + 1, contentWidth - 2, 6, 6, 6, 'F');

      const totalChipWidth = 190;
      const titleMaxWidth = contentWidth - totalChipWidth - 56;
      const billName = `${room.name || 'Shared Bill'}`.trim() || 'Shared Bill';
      const billTitleLines = (doc.splitTextToSize(billName, titleMaxWidth) as string[]).slice(0, 2);
      const metaLineY = y + 66 + Math.max(0, billTitleLines.length - 1) * 20;
      setText(22, 'bold', palette.textWhite);
      billTitleLines.forEach((line, index) => {
        doc.text(line, marginX + 18, y + 34 + index * 20);
      });
      setText(10, 'normal', palette.textWhiteMuted);
      doc.text(`Bill Code ${roomCode.toUpperCase()}`, marginX + 18, metaLineY);
      doc.text(`Generated ${now.toLocaleString()}`, marginX + 18, metaLineY + 15);
      if (currentSummary.converted) {
        doc.text(
          `${roomCurrency} -> ${currentSummary.converted.currency} @ ${currentSummary.converted.rate.toFixed(4)}${currentSummary.converted.asOf ? ` (${currentSummary.converted.asOf})` : ''}`,
          marginX + 18,
          metaLineY + 30
        );
      }

      const chipX = marginX + contentWidth - totalChipWidth - 14;
      const chipY = y + 18;
      const chipHeight = 90;
      drawRoundedCard(chipX, chipY, totalChipWidth, chipHeight, [18, 35, 64], [48, 76, 116], 14);
      drawRightText('BILL TOTAL', chipX + totalChipWidth - 12, chipY + 20, 9, 'bold', palette.textWhiteMuted);
      drawRightText(
        formatAmount(currentSummary.total, roomCurrency),
        chipX + totalChipWidth - 12,
        chipY + 47,
        20,
        'bold',
        palette.textWhite
      );
      if (currentSummary.converted) {
        drawRightText(
          formatConverted(currentSummary.converted.total),
          chipX + totalChipWidth - 12,
          chipY + 68,
          10,
          'normal',
          palette.cyan
        );
      }
      y += heroHeight + 16;

      activeSectionLabel = 'Bill totals';
      const totalRows: Array<{
        label: string;
        value: number;
        converted?: number;
        negative?: boolean;
        emphasis?: boolean;
      }> = [
        { label: 'Gross items', value: currentSummary.gross, converted: currentSummary.converted?.gross },
        {
          label: 'Item discounts',
          value: currentSummary.itemDiscount,
          converted: currentSummary.converted?.itemDiscount,
          negative: true
        },
        {
          label: 'Bill-wide discount',
          value: currentSummary.billDiscount,
          converted: currentSummary.converted?.billDiscount,
          negative: true
        },
        {
          label: 'Bill subtotal',
          value: currentSummary.net,
          converted: currentSummary.converted?.net,
          emphasis: true
        },
        {
          label: 'Bill-wide non-tip charges',
          value: currentSummary.billCharges,
          converted: currentSummary.converted?.billCharges
        },
        { label: 'Tax', value: currentSummary.tax, converted: currentSummary.converted?.tax },
        { label: 'Tip', value: currentSummary.tip, converted: currentSummary.converted?.tip },
        {
          label: 'Bill total',
          value: currentSummary.total,
          converted: currentSummary.converted?.total,
          emphasis: true
        }
      ];
      const totalsHeaderHeight = 34;
      const totalsRowHeight = currentSummary.converted ? 34 : 24;
      const totalsCardHeight = totalsHeaderHeight + totalRows.length * totalsRowHeight + 10;
      ensureSpace(totalsCardHeight + 12, 'Bill totals');
      drawRoundedCard(marginX, y, contentWidth, totalsCardHeight, palette.card, palette.cardBorder, 12);
      setText(13, 'bold', palette.textStrong);
      doc.text('Bill totals', marginX + 16, y + 22);
      let rowY = y + totalsHeaderHeight;
      totalRows.forEach((row, index) => {
        if (index % 2 === 0) {
          doc.setFillColor(247, 251, 255);
          doc.rect(marginX + 1, rowY, contentWidth - 2, totalsRowHeight, 'F');
        }
        setText(10, row.emphasis ? 'bold' : 'normal', palette.textMuted);
        doc.text(row.label, marginX + 16, rowY + 15);
        const sign = row.negative ? '-' : '';
        drawRightText(
          `${sign}${formatAmount(row.value, roomCurrency)}`,
          marginX + contentWidth - 16,
          rowY + 15,
          row.emphasis ? 11 : 10,
          row.emphasis ? 'bold' : 'normal',
          row.negative ? palette.danger : palette.textStrong
        );
        if (currentSummary.converted && typeof row.converted === 'number') {
          drawRightText(
            `${sign}${formatConverted(row.converted)}`,
            marginX + contentWidth - 16,
            rowY + 28,
            9,
            'normal',
            row.negative ? palette.danger : palette.success
          );
        }
        rowY += totalsRowHeight;
      });
      y += totalsCardHeight + 14;

      activeSectionLabel = 'Per-person summary';
      setText(14, 'bold', palette.textStrong);
      ensureSpace(24, 'Per-person summary');
      doc.text('Per-person summary', marginX, y + 16);
      y += 24;

      currentSummary.perPerson.forEach((person) => {
        const accent = parseHexColor(person.color, palette.heroAccent);
        const detailRows: Array<{ label: string; value: number; negative?: boolean }> = [
          { label: 'Item share', value: person.itemsTotal },
          { label: 'Tax share', value: person.taxShare },
          { label: 'Tip share', value: person.tipShare }
        ];
        if (person.billDiscountShare > 0) {
          detailRows.splice(1, 0, {
            label: 'Bill-wide discount share',
            value: person.billDiscountShare,
            negative: true
          });
        }
        if (person.billChargesShare > 0) {
          detailRows.splice(2, 0, {
            label: 'Bill-wide non-tip charges share',
            value: person.billChargesShare
          });
        }

        setText(9, 'normal', palette.textMuted);
        const itemRows = person.items.map((item) => {
          const labelLines = doc.splitTextToSize(summaryItemShareLabel(item), contentWidth - 188) as string[];
          const lines = labelLines.length > 0 ? labelLines : [summaryItemShareLabel(item)];
          const rowHeight = Math.max(12, lines.length * 10) + 6;
          return { item, lines, rowHeight };
        });
        const itemsBlockHeight =
          itemRows.length > 0
            ? 18 + itemRows.reduce((sum, row) => sum + row.rowHeight, 0)
            : 0;
        const convertedPersonTotal = convertedPerPerson.get(person.id);
        const personHeaderHeight = convertedPersonTotal != null ? 58 : 44;
        const personDetailsHeight = detailRows.length * 20;
        const personCardHeight = 14 + personHeaderHeight + personDetailsHeight + itemsBlockHeight + 12;
        ensureSpace(personCardHeight + 10, 'Per-person summary');
        drawRoundedCard(marginX, y, contentWidth, personCardHeight, palette.card, palette.cardBorder, 12);
        doc.setFillColor(accent[0], accent[1], accent[2]);
        doc.roundedRect(marginX + 10, y + 10, 4, personCardHeight - 20, 4, 4, 'F');

        setText(13, 'bold', palette.textStrong);
        doc.text(person.name, marginX + 22, y + 28);
        drawRightText('TOTAL', marginX + contentWidth - 16, y + 16, 8, 'bold', palette.textMuted);
        drawRightText(
          formatAmount(person.total, roomCurrency),
          marginX + contentWidth - 16,
          y + 32,
          16,
          'bold',
          palette.textStrong
        );
        if (convertedPersonTotal != null) {
          drawRightText(
            formatConverted(convertedPersonTotal),
            marginX + contentWidth - 16,
            y + 46,
            9,
            'normal',
            palette.success
          );
        }

        let personY = y + personHeaderHeight;
        doc.setDrawColor(palette.lineSoft[0], palette.lineSoft[1], palette.lineSoft[2]);
        doc.line(marginX + 16, personY, marginX + contentWidth - 16, personY);
        personY += 14;
        detailRows.forEach((row) => {
          setText(10, 'normal', palette.textMuted);
          doc.text(row.label, marginX + 22, personY);
          const sign = row.negative ? '-' : '';
          drawRightText(
            `${sign}${formatAmount(row.value, roomCurrency)}`,
            marginX + contentWidth - 16,
            personY,
            10,
            row.negative ? 'normal' : 'bold',
            row.negative ? palette.danger : palette.textStrong
          );
          personY += 20;
        });

        if (itemRows.length > 0) {
          setText(9, 'bold', palette.textMuted);
          doc.text('Item shares', marginX + 22, personY);
          personY += 12;
          itemRows.forEach((row) => {
            setText(9, 'normal', palette.textMuted);
            row.lines.forEach((line, lineIndex) => {
              const prefix = lineIndex === 0 ? '• ' : '  ';
              doc.text(`${prefix}${line}`, marginX + 22, personY + lineIndex * 10);
            });
            drawRightText(
              formatAmount(row.item.share_cents, roomCurrency),
              marginX + contentWidth - 16,
              personY,
              9,
              'bold',
              palette.textStrong
            );
            personY += row.rowHeight;
          });
        }

        y += personCardHeight + 10;
      });

      ensureSpace(24, 'Summary');
      doc.setDrawColor(palette.lineSoft[0], palette.lineSoft[1], palette.lineSoft[2]);
      doc.line(marginX, y, marginX + contentWidth, y);
      setText(8, 'normal', palette.textMuted);
      doc.text('Generated by Divvi', marginX, y + 13);

      const safeStem =
        (room.name || 'bill-summary')
          .trim()
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, '-')
          .replace(/^-+|-+$/g, '') || 'bill-summary';
      const filename = `${safeStem}-${roomCode.toLowerCase()}-${now.toISOString().slice(0, 10)}.pdf`;
      const blob = doc.output('blob');
      const file = new File([blob], filename, { type: 'application/pdf' });
      const nav = navigator as Navigator & {
        share?: (data: ShareData) => Promise<void>;
        canShare?: (data: ShareData) => boolean;
      };

      const shouldUseNativeShare =
        !!nav.share &&
        !!nav.canShare &&
        nav.maxTouchPoints > 0 &&
        nav.canShare({ files: [file] });

      if (shouldUseNativeShare) {
        const shareResult = await Promise.race([
          nav
            .share!({
              title: `${room.name || 'Bill'} summary`,
              text: `Summary for ${room.name || 'bill'}`,
              files: [file]
            })
            .then(() => 'shared' as const)
            .catch((error) => {
              if (error instanceof Error && error.name === 'AbortError') {
                return 'aborted' as const;
              }
              return 'failed' as const;
            }),
          new Promise<'timeout'>((resolve) => setTimeout(() => resolve('timeout'), 20000))
        ]);
        if (shareResult === 'shared' || shareResult === 'aborted') {
          return;
        }
      }

      {
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        link.remove();
        setTimeout(() => URL.revokeObjectURL(url), 2000);
      }
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') {
        return;
      }
      console.error('summary pdf export failed', err);
      summaryExportError = 'Could not export PDF. Please try again.';
    } finally {
      summaryExporting = false;
    }
  };

  const cloneForReceiptReview = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

  const captureReceiptReviewSnapshot = (): ReceiptReviewSnapshot => ({
    receiptResult: receiptResult ? cloneForReceiptReview(receiptResult) : null,
    editableItems: cloneForReceiptReview(editableItems),
    editableItemReviewFlags: cloneForReceiptReview(editableItemReviewFlags),
    receiptWarnings: [...receiptWarnings],
    warningBanner,
    detectedCurrency,
    receiptCurrencySelection,
    parsedTaxInput,
    parsedTipInput,
    parsedBillDiscountInput,
    parsedBillChargesInput,
    showReceiptFlaggedOnly,
    receiptReviewFocusIndex
  });

  const applyReceiptReviewSnapshot = (snapshot: ReceiptReviewSnapshot) => {
    receiptResult = snapshot.receiptResult ? cloneForReceiptReview(snapshot.receiptResult) : null;
    editableItems = cloneForReceiptReview(snapshot.editableItems);
    editableItemReviewFlags = cloneForReceiptReview(snapshot.editableItemReviewFlags);
    receiptWarnings = [...snapshot.receiptWarnings];
    warningBanner = snapshot.warningBanner;
    detectedCurrency = snapshot.detectedCurrency;
    receiptCurrencySelection = snapshot.receiptCurrencySelection;
    parsedTaxInput = snapshot.parsedTaxInput;
    parsedTipInput = snapshot.parsedTipInput;
    parsedBillDiscountInput = snapshot.parsedBillDiscountInput;
    parsedBillChargesInput = snapshot.parsedBillChargesInput;
    showReceiptFlaggedOnly = snapshot.showReceiptFlaggedOnly;
    receiptReviewFocusIndex = snapshot.receiptReviewFocusIndex;
    showReceiptReview = true;
  };

  const clearReceiptRetryState = () => {
    receiptRetryConsumed = false;
    receiptOriginalSnapshot = null;
    receiptRetrySnapshot = null;
    receiptUsingRetryResult = false;
    receiptRetryStatus = null;
  };

  const swapReceiptRetrySource = () => {
    if (!receiptRetryConsumed) return;
    if (receiptUsingRetryResult) {
      if (!receiptOriginalSnapshot) return;
      applyReceiptReviewSnapshot(receiptOriginalSnapshot);
      receiptUsingRetryResult = false;
      receiptRetryStatus = 'Restored original parsed result.';
      return;
    }
    if (!receiptRetrySnapshot) return;
    applyReceiptReviewSnapshot(receiptRetrySnapshot);
    receiptUsingRetryResult = true;
    receiptRetryStatus = 'Using try-again parsed result.';
  };

  const parseReceiptFile = async (
    sourceFile: File,
    options: { escalate?: boolean; userCropped?: boolean } = {}
  ) => {
    if (!sourceFile) return;
    const escalate = options.escalate === true;
    const userCropped = options.userCropped === true;
    if (!escalate) {
      clearReceiptRetryState();
    } else {
      receiptRetryStatus = null;
    }
    receiptError = null;
    receiptUploading = true;
    receiptTryingAgain = escalate;
    parsedTaxInput = '';
    parsedTipInput = '';
    parsedBillDiscountInput = '';
    parsedBillChargesInput = '';
    editableItemReviewFlags = [];
    setReceiptEditingIndex(null);
    showReceiptFlaggedOnly = false;
    receiptReviewFocusIndex = 0;
    try {
      const file = await normalizeReceiptImage(sourceFile);
      receiptLastUploadedFile = file;
      receiptLastUploadedCropped = userCropped;
      const form = new FormData();
      form.append('file', file);
      if (escalate) {
        form.append('parse_mode', 'accurate');
      }
      if (userCropped) {
        form.append('user_cropped', '1');
      }
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
      const {
        items: parsedItemsWithItemDiscounts,
        attachedLines: attachedItemDiscountLines
      } = attachAdjacentItemDiscountLines(Array.isArray(result.items) ? result.items : []);
      const {
        items: parsedItemsWithoutBillAdjustments,
        inferredBillDiscountCents,
        inferredBillChargesCents,
        inferredDiscountLines,
        inferredChargeLines
      } = splitBillLevelAdjustmentsFromParsedItems(parsedItemsWithItemDiscounts);
      const { items: parsedItemsForImport, foldedLines: foldedAddonLines } = foldStandaloneAddonsIntoItems(
        parsedItemsWithoutBillAdjustments
      );
      const parsedBillDiscountTotal = Math.max(0, Number(result?.bill_discount_cents ?? 0)) + inferredBillDiscountCents;
      const parsedBillChargesTotal = Math.max(0, Number(result?.bill_charges_cents ?? 0)) + inferredBillChargesCents;
      parsedTaxInput =
        result?.tax_cents != null ? (Number(result.tax_cents) / parseFactor).toFixed(parseExp) : '';
      parsedTipInput =
        result?.tip_cents != null ? (Number(result.tip_cents) / parseFactor).toFixed(parseExp) : '';
      parsedBillDiscountInput = parsedBillDiscountTotal > 0 ? (parsedBillDiscountTotal / parseFactor).toFixed(parseExp) : '';
      parsedBillChargesInput = parsedBillChargesTotal > 0 ? (parsedBillChargesTotal / parseFactor).toFixed(parseExp) : '';
      receiptWarnings = Array.isArray(result.warnings) ? result.warnings.filter(Boolean) : [];
      if (inferredDiscountLines.length > 0) {
        receiptWarnings.push(
          `Moved likely bill-wide discount lines to receipt-level discount: ${inferredDiscountLines.join(', ')}`
        );
      }
      if (attachedItemDiscountLines.length > 0) {
        receiptWarnings.push(
          `Attached likely item-level discount lines to items: ${attachedItemDiscountLines.join(', ')}`
        );
      }
      if (inferredChargeLines.length > 0) {
        receiptWarnings.push(
          `Moved likely bill-wide fee/charge lines to receipt-level charges: ${inferredChargeLines.join(', ')}`
        );
      }
      if (foldedAddonLines.length > 0) {
        receiptWarnings.push(`Folded add-on lines into their parent items: ${foldedAddonLines.join(', ')}`);
      }
      // Warnings mean "review recommended", not necessarily a broken import.
      warningBanner = receiptWarnings.length > 0 || (typeof result.confidence === 'number' && result.confidence < 0.6);
      const agg = new Map<
        string,
        {
          name: string;
          qty: number;
          unit: number;
          discount: number;
          discountPct: number;
          line: number;
          addons: ItemFormAddon[];
          review: EditableItemReviewFlags;
        }
      >();
      parsedItemsForImport.forEach((item) => {
        const review = reviewFlagsFromParsedItem(item);
        const qty = item.quantity && item.quantity > 0 ? item.quantity : 1;
        const baseName = normalizeReceiptLineLabel(item.name || '') || 'Item';
        const parsedAddons = (item.addons || [])
          .map((addon) => {
            const label = normalizeAddonLabel(addon?.name || addon?.raw_text || '');
            const price = addon?.price_cents != null ? Number(addon.price_cents) / parseFactor : 0;
            return {
              name: label,
              price: price > 0 ? price.toFixed(parseExp) : ''
            };
          })
          .filter((addon) => addon.name || addon.price);
        const normalizedParsedAddons = normalizedFormAddons(parsedAddons, receiptCurrencySelection);
        const addonSig = addonsSignature(normalizedParsedAddons);
        // Receipt parse values are in minor units for the detected currency.
        let unit = item.unit_price_cents != null ? item.unit_price_cents / parseFactor : 0;
        let line = item.line_price_cents != null ? item.line_price_cents / parseFactor : 0;
        if (!unit && line && qty > 0) unit = line / qty;
        if (!line && unit && qty > 0) line = unit * qty;
        const disc = item.discount_cents != null ? item.discount_cents / parseFactor : 0;
        const discPct = item.discount_percent != null ? item.discount_percent : unit ? (disc / unit) * 100 : 0;
        const key = `${baseName.trim().toLowerCase()}|${addonSig}|${unit.toFixed(parseExp)}|${disc.toFixed(parseExp)}`;
        const existing = agg.get(key);
        if (existing) {
          existing.qty += qty;
          existing.line += line;
          existing.review = mergeEditableItemReviewFlags(existing.review, review);
        } else {
          agg.set(key, {
            name: baseName,
            qty,
            unit,
            discount: disc,
            discountPct: discPct,
            line,
            addons: parsedAddons,
            review
          });
        }
      });
      const aggregated = Array.from(agg.values());
      editableItems = aggregated.map((item) => ({
        name: item.name,
        quantity: String(item.qty),
        unitPrice: item.unit ? item.unit.toFixed(parseExp) : '',
        linePrice: item.line ? item.line.toFixed(parseExp) : '',
        discountCents: item.discount ? item.discount.toFixed(parseExp) : '',
        discountPercent: item.discountPct ? item.discountPct.toFixed(2) : '',
        addons: item.addons,
        discountMode: item.discountPct && item.discountPct > 0 ? 'percent' : 'amount',
        totalInputMode: 'auto'
      }));
      editableItemReviewFlags = aggregated.map((item) => item.review);
      const flaggedCount = editableItemReviewFlags.filter((flags) => reviewFlagsNeedReview(flags)).length;
      if (flaggedCount > 0) {
        receiptWarnings.unshift(
          `${flaggedCount} item line${flaggedCount === 1 ? '' : 's'} need review (highlighted in yellow).`
        );
      }
      warningBanner =
        flaggedCount > 0 ||
        receiptWarnings.length > 0 ||
        (typeof result.confidence === 'number' && result.confidence < 0.6);
      setReceiptEditingIndex(null);
      showReceiptReview = true;
    } catch (err) {
      receiptError = err instanceof Error ? err.message : 'Receipt upload failed';
    } finally {
      receiptUploading = false;
      receiptTryingAgain = false;
    }
  };

  const openReceiptCropModal = (file: File) => {
    if (!file) return;
    receiptError = null;
    receiptCropSourceFile = file;
    showReceiptCropModal = true;
  };

  const closeReceiptCropModal = () => {
    if (receiptUploading) return;
    showReceiptCropModal = false;
    receiptCropSourceFile = null;
  };

  const confirmReceiptCrop = async (event: CustomEvent<{ file: File; cropped: boolean }>) => {
    const nextFile = event.detail?.file;
    const cropped = event.detail?.cropped === true;
    showReceiptCropModal = false;
    receiptCropSourceFile = null;
    if (!nextFile) return;
    await parseReceiptFile(nextFile, { escalate: false, userCropped: cropped });
  };

  const submitReceipt = async (event: Event) => {
    const target = event.target as HTMLInputElement;
    const file = target.files?.[0];
    target.value = '';
    if (!file) return;
    openReceiptCropModal(file);
  };

  const retryReceiptParse = async () => {
    if (!receiptLastUploadedFile || receiptUploading || receiptRetryConsumed) return;
    const beforeSnapshot = captureReceiptReviewSnapshot();
    const beforeSignature = JSON.stringify({
      currency: receiptCurrencySelection,
      tax: parsedTaxInput,
      tip: parsedTipInput,
      billDiscount: parsedBillDiscountInput,
      billCharges: parsedBillChargesInput,
      items: editableItems.map((item) => ({
        name: normalizeReceiptLineLabel(item.name || ''),
        quantity: item.quantity || '',
        unitPrice: item.unitPrice || '',
        linePrice: item.linePrice || '',
        discountCents: item.discountCents || '',
        discountPercent: item.discountPercent || '',
        addons: (item.addons || []).map((addon) => ({
          name: normalizeAddonLabel(addon?.name || ''),
          price: addon?.price || ''
        }))
      }))
    });
    receiptRetryConsumed = true;
    await parseReceiptFile(receiptLastUploadedFile, {
      escalate: true,
      userCropped: receiptLastUploadedCropped
    });
    if (receiptError) {
      const errorMessage = receiptError;
      receiptOriginalSnapshot = beforeSnapshot;
      receiptRetrySnapshot = null;
      receiptUsingRetryResult = false;
      applyReceiptReviewSnapshot(beforeSnapshot);
      receiptError = null;
      receiptRetryStatus = errorMessage
        ? `Try again failed (${errorMessage}). Kept original parsed result.`
        : 'Try again failed. Kept original parsed result.';
      return;
    }
    const afterSnapshot = captureReceiptReviewSnapshot();
    receiptOriginalSnapshot = beforeSnapshot;
    receiptRetrySnapshot = afterSnapshot;
    receiptUsingRetryResult = true;
    const afterSignature = JSON.stringify({
      currency: receiptCurrencySelection,
      tax: parsedTaxInput,
      tip: parsedTipInput,
      billDiscount: parsedBillDiscountInput,
      billCharges: parsedBillChargesInput,
      items: editableItems.map((item) => ({
        name: normalizeReceiptLineLabel(item.name || ''),
        quantity: item.quantity || '',
        unitPrice: item.unitPrice || '',
        linePrice: item.linePrice || '',
        discountCents: item.discountCents || '',
        discountPercent: item.discountPercent || '',
        addons: (item.addons || []).map((addon) => ({
          name: normalizeAddonLabel(addon?.name || ''),
          price: addon?.price || ''
        }))
      }))
    });
    receiptRetryStatus =
      beforeSignature === afterSignature
        ? 'Try again finished with the same parsed result.'
        : 'Try again applied updated parsed lines.';
  };

  const recalcDerived = (
    index: number,
    changed: 'quantity' | 'unitPrice' | 'linePrice' | 'discount' | 'discountMode' | 'totalInputMode'
  ) => {
    editableItems = editableItems.map((item, i) => {
      if (i !== index) return item;

      const qtyVal = Number.parseFloat(item.quantity || '');
      const hasQty = Number.isFinite(qtyVal) && qtyVal > 0;
      const quantity = hasQty ? qtyVal : null;

      const code = receiptCurrencySelection || roomCurrency;
      const exp = exponentFor(code);
      const factor = factorFor(code);
      let unitPrice = Number.parseFloat(item.unitPrice || '');
      let linePrice = Number.parseFloat(item.linePrice || '');
      let discountCents = Number.parseFloat(item.discountCents || '');
      let discountPercent = Number.parseFloat(item.discountPercent || '');
      const discountMode =
        item.discountMode || ((Number.parseFloat(item.discountPercent || '0') || 0) > 0 ? 'percent' : 'amount');
      const totalInputMode = item.totalInputMode || 'auto';

      if (!Number.isFinite(discountCents)) discountCents = 0;
      if (!Number.isFinite(discountPercent)) discountPercent = 0;

      const activeDiscountRaw = discountMode === 'percent' ? item.discountPercent || '' : item.discountCents || '';
      if (changed === 'discount' && (activeDiscountRaw.trim() === '' || Number(activeDiscountRaw) === 0)) {
        discountCents = 0;
        discountPercent = 0;
      }

      if (changed === 'quantity' && quantity !== null) {
        if (unitPrice) {
          linePrice = hasQty ? unitPrice * quantity : linePrice;
        } else if (linePrice && hasQty) {
          unitPrice = linePrice / quantity;
        }
      } else if (changed === 'unitPrice' && hasQty && unitPrice) {
        linePrice = unitPrice * quantity;
      } else if (
        (changed === 'linePrice' || (totalInputMode === 'manual' && changed === 'quantity')) &&
        hasQty &&
        linePrice
      ) {
        unitPrice = linePrice / quantity;
      } else if (!linePrice && hasQty && unitPrice) {
        linePrice = unitPrice * quantity;
      } else if (!unitPrice && hasQty && linePrice) {
        unitPrice = linePrice / quantity;
      }

      if (totalInputMode === 'auto' && hasQty && unitPrice) {
        linePrice = unitPrice * quantity;
      }

      if (unitPrice) {
        if (discountMode === 'percent') {
          if (discountPercent > 0) {
            discountCents = unitPrice * (discountPercent / 100);
          } else if (discountCents > 0) {
            discountPercent = (discountCents / unitPrice) * 100;
          }
        } else if (discountCents > 0) {
          discountPercent = (discountCents / unitPrice) * 100;
        } else if (discountPercent > 0) {
          discountCents = unitPrice * (discountPercent / 100);
        }
      }

      const fmt = (v: number) => (Number.isFinite(v) && v !== 0 ? v.toFixed(exp) : '');
      const next = { ...item };
      if (quantity !== null) next.quantity = String(quantity); // leave as-is if user cleared
      next.unitPrice = changed === 'unitPrice' ? next.unitPrice : fmt(unitPrice);
      next.linePrice = changed === 'linePrice' ? next.linePrice : fmt(linePrice);
      if (!(changed === 'discount' && discountMode === 'amount')) {
        next.discountCents = fmt(discountCents);
      }
      if (!(changed === 'discount' && discountMode === 'percent')) {
        next.discountPercent = fmt(discountPercent);
      }
      next.discountMode = discountMode;
      next.totalInputMode = totalInputMode;
      return next;
    });
  };

  const editableItemDiscountMode = (item: (typeof editableItems)[number]) =>
    item.discountMode || ((Number.parseFloat(item.discountPercent || '0') || 0) > 0 ? 'percent' : 'amount');

  const editableItemTotalInputMode = (item: (typeof editableItems)[number]) => item.totalInputMode || 'auto';

  const setEditableItemField = (
    index: number,
    field: 'name' | 'quantity' | 'unitPrice' | 'linePrice' | 'discountCents' | 'discountPercent',
    value: string
  ) => {
    editableItems = editableItems.map((item, i) => (i === index ? { ...item, [field]: value } : item));
  };

  const setEditableItemDiscountMode = (index: number, mode: 'amount' | 'percent') => {
    editableItems = editableItems.map((item, i) =>
      i === index ? { ...item, discountMode: mode } : item
    );
    queueMicrotask(() => recalcDerived(index, 'discountMode'));
  };

  const toggleEditableItemTotalInputMode = (index: number) => {
    editableItems = editableItems.map((item, i) =>
      i === index
        ? { ...item, totalInputMode: editableItemTotalInputMode(item) === 'auto' ? 'manual' : 'auto' }
        : item
    );
    queueMicrotask(() => recalcDerived(index, 'totalInputMode'));
  };

  const pricingPreviewFromEditable = (item: (typeof editableItems)[number]) => {
    const code = receiptCurrencySelection || roomCurrency;
    const qty = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
    const unit = toCentsInput(item.unitPrice, code);
    const explicitTotal = toCentsInput(item.linePrice, code);
    const baseTotal = explicitTotal || (unit ? unit * qty : 0);
    const addonPerItem = (item.addons || []).reduce(
      (sum, addon) => sum + toCentsInput(addon?.price || '', code),
      0
    );
    const addonTotal = addonPerItem * qty;
    const grossTotal = Math.max(0, baseTotal + addonTotal);
    return { qty, unit, baseTotal, addonPerItem, addonTotal, grossTotal };
  };

  const confirmReceipt = () => {
    if (!receiptResult || !ws) return;
    const toCents = (val: string) => parseByCurrency(val, receiptCurrencySelection || roomCurrency);
    const aggregated = new Map<
      string,
      {
        name: string;
        qty: number;
        unit: number;
        line: number;
        disc: number;
        discPct: number;
        addons: Array<{ name: string; price_cents: number }>;
      }
    >();

    editableItems.forEach((item) => {
      const quantity = Math.max(1, Number.parseInt(item.quantity || '1', 10) || 1);
      let unitPriceCents = toCents(item.unitPrice);
      let linePriceCents = toCents(item.linePrice);
      if (!unitPriceCents && linePriceCents && quantity > 0) {
        unitPriceCents = Math.round(linePriceCents / quantity);
      }
      if (!linePriceCents && unitPriceCents) {
        linePriceCents = unitPriceCents * quantity;
      }
      let discountCents = toCents(item.discountCents); // per-unit
      const discountPercent = Number.parseFloat(item.discountPercent || '0') || 0;
      const addons = normalizedFormAddons(item.addons || [], receiptCurrencySelection);
      const addonPerItemCents = addons.reduce((sum, addon) => sum + Math.max(0, addon.price_cents || 0), 0);
      const unitGrossCents = Math.max(0, unitPriceCents + addonPerItemCents);
      const lineGrossCents = Math.max(0, linePriceCents + addonPerItemCents * quantity);
      const addonSig = addonsSignature(addons);
      if (!discountCents && discountPercent && unitPriceCents) {
        discountCents = Math.round(unitPriceCents * (discountPercent / 100));
      }
      const normalizedName = normalizeReceiptLineLabel(item.name || '') || 'Item';
      const key = `${normalizedName.trim().toLowerCase()}|${addonSig}|${unitGrossCents}|${lineGrossCents}|${discountCents}|${discountPercent}`;
      const existing = aggregated.get(key);
      if (existing) {
        existing.qty += quantity;
        existing.line = existing.line + lineGrossCents;
      } else {
        aggregated.set(key, {
          name: normalizedName,
          qty: quantity,
          unit: unitGrossCents,
          line: lineGrossCents,
          disc: discountCents,
          discPct: discountPercent,
          addons
        });
      }
    });

    const importItems: {
      name: string;
      unit: number;
      line: number;
      disc: number;
      discPct: number;
      addons: Array<{ name: string; price_cents: number }>;
      sortOrder: number;
    }[] = [];
    let nextImportSortOrder = nextStandaloneSortOrder(items);
    Array.from(aggregated.values()).forEach((agg) => {
      // show aggregated qty in review, but fan out into single-quantity items when importing
      const unitLine = Math.round((agg.line || agg.unit * agg.qty) / agg.qty) || agg.unit;
      const groupSortOrderBase = nextImportSortOrder;
      for (let i = 0; i < agg.qty; i++) {
        importItems.push({
          name: numberedGeneratedItemName(agg.name, i, agg.qty),
          unit: agg.unit || unitLine,
          line: unitLine,
          disc: Math.max(0, Math.min(unitLine, agg.disc)),
          discPct: agg.discPct,
          addons: agg.addons,
          sortOrder: groupSortOrderBase + i
        });
      }
      nextImportSortOrder += SORT_ORDER_STEP;
    });

    importItems.forEach((it, idx) => {
      const itemId = `${Date.now()}-${idx}-${Math.random().toString(36).slice(2, 6)}`;
      const meta = it.addons.length > 0 ? { addons: it.addons } : undefined;
      upsertItem({
        id: itemId,
        name: it.name,
        quantity: 1,
        unit_price_cents: it.unit,
        line_price_cents: it.line,
        discount_cents: it.disc,
        discount_percent: it.discPct,
        assigned: {},
        sort_order: it.sortOrder,
        ...(meta ? { meta } : {})
      });
    });
    showReceiptReview = false;
    const taxDelta = parsedTaxCents;
    const tipDelta = parsedTipCents;
    const billDiscountDelta = parseByCurrency(parsedBillDiscountInput, receiptCurrencySelection || roomCurrency);
    const billChargesDelta = parseByCurrency(parsedBillChargesInput, receiptCurrencySelection || roomCurrency);
    if (ws && room) {
      if (receiptCurrencySelection && receiptCurrencySelection !== roomCurrency) {
        changeCurrency(receiptCurrencySelection);
      }
      const payload = {
        tax_cents: taxDelta,
        tip_cents: tipDelta,
        bill_discount_cents: billDiscountDelta,
        bill_charges_cents: billChargesDelta
      };
      ws.send(
        JSON.stringify({
          type: 'op',
          op: { kind: 'set_tax_tip', actor_id: identity.userId, payload }
        })
      );
      applyLocalOp({ kind: 'set_tax_tip', payload, timestamp: Date.now() });
      syncBillSettingsInputsFromRoom();
    }
    receiptResult = null;
    editableItems = [];
    setReceiptEditingIndex(null);
    showReceiptItemAddonsModal = false;
    receiptItemAddonsIndex = null;
    showReceiptAttachModal = false;
    receiptAttachSourceIndex = null;
    parsedTaxInput = '';
    parsedTipInput = '';
    parsedBillDiscountInput = '';
    parsedBillChargesInput = '';
    receiptWarnings = [];
    warningBanner = false;
    detectedCurrency = null;
    receiptCurrencySelection = (roomCurrency || DEFAULT_CURRENCY).toUpperCase();
    clearReceiptRetryState();
  };

  const parseCents = (value: string) => {
    return parseByCurrency(value, roomCurrency);
  };

  const recalcItemForm = (changed: 'quantity' | 'unitPrice' | 'linePrice' | 'discount' | 'discountMode') => {
    const exp = exponentFor(roomCurrency);
    const qtyVal = Number.parseFloat(itemForm.quantity || '');
    const hasQty = Number.isFinite(qtyVal) && qtyVal > 0;
    const quantity = hasQty ? qtyVal : null;

    let unitPrice = Number.parseFloat(itemForm.unitPrice || '');
    let linePrice = Number.parseFloat(itemForm.linePrice || '');
    const discountCentsRaw = itemForm.discountCents || '';
    const discountPercentRaw = itemForm.discountPercent || '';
    let discountCents = Number.parseFloat(discountCentsRaw);
    let discountPercent = Number.parseFloat(discountPercentRaw);

    if (!Number.isFinite(discountCents)) discountCents = 0;
    if (!Number.isFinite(discountPercent)) discountPercent = 0;

    const activeDiscountRaw = itemFormDiscountMode === 'percent' ? discountPercentRaw : discountCentsRaw;
    if (changed === 'discount' && (activeDiscountRaw.trim() === '' || Number(activeDiscountRaw) === 0)) {
      discountCents = 0;
      discountPercent = 0;
    }

    if (changed === 'quantity' && quantity !== null) {
      if (unitPrice) {
        linePrice = unitPrice * quantity; // keep gross unit
      } else if (linePrice) {
        unitPrice = linePrice / quantity;
      }
    } else if (changed === 'unitPrice' && hasQty && unitPrice) {
      linePrice = unitPrice * quantity!;
    } else if (
      (changed === 'linePrice' || (itemFormTotalInputMode === 'manual' && changed === 'quantity')) &&
      hasQty &&
      linePrice
    ) {
      unitPrice = linePrice / quantity!;
    } else if (!linePrice && hasQty && unitPrice) {
      linePrice = unitPrice * quantity!;
    } else if (!unitPrice && hasQty && linePrice) {
      unitPrice = linePrice / quantity!;
    }

    if (itemFormTotalInputMode === 'auto' && hasQty && unitPrice) {
      linePrice = unitPrice * quantity!;
    }

    if (unitPrice) {
      if (itemFormDiscountMode === 'percent') {
        if (discountPercent > 0) {
          discountCents = unitPrice * (discountPercent / 100);
        } else if (discountCents > 0) {
          discountPercent = unitPrice ? (discountCents / unitPrice) * 100 : 0;
        }
      } else if (discountCents > 0) {
        discountPercent = unitPrice ? (discountCents / unitPrice) * 100 : 0;
      } else if (discountPercent > 0) {
        discountCents = unitPrice * (discountPercent / 100);
      }
    }

    const fmt = (v: number) => (Number.isFinite(v) && v !== 0 ? v.toFixed(exp) : '');
    const next = { ...itemForm };
    if (quantity !== null) next.quantity = String(quantity); // allow user to clear
    next.unitPrice = changed === 'unitPrice' ? next.unitPrice : fmt(unitPrice);
    next.linePrice = changed === 'linePrice' ? next.linePrice : fmt(linePrice);
    if (!(changed === 'discount' && itemFormDiscountMode === 'amount')) {
      next.discountCents = fmt(discountCents);
    }
    if (!(changed === 'discount' && itemFormDiscountMode === 'percent')) {
      next.discountPercent = fmt(discountPercent);
    }
    itemForm = next;
  };

  const setItemFormDiscountMode = (mode: 'amount' | 'percent') => {
    if (itemFormDiscountMode === mode) return;
    itemFormDiscountMode = mode;
    queueMicrotask(() => recalcItemForm('discountMode'));
  };

  const buildItemFormFromRepresentative = (
    item: Item,
    quantityOverride = Math.max(1, item.quantity || 1),
    stripGeneratedSuffix = false
  ) => {
    const metaAddons = formAddonsFromItemMeta(item);
    const addonTotalPerItemCents = metaAddons.reduce(
      (sum, addon) => sum + parseByCurrency(addon.price || '', roomCurrency),
      0
    );
    const parsed = splitNameAndAddonLabels(item.name || '');
    const baseName = stripGeneratedSuffix
      ? stripGeneratedItemNumberSuffix(parsed.baseName || item.name || '')
      : parsed.baseName || item.name || '';
    const inferredAddons = parsed.addonLabels.map((label) => ({ name: label, price: '' }));
    const addons = metaAddons.length ? metaAddons : inferredAddons;
    const exp = exponentFor(roomCurrency);
    const factor = factorFor(roomCurrency);
    const totalGrossCents = Math.max(0, Number(item.line_price_cents || 0)) * quantityOverride;
    const unitForFormCents = Math.max(0, (item.unit_price_cents || 0) - addonTotalPerItemCents);
    const lineForFormCents = Math.max(0, totalGrossCents - addonTotalPerItemCents * quantityOverride);
    return {
      name: baseName || item.name,
      quantity: String(quantityOverride),
      unitPrice: (Math.max(0, unitForFormCents) / factor).toFixed(exp),
      linePrice: (Math.max(0, lineForFormCents) / factor).toFixed(exp),
      discountCents: (Math.max(0, item.discount_cents || 0) / factor).toFixed(exp),
      discountPercent: Math.max(0, item.discount_percent || 0).toFixed(2),
      addons
    };
  };

  const openGroupEditModal = (group: RepeatedGroup) => {
    const representative = group.items[0];
    if (!representative) return;
    itemModalMode = 'group';
    itemModalId = null;
    itemModalGroupKey = group.key;
    showItemAddonsModal = false;
    itemFormDiscountMode = representative.discount_percent && representative.discount_percent > 0 ? 'percent' : 'amount';
    itemFormTotalInputMode = 'auto';
    itemForm = buildItemFormFromRepresentative(representative, group.items.length, true);
    showItemModal = true;
  };

  const repeatedGroupRemovalCandidates = (groupItems: Item[], removeCount: number) =>
    [...groupItems]
      .sort((left, right) => {
        const countDelta = servingAssignedCount(left) - servingAssignedCount(right);
        if (countDelta !== 0) return countDelta;
        return itemSortOrderOr(right, -1) - itemSortOrderOr(left, -1);
      })
      .slice(0, Math.max(0, removeCount));

  const openNewItemModal = () => {
    itemModalMode = 'new';
    itemModalId = null;
    itemModalGroupKey = null;
    showItemAddonsModal = false;
    itemFormDiscountMode = 'amount';
    itemFormTotalInputMode = 'auto';
    itemForm = {
      name: '',
      quantity: '1',
      unitPrice: '0',
      linePrice: '0',
      discountCents: '0',
      discountPercent: '0',
      addons: []
    };
    showItemModal = true;
  };

  const openEditItemModal = (item: Item) => {
    itemModalMode = 'edit';
    itemModalId = item.id;
    itemModalGroupKey = null;
    showItemAddonsModal = false;
    itemFormDiscountMode = item.discount_percent && item.discount_percent > 0 ? 'percent' : 'amount';
    itemFormTotalInputMode = 'auto';
    itemForm = buildItemFormFromRepresentative(item);
    showItemModal = true;
  };

  const submitItemModal = () => {
    const rawBaseName = splitNameAndAddonLabels(itemForm.name || '').baseName.trim();
    const baseName =
      itemModalMode === 'group' || itemModalMode === 'new'
        ? stripGeneratedItemNumberSuffix(rawBaseName)
        : rawBaseName;
    if (!baseName) return;
    const quantity =
      itemModalMode === 'edit' ? 1 : Math.max(1, Number.parseFloat(itemForm.quantity || '1') || 1);
    const unitPriceCents = parseCents(itemForm.unitPrice);
    const discountCentsRaw = (itemForm.discountCents || '').trim();
    const discountPercentRaw = (itemForm.discountPercent || '').trim();
    const addons = normalizedFormAddons(itemForm.addons || []);
    const addonTotalCents = addons.reduce((sum, addon) => sum + Math.max(0, addon.price_cents || 0), 0);
    const itemName = baseName;

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
      linePriceCents = unitPriceCents * quantity;
    }
    const perItemGrossBase = linePriceCents ? Math.round(linePriceCents / quantity) : unitPriceCents;
    const perItemGross = Math.max(0, perItemGrossBase + addonTotalCents);
    const perItemUnit = Math.max(0, (unitPriceCents || perItemGrossBase) + addonTotalCents);
    const buildItem = (id: string, name: string, sortOrder: number | null, source?: Item) => {
      const assigned = source?.assigned ? { ...source.assigned } : {};
      const existingMeta = source?.meta ? { ...source.meta } : {};
      if (addons.length > 0) {
        existingMeta.addons = addons;
      } else if (existingMeta.addons) {
        delete existingMeta.addons;
      }
      if (itemModalMode === 'group' && existingMeta.repeated_group_excluded) {
        delete existingMeta.repeated_group_excluded;
      }
      const meta = Object.keys(existingMeta).length ? existingMeta : undefined;
      return normalizeItem({
        ...(source || {}),
        id,
        name,
        quantity: 1,
        unit_price_cents: perItemUnit || perItemGross,
        line_price_cents: perItemGross || perItemUnit,
        discount_cents: discountCents,
        discount_percent: discountPercent,
        assigned,
        sort_order: sortOrder,
        ...(meta ? { meta } : {})
      });
    };

    if (itemModalMode === 'group' && itemModalGroupKey) {
      const group = repeatedGroupsByKey[itemModalGroupKey];
      if (!group) return;
      const orderedGroupItems = sortItemsByOrder(group.items);
      let keptItems = [...orderedGroupItems];
      let removedItemsForShrink: Item[] = [];
      if (quantity < orderedGroupItems.length) {
        removedItemsForShrink = repeatedGroupRemovalCandidates(orderedGroupItems, orderedGroupItems.length - quantity);
        const removedIds = new Set(removedItemsForShrink.map((item) => item.id));
        const removedLabels = removedItemsForShrink
          .map((item) => {
            const servingIndex = orderedGroupItems.findIndex((candidate) => candidate.id === item.id);
            return `#${servingIndex + 1} (${servingAssignedCount(item)} assigned)`;
          })
          .join(', ');
        const hasAssignedServings = removedItemsForShrink.some((item) => servingAssignedCount(item) > 0);
        if (hasAssignedServings && browser) {
          const confirmed = window.confirm(
            `Reducing this group to ${quantity} serving${quantity === 1 ? '' : 's'} will remove ${removedItemsForShrink.length} serving${removedItemsForShrink.length === 1 ? '' : 's'}: ${removedLabels}. Continue?`
          );
          if (!confirmed) return;
        }
        keptItems = orderedGroupItems.filter((item) => !removedIds.has(item.id));
      }

      keptItems.forEach((item, index) => {
        upsertItem(
          buildItem(
            item.id,
            numberedGeneratedItemName(itemName, index, quantity),
            item.sort_order ?? null,
            item
          )
        );
      });

      if (quantity > keptItems.length) {
        const lastOrder = keptItems.length
          ? itemSortOrderOr(keptItems[keptItems.length - 1], nextStandaloneSortOrder(items) - 1)
          : itemSortOrderOr(orderedGroupItems[orderedGroupItems.length - 1], nextStandaloneSortOrder(items) - 1);
        for (let i = keptItems.length; i < quantity; i++) {
          const source = keptItems[0] || orderedGroupItems[0];
          const itemId = `${Date.now()}-group-${i}-${Math.random().toString(36).slice(2, 6)}`;
          upsertItem(
            buildItem(
              itemId,
              numberedGeneratedItemName(itemName, i, quantity),
              lastOrder + (i - keptItems.length + 1),
              source
            )
          );
        }
      }

      if (removedItemsForShrink.length > 0) {
        removeItems(removedItemsForShrink.map((item) => item.id));
      }
    } else if (itemModalMode === 'edit' && itemModalId) {
      const existingItem = room?.items?.[itemModalId];
      if (!existingItem) return;
      upsertItem(buildItem(itemModalId, itemName, existingItem.sort_order ?? null, existingItem));
    } else {
      const baseSortOrder = nextStandaloneSortOrder(items);
      for (let i = 0; i < quantity; i++) {
        upsertItem(
          buildItem(
          `${Date.now()}-${i}-${Math.random().toString(36).slice(2, 6)}`,
            numberedGeneratedItemName(itemName, i, quantity),
            baseSortOrder + i
          )
        );
      }
    }
    showItemAddonsModal = false;
    showItemModal = false;
    itemModalGroupKey = null;
    // ensure we converge with server after batch sends
    requestSnapshot();
  };

  const duplicateItem = (item: Item) => {
    openEditItemModal(item);
    itemModalMode = 'new';
    itemModalId = null;
  };

  const sendParticipantUpdate = (
    id: string,
    name: string,
    present: boolean,
    venmoUsername?: string | null
  ) => {
    if (!name.trim()) return;

    const normalizedName = name.trim();
    const nameKey = normalizedName.toLowerCase();
    const existingMatch = Object.entries(room?.participants || {}).find(
      ([, p]) => (p as Participant).name.trim().toLowerCase() === nameKey
    );
    const targetId = existingMatch ? existingMatch[0] : id;
    const existingParticipant = room?.participants?.[targetId];
    const colorSeed =
      existingParticipant?.colorSeed ||
      hexSeed(targetId) ||
      fallbackSeed(targetId, normalizedName);
    const normalizedVenmoUsername =
      venmoUsername === undefined
        ? normalizeVenmoUsername(existingParticipant?.venmoUsername || '')
        : normalizeVenmoUsername(venmoUsername);

    const participant = {
      id: targetId,
      name: normalizedName,
      initials: initialsFromName(normalizedName),
      color_seed: colorSeed,
      colorSeed,
      venmo_username: normalizedVenmoUsername,
      venmoUsername: normalizedVenmoUsername,
      present,
      finished: existingParticipant?.finished ?? false
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
        colorSeed: participant.colorSeed,
        venmoUsername: participant.venmoUsername
      };
      localStorage.setItem(`room:${roomCode}:identity`, JSON.stringify(identity));
    }
  };

  const sendPresence = (present: boolean) => {
    if (!room) return;
    const stored = localStorage.getItem(`room:${roomCode}:identity`);
    if (!stored) return;
    const id = JSON.parse(stored);
    sendParticipantUpdate(id.userId, id.name, present, id.venmoUsername);
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

  const toggleMyFinished = () => {
    const p = room?.participants?.[identity.userId];
    if (!p) return;
    const participant = {
      id: p.id,
      name: p.name,
      initials: p.initials,
      color_seed: p.colorSeed,
      colorSeed: p.colorSeed,
      venmo_username: p.venmoUsername || '',
      venmoUsername: p.venmoUsername || '',
      present: p.present,
      finished: !p.finished
    };
    sendOp({
      kind: 'set_participant',
      actor_id: identity.userId,
      payload: { participant }
    });
    applyLocalOp({ kind: 'set_participant', payload: { participant } });
  };

  const removeItem = (itemId: string) => {
    removeItems([itemId]);
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
        body: JSON.stringify({
          room_code: roomCode.toUpperCase(),
          name: joinNameInput.trim(),
          venmo_username: normalizeVenmoUsername(joinVenmoInput)
        })
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
        colorSeed: `${data.color_seed || hexSeed(data.user_id)}`,
        venmoUsername: normalizeVenmoUsername(joinVenmoInput)
      };
      localStorage.setItem(`room:${data.room_code}:identity`, JSON.stringify(identity));
      rememberIdentityPrefs(identity.name, identity.venmoUsername);
      showJoinPrompt = false;
      connectWS();
    } catch (err) {
      joinError = err instanceof Error ? err.message : 'Join failed';
    }
  };

  onMount(() => {
    hydrateJoinPrefillFromCookies();
    migrateFromFriendGroups();
    const stored = localStorage.getItem(`room:${roomCode}:identity`);
    if (stored) {
      identity = JSON.parse(stored);
      connectWS();
    } else {
      showJoinPrompt = true;
      joinPrefillLocked = false;
      prefillJoinPromptFromCookies();
      connectWS();
    }
    if (browser) {
      shareLink = `${window.location.origin}/room/${roomCode}`;
      void (async () => {
        qrUrl = await buildQrDataUrl(shareLink, 240);
        qrUrlFullscreen = await buildQrDataUrl(shareLink, 960);
      })();
      const kickReconnect = () => {
        reconnectDelay = RECONNECT_MIN;
        scheduleReconnect();
      };
      const onVisibilityChange = () => {
        if (document.visibilityState !== 'visible') return;
        kickReconnect();
        if (summaryChargeQueueActive && summaryChargeQueuePendingResume) {
          summaryChargeQueuePendingResume = false;
          setTimeout(() => advanceSummaryChargeQueue(), 280);
        }
      };
      window.addEventListener('online', kickReconnect);
      window.addEventListener('visibilitychange', onVisibilityChange);
      return () => {
        window.removeEventListener('online', kickReconnect);
        window.removeEventListener('visibilitychange', onVisibilityChange);
        if (roomHistoryPersistTimer) clearTimeout(roomHistoryPersistTimer);
        roomHistoryPersistTimer = null;
        if (billCodeShareFeedbackTimer) clearTimeout(billCodeShareFeedbackTimer);
        billCodeShareFeedbackTimer = null;
        setDocumentModalLock(false);
      };
    }
    return () => {
      if (roomHistoryPersistTimer) clearTimeout(roomHistoryPersistTimer);
      roomHistoryPersistTimer = null;
      if (billCodeShareFeedbackTimer) clearTimeout(billCodeShareFeedbackTimer);
      billCodeShareFeedbackTimer = null;
      setDocumentModalLock(false);
    };
  });

  $: items = room ? sortItemsByOrder(Object.values(room.items).map((item) => normalizeItem(item))) : [];
  $: repeatedGroupsByKey = buildRepeatedGroups(items);
  $: itemEntries = (() => {
    const entries: ItemListEntry[] = [];
    const emittedGroups = new Set<string>();
    items.forEach((item) => {
      const key = repeatedGroupSignature(item);
      const group = key ? repeatedGroupsByKey[key] : null;
      if (group) {
        if (emittedGroups.has(key!)) return;
        emittedGroups.add(key!);
        entries.push({
          kind: 'group',
          sortOrder: group.sortOrder,
          group
        });
        return;
      }
      entries.push({
        kind: 'item',
        sortOrder: itemSortOrderOr(item, Number.MAX_SAFE_INTEGER),
        item
      });
    });
    return entries;
  })();
  $: standaloneItems = itemEntries
    .filter((entry): entry is Extract<ItemListEntry, { kind: 'item' }> => entry.kind === 'item')
    .map((entry) => entry.item);
  $: activeRepeatedGroup =
    repeatedGroupSheetKey && repeatedGroupsByKey[repeatedGroupSheetKey]
      ? repeatedGroupsByKey[repeatedGroupSheetKey]
      : null;
  $: participants = room ? (Object.values(room.participants) as Participant[]) : [];
  $: if (showAddPersonModal) {
    contacts = loadContacts();
    addPersonSelectedContactIds = new Set();
    addPersonSearchFilter = '';
  }
  $: filteredAddPersonContacts = addPersonSearchFilter.trim()
    ? contacts.filter((c) => c.name.toLowerCase().includes(addPersonSearchFilter.trim().toLowerCase()))
    : contacts;
  $: if (browser && room && identity.userId && identity.name.trim()) {
    const others = Object.entries(room.participants || {})
      .filter(([id]) => id !== identity.userId)
      .map(([, p]) => ({
        name: (p as Participant).name,
        venmoUsername: (p as Participant).venmoUsername
      }))
      .filter((p) => p.name.trim());
    if (others.length > 0) {
      trackRecentPeople(others);
    }
  }
  $: if (browser && room && identity.userId && identity.name.trim()) {
    scheduleRoomHistoryPersist();
  }
  $: roomParticipantsForJoin = [...participants].sort((a, b) => a.name.localeCompare(b.name));
  $: if (!showReceiptReview) {
    if (showReceiptItemEditModal) {
      closeReceiptItemEditor();
    } else if (showReceiptAttachModal) {
      closeReceiptAttachModal();
    }
  }
  $: if (showJoinPrompt && !joinPrefillLocked) {
    prefillJoinPromptFromCookies();
  }
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
  $: anyModalOpen =
    showAssign ||
    showRepeatedGroupSheet ||
    showReceiptReview ||
    showReceiptItemEditModal ||
    showReceiptItemAddonsModal ||
    showReceiptAttachModal ||
    showItemModal ||
    showItemAddonsModal ||
    showBillSettingsModal ||
    showSummary ||
    showNameModal ||
    showRoomNameModal ||
    showAddPersonModal ||
    showContactsModal ||
    showJoinPrompt ||
    showQrFullscreen;
  $: if (showRepeatedGroupSheet && !activeRepeatedGroup) {
    closeRepeatedGroupSheet();
  }
  $: if (browser) {
    setDocumentModalLock(anyModalOpen);
  }
  $: parsedBillDiscountCents = parseByCurrency(parsedBillDiscountInput, receiptCurrencySelection || roomCurrency);
  $: parsedBillChargesCents = parseByCurrency(parsedBillChargesInput, receiptCurrencySelection || roomCurrency);
  $: parsedTipCents = parseByCurrency(parsedTipInput, receiptCurrencySelection || roomCurrency);
  $: receiptSubtotalCents = subtotalFromEditable(editableItems);
  $: receiptGrossSubtotalCents = itemGrossSubtotalFromEditable(editableItems);
  $: parsedTaxCents = parseByCurrency(parsedTaxInput, receiptCurrencySelection || roomCurrency);
  $: receiptImportedTotalCents = Math.max(
    0,
    receiptSubtotalCents + parsedTaxCents + parsedTipCents + parsedBillChargesCents
  );
  $: receiptTaxPercent =
    receiptSubtotalCents > 0 ? (parsedTaxCents / receiptSubtotalCents) * 100 : 0;
  $: receiptTipPercent =
    receiptGrossSubtotalCents > 0 ? (parsedTipCents / receiptGrossSubtotalCents) * 100 : 0;

  const toCents = (val: string, code = roomCurrency) => {
    const factor = factorFor(code);
    return Math.round((Number.parseFloat(val || '0') || 0) * factor);
  };
  const preTaxGrossSubtotal = (list: Item[]) => {
    if (!list?.length) return 0;
    const subtotal = list.reduce((sum, it) => {
      const gross = Math.max(0, Number(it.line_price_cents || 0));
      return sum + gross;
    }, 0);
    return Number.isFinite(subtotal) ? Math.max(0, Math.round(subtotal)) : 0;
  };
  const preTaxNetSubtotal = (list: Item[]) => {
    if (!list?.length) return 0;
    const subtotal = list.reduce((sum, it) => {
      const qty = it.quantity || 1;
      const gross = Number(it.line_price_cents || 0);
      const discount = Number(it.discount_cents || 0) * qty;
      const net = Math.max(0, gross - discount);
      return sum + net;
    }, 0);
    const billDiscount = Math.max(0, room?.bill_discount_cents || 0);
    return Number.isFinite(subtotal) ? Math.max(0, Math.round(subtotal) - billDiscount) : 0;
  };
  $: preTaxGrossSubtotalCents = preTaxGrossSubtotal(items);
  $: preTaxNetSubtotalCents = preTaxNetSubtotal(items);
  $: (() => {
    taxCentsPreview = toCents(taxInput, roomCurrency);
    tipCentsPreview = toCents(tipInput, roomCurrency);
    billDiscountCentsPreview = toCents(billDiscountInput, roomCurrency);
    billChargesCentsPreview = toCents(billChargesInput, roomCurrency);
  })();
  $: taxPercent = preTaxNetSubtotalCents > 0 ? (taxCentsPreview / preTaxNetSubtotalCents) * 100 : 0;
  $: tipPercent = preTaxGrossSubtotalCents > 0 ? (tipCentsPreview / preTaxGrossSubtotalCents) * 100 : 0;
  $: billDiscountPercent =
    preTaxGrossSubtotalCents > 0 ? (billDiscountCentsPreview / preTaxGrossSubtotalCents) * 100 : 0;
  $: billChargesPercent =
    preTaxGrossSubtotalCents > 0 ? (billChargesCentsPreview / preTaxGrossSubtotalCents) * 100 : 0;
  $: billSettingsDirty =
    !numericInputToString(taxInput) ||
    !numericInputToString(tipInput) ||
    !numericInputToString(billDiscountInput) ||
    !numericInputToString(billChargesInput) ||
    taxCentsPreview !== Math.max(0, room?.tax_cents || 0) ||
    tipCentsPreview !== Math.max(0, room?.tip_cents || 0) ||
    billDiscountCentsPreview !== Math.max(0, room?.bill_discount_cents || 0) ||
    billChargesCentsPreview !== Math.max(0, room?.bill_charges_cents || 0);

  const syncBillSettingsInputsFromRoom = () => {
    const exp = exponentFor(roomCurrency);
    const factor = factorFor(roomCurrency);
    taxInput = ((room?.tax_cents || 0) / factor).toFixed(exp);
    tipInput = ((room?.tip_cents || 0) / factor).toFixed(exp);
    billDiscountInput = ((room?.bill_discount_cents || 0) / factor).toFixed(exp);
    billChargesInput = ((room?.bill_charges_cents || 0) / factor).toFixed(exp);
  };

  $: if (room && !billSettingsInitialized) {
    syncBillSettingsInputsFromRoom();
    billSettingsInitialized = true;
  }

  const setTipPercent = (pct: number) => {
    const base = preTaxGrossSubtotalCents || 0;
    const tip = Math.round((base * pct) / 100);
    const exp = exponentFor(roomCurrency);
    const factor = factorFor(roomCurrency);
    tipInput = (tip / factor).toFixed(exp);
    tipCentsPreview = tip;
    tipPercent = base > 0 ? (tip / base) * 100 : 0;
  };

  const saveBillSettings = () => {
    if (!room) return;
    const payload = {
      tax_cents: Math.max(0, taxCentsPreview),
      tip_cents: Math.max(0, tipCentsPreview),
      bill_discount_cents: Math.max(0, billDiscountCentsPreview),
      bill_charges_cents: Math.max(0, billChargesCentsPreview)
    };
    sendOp({ kind: 'set_tax_tip', actor_id: identity.userId, payload });
    applyLocalOp({ kind: 'set_tax_tip', payload });
    syncBillSettingsInputsFromRoom();
    showBillSettingsModal = false;
  };

  const clearBillSettingsInputs = () => {
    taxInput = '';
    tipInput = '';
    billDiscountInput = '';
    billChargesInput = '';
  };
</script>

<div class="app-screen relative pb-28">
  {#if wsStatus !== 'connected'}
    <div class="absolute top-3 inset-x-0 z-40 flex justify-center px-4 pointer-events-none">
      <div class="pointer-events-auto flex items-center gap-3 rounded-xl border border-amber-300/45 bg-amber-500/16 px-3 py-2 text-xs text-amber-100 shadow-lg">
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
  <header class="mx-auto w-full max-w-md px-4 pt-5 pb-3 space-y-3 motion-rise">
    <div class="glass-card accent-gradient room-hero-card relative p-4 sm:p-5 space-y-4">
      <button class="action-btn action-btn-surface action-btn-compact absolute left-3 top-3 z-10" on:click={() => goto('/')}>
        <svg class="inline-block align-middle" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round">
          <path d="M15 18l-6-6 6-6" />
        </svg>
        <span class="ml-1">Landing</span>
      </button>
      <div class="text-center">
        <p class="text-xs uppercase tracking-wide text-white/70">Restaurant</p>
        <h1 class="text-2xl font-semibold text-white">{room?.name || 'Shared Bill'}</h1>
        <div class="mt-2 flex items-center justify-center gap-3 text-sm text-white/80 flex-wrap">
          <button
            type="button"
            class="rounded-full border border-white/20 bg-black/20 px-3 py-1 font-mono text-white transition-colors hover:bg-black/30"
            on:click={shareRoomFromBillCode}
            title="Share or copy join link"
          >
            Bill Code: {roomCode?.toUpperCase()}
          </button>
          {#if billCodeShareFeedback}
            <span class="rounded-full border border-cyan-300/35 bg-cyan-500/12 px-2 py-0.5 text-[11px] font-semibold text-cyan-100">
              {billCodeShareFeedback}
            </span>
          {/if}
          {#if qrUrl}
            <button
              type="button"
              class="rounded-lg border border-white/20 bg-white/5 p-0.5 qr-launch"
              title="Expand QR code"
              aria-label="Expand QR code"
              on:click={() => (showQrFullscreen = true)}
            >
              <img src={qrUrl} alt="Join QR code" class="h-14 w-14 rounded-md" />
            </button>
          {/if}
        </div>
      </div>

      <div class="flex items-center justify-center gap-3">
        <Avatar
          initials={identity.initials}
          color={colorHex(identity.colorSeed)}
          size={56}
          badge={initialsBadges[identity.userId] ? String(initialsBadges[identity.userId]) : undefined}
          title={identity.name}
          finished={room?.participants?.[identity.userId]?.finished ?? false}
        />
        <div class="min-w-0">
          <p class="text-xs uppercase tracking-wide text-white/70">You</p>
          <p class="truncate text-sm font-semibold text-white">{identity.name || 'Guest'}</p>
          <button
            class="action-btn action-btn-surface mt-2"
            on:click={() => {
              nameInput = identity.name;
              venmoInput = room?.participants?.[identity.userId]?.venmoUsername || '';
              showNameModal = true;
            }}
          >
            Edit Profile
          </button>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-2">
        <button class="action-btn action-btn-surface w-full" on:click={() => { roomNameInput = room?.name || ''; showRoomNameModal = true; }}>
          Rename Bill
        </button>
        <button class="action-btn action-btn-primary w-full" on:click={() => { addPersonName = ''; addPersonVenmoInput = ''; addPersonSearchFilter = ''; addPersonSelectedContactIds = new Set(); contacts = loadContacts(); showAddPersonModal = true; }}>
          Add Person
        </button>
      </div>
    </div>

    <div class="glass-card room-sub-card ui-card-lift motion-rise motion-rise-delay-1 p-3 flex items-center justify-between gap-3">
      <span class="text-sm text-surface-200">Bill currency</span>
      <select
        class="input max-w-[11rem]"
        bind:value={roomCurrency}
        on:change={(e) => changeCurrency((e.target as HTMLSelectElement).value)}
      >
        {#each COMMON_CURRENCIES as c}
          <option value={c.code}>{c.flag} {c.code} {c.symbol}</option>
        {/each}
      </select>
    </div>

    <div class="glass-card room-sub-card ui-card-lift motion-rise motion-rise-delay-2 p-3">
      <div class="flex gap-3 overflow-x-auto py-1">
        {#if room}
          {#each participants as participant}
            <div class="relative w-14 shrink-0 text-center">
              <div class="mx-auto w-fit">
                <Avatar
                  initials={participant.initials}
                  color={colorHex(participant.colorSeed)}
                  size={36}
                  badge={initialsBadges[participant.id] ? String(initialsBadges[participant.id]) : undefined}
                  title={participant.name}
                  finished={participant.finished ?? false}
                />
              </div>
              <span
                class="absolute right-0 top-0 h-3 w-3 rounded-full border border-surface-900"
                style={`background:${participant.present ? '#22c55e' : '#64748b'};`}
                title={participant.present ? 'Present' : 'Not present'}
              ></span>
              {#if !participant.present && !participantAssignments[participant.id]}
                <button
                  class="action-btn action-btn-danger action-btn-compact mt-1 w-full"
                  title="Remove"
                  on:click={() => removeParticipant(participant.id)}
                >
                  Remove
                </button>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
    </div>
  </header>

  <main class="mx-auto w-full max-w-md px-4 space-y-4 pb-24">
    {#if warningBanner}
      <div class="rounded-xl bg-amber-500/15 text-amber-100 px-4 py-3 text-sm border border-amber-400/40 flex items-center justify-between gap-3 room-alert-card ui-panel">
        <div class="min-w-0">
          Receipt parsed with {receiptWarnings.length} note{receiptWarnings.length === 1 ? '' : 's'}—review recommended.
        </div>
        <button class="action-btn action-btn-surface shrink-0" type="button" on:click={() => (showReceiptReview = true)}>
          Review
        </button>
      </div>
    {/if}
    {#if receiptError}
      <div class="rounded-xl bg-error-500/20 text-error-200 px-4 py-3 text-sm border border-error-500/40 room-alert-card ui-panel">
        <div class="flex items-center justify-between gap-3">
          <div class="min-w-0">{receiptError}</div>
          {#if receiptLastUploadedFile && !receiptRetryConsumed}
            <button
              class="action-btn action-btn-surface action-btn-compact shrink-0"
              type="button"
              on:click={retryReceiptParse}
              disabled={receiptUploading}
            >
              {receiptUploading && receiptTryingAgain ? 'Trying again...' : 'Try Again'}
            </button>
          {/if}
        </div>
      </div>
    {/if}

    <section class="space-y-3 room-items-section motion-rise motion-rise-delay-1">
      <div class="flex items-center justify-between section-heading-row">
        <h2 class="text-lg font-semibold">Items</h2>
        {#if room && items.length === 0}
          <div class="flex items-center gap-2">
            <button
              class={`action-btn action-btn-surface receipt-upload-btn ${receiptUploading ? 'opacity-60 pointer-events-none' : ''}`}
              type="button"
              on:click={() => receiptFileInputEl?.click()}
            >
              <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round">
                <path d="M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2Z" />
                <path d="M9 7l1.5-2.5h3L15 7" />
                <circle cx="12" cy="13" r="3.5" />
              </svg>
              <span class="ml-1">
                {receiptUploading
                  ? receiptTryingAgain
                    ? 'Trying again...'
                    : 'Uploading...'
                  : 'Upload receipt'}
              </span>
            </button>
            <input bind:this={receiptFileInputEl} type="file" class="hidden" accept="image/*" on:change={submitReceipt} />
          </div>
        {/if}
      </div>
      {#if bulkAssignMode}
        <div class="rounded-xl border border-cyan-400/35 bg-cyan-500/10 px-3 py-2 ui-panel">
          <p class="text-sm text-cyan-100 font-semibold">
            Bulk assign mode
            <span class="text-cyan-200/90 font-normal">· {bulkSelectedItemIds.length} selected</span>
          </p>
        </div>
      {/if}
      {#if room}
        {#each itemEntries as entry (entry.kind === 'group' ? `group:${entry.group.key}` : `item:${entry.item.id}`)}
          {#if entry.kind === 'item'}
            {@const item = entry.item}
            <div
              class={`glass-card touch-card room-item-card rounded-2xl p-4 flex items-center justify-between w-full text-left ${
                bulkAssignMode && bulkAssignSelectedByItemId[item.id]
                  ? 'border-cyan-300/60 shadow-[0_0_0_1px_rgba(34,211,238,0.35)]'
                  : ''
              }`}
              role="button"
              tabindex="0"
              on:click={() =>
                bulkAssignMode ? toggleBulkItemSelection(item.id) : toggleAssign(item.id, identity.userId)}
              on:keydown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  if (bulkAssignMode) {
                    toggleBulkItemSelection(item.id);
                  } else {
                    toggleAssign(item.id, identity.userId);
                  }
                }
              }}
            >
              <div class="flex-1 pr-4 min-w-0">
                <p
                  class="font-semibold text-white leading-tight break-words truncate"
                  style="font-size: clamp(13px, 4vw, 16px);"
                  title={itemDisplayParts(item).baseName}
                >
                  {itemDisplayParts(item).baseName}
                </p>
                {#if bulkAssignMode}
                  <p class="mt-1 text-xs text-cyan-100/85">
                    {bulkAssignSelectedByItemId[item.id] ? 'Selected for bulk action' : 'Tap to select'}
                  </p>
                {/if}
                {#if itemDisplayParts(item).addons.length > 0}
                  <div class="mt-1 flex flex-wrap gap-1">
                    {#each itemDisplayParts(item).addons as addon}
                      <span class="text-[11px] rounded-full border border-white/15 bg-white/5 px-2 py-0.5 text-surface-300">
                        + {addon.name}
                        {#if addon.price_cents > 0}
                          · {formatAmount(addon.price_cents)}
                        {/if}
                      </span>
                    {/each}
                  </div>
                {/if}
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
                {#if bulkAssignMode}
                  <span class={`text-xs px-2 py-1 rounded-full border ${
                    bulkAssignSelectedByItemId[item.id]
                      ? 'border-cyan-300/55 bg-cyan-500/20 text-cyan-100'
                      : 'border-surface-700 bg-surface-900/70 text-surface-300'
                  }`}>
                    {bulkAssignSelectedByItemId[item.id] ? 'Selected' : 'Select'}
                  </span>
                {:else}
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
                {/if}
              </div>
            </div>
          {:else}
            {@const group = entry.group}
            <div
              class="glass-card touch-card room-item-card rounded-2xl p-4 flex items-center justify-between w-full text-left"
              role="button"
              tabindex="0"
              on:click={() => openRepeatedGroupSheet(group.key)}
              on:keydown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  openRepeatedGroupSheet(group.key);
                }
              }}
            >
              <div class="flex-1 pr-4 min-w-0">
                <p
                  class="font-semibold text-white leading-tight break-words truncate"
                  style="font-size: clamp(13px, 4vw, 16px);"
                  title={`${group.baseName} ×${group.items.length}`}
                >
                  {group.baseName} ×{group.items.length}
                </p>
                <p class="mt-1 text-xs text-cyan-100/85">
                  {bulkAssignMode ? 'Managed in servings. Not part of global bulk mode.' : 'Tap to manage servings'}
                </p>
                {#if group.addons.length > 0}
                  <div class="mt-1 flex flex-wrap gap-1">
                    {#each group.addons as addon}
                      <span class="text-[11px] rounded-full border border-white/15 bg-white/5 px-2 py-0.5 text-surface-300">
                        + {addon.name}
                        {#if addon.price_cents > 0}
                          · {formatAmount(addon.price_cents)}
                        {/if}
                      </span>
                    {/each}
                  </div>
                {/if}
                <p class="text-sm text-surface-200 whitespace-nowrap">{formatAmount(group.totalLinePriceCents)}</p>
                {#if group.totalDiscountCents}
                  <p class="text-xs text-surface-300">
                    Discount: {formatAmount(group.totalDiscountCents)} · Net: {formatAmount(group.totalNetPriceCents)}
                  </p>
                {/if}
                <div class="mt-2 flex flex-wrap gap-1.5">
                  {#each group.items as serving, index (serving.id)}
                    <span class="text-[11px] rounded-full border border-cyan-300/25 bg-cyan-500/10 px-2 py-1 text-cyan-100">
                      #{index + 1} · {servingAssignedCount(serving)} {servingAssignedCount(serving) === 1 ? 'person' : 'people'}
                    </span>
                  {/each}
                </div>
              </div>
              <div class="flex flex-col gap-2 mt-2 items-end flex-shrink-0 w-auto ml-2">
                <div class="flex gap-2 flex-wrap justify-end">
                  <button
                    class="action-btn action-btn-surface action-btn-compact"
                    title="Edit group"
                    type="button"
                    on:click|stopPropagation={() => openGroupEditModal(group)}
                  >
                    <svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#eab308">
                      <path d="M12 20h9" />
                      <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5Z" />
                    </svg>
                    <span class="hidden sm:inline ml-1">Edit</span>
                  </button>
                  <button
                    class="action-btn action-btn-danger action-btn-compact"
                    title="Delete group"
                    type="button"
                    on:click|stopPropagation={() => removeRepeatedGroup(group)}
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
          {/if}
        {/each}
      {/if}
    </section>

  </main>

  <div class="sticky-toolbar toolbar-polish">
    <div class="mx-auto w-full max-w-md space-y-2">
      {#if bulkAssignMode}
        <div class="space-y-2">
          <div class="flex items-center justify-between gap-2">
            <p class="text-sm text-cyan-100 font-semibold">
              Bulk · {bulkSelectedItemIds.length} selected
            </p>
            <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={() => (bulkAssignSelectedByItemId = {})}>
              Clear selected
            </button>
          </div>
          <label class="block space-y-1">
            <span class="text-xs uppercase tracking-wide text-cyan-100/80">Assign selected to</span>
            <select class="input w-full" bind:value={bulkAssignTargetParticipantId}>
              <option value="" disabled>Select person</option>
              {#each participants as participant}
                <option value={participant.id}>{participant.name}</option>
              {/each}
            </select>
          </label>
          <div class="grid grid-cols-2 gap-2">
            <button
              class="btn btn-outline w-full"
              type="button"
              on:click={applyBulkSplitEvenlyAcrossRoom}
              disabled={bulkSelectedItemIds.length === 0 || participants.length === 0}
            >
              Split evenly
            </button>
            <button
              class="btn btn-outline w-full"
              type="button"
              on:click={applyBulkAssignToParticipant}
              disabled={bulkSelectedItemIds.length === 0 || !bulkAssignTargetParticipantId}
            >
              Assign selected
            </button>
            <button
              class="btn btn-outline w-full"
              type="button"
              on:click={clearBulkAssignments}
              disabled={bulkSelectedItemIds.length === 0}
            >
              Clear assignments
            </button>
            <button
              class="btn w-full border-cyan-400/40 bg-cyan-500/20 text-cyan-100"
              type="button"
              on:click={() => setBulkAssignMode(false)}
            >
              Exit Bulk
            </button>
          </div>
        </div>
      {:else}
        {#if room?.participants?.[identity.userId]}
          <button
            class="btn w-full {room.participants[identity.userId].finished ? 'border-green-400/40 bg-green-500/20 text-green-100' : 'border-red-400/40 bg-red-500/20 text-red-100'}"
            on:click={toggleMyFinished}
          >
            {#if room.participants[identity.userId].finished}
              ✓ I'm Done
            {:else}
              Mark as Done
            {/if}
          </button>
        {/if}
        <div class="grid grid-cols-2 gap-2">
          <button class="btn btn-primary w-full" on:click={openNewItemModal}>Add Item</button>
          <button class="btn btn-outline w-full" on:click={() => { syncBillSettingsInputsFromRoom(); showBillSettingsModal = true; }}>Tax/Tip</button>
          <button class="btn btn-outline w-full" on:click={async () => { await buildSummary(); showSummary = true; }}>Summary</button>
          {#if room}
            <button
              class="btn btn-outline w-full"
              type="button"
              on:click={() => setBulkAssignMode(true)}
            >
              Bulk
            </button>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  {#if showRepeatedGroupSheet && activeRepeatedGroup}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[80vh] overflow-y-auto">
        <div class="flex items-start justify-between gap-3">
          <div>
            <h3 class="text-lg font-semibold modal-title">{activeRepeatedGroup.baseName} ×{activeRepeatedGroup.items.length}</h3>
            <p class="text-xs text-surface-300 modal-subtitle">
              Manage servings for repeated identical items. Assignment happens per serving.
            </p>
          </div>
          <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={closeRepeatedGroupSheet}>
            Done
          </button>
        </div>

        <div class="rounded-xl border border-white/10 bg-white/5 px-3 py-2 ui-panel">
          <div class="flex items-center justify-between gap-3 text-sm text-surface-200">
            <span>Group total</span>
            <span class="font-semibold text-white">{formatAmount(activeRepeatedGroup.totalLinePriceCents)}</span>
          </div>
          {#if activeRepeatedGroup.totalDiscountCents > 0}
            <div class="mt-1 flex items-center justify-between gap-3 text-xs text-surface-300">
              <span>Net after discount</span>
              <span>{formatAmount(activeRepeatedGroup.totalNetPriceCents)}</span>
            </div>
          {/if}
        </div>

        <div class={`grid gap-2 ${activeRepeatedGroup.items.length === participants.length ? 'grid-cols-4' : 'grid-cols-3'}`}>
          <button
            class="action-btn action-btn-surface action-btn-compact h-11 w-full px-0"
            type="button"
            title="Edit group"
            aria-label="Edit group"
            on:click={() => openGroupEditModal(activeRepeatedGroup!)}
          >
            <svg class="inline-block align-middle" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M12 20h9" />
              <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5Z" />
            </svg>
          </button>
          {#if activeRepeatedGroup.items.length === participants.length}
            <button
              class="action-btn action-btn-surface action-btn-compact h-11 w-full px-0"
              type="button"
              on:click={() => applyRepeatedGroupOneEach(activeRepeatedGroup!)}
              title="Assign one serving to each person"
              aria-label="One each"
            >
              <svg width="28" height="28" viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M12 20.5799L16 18V30" />
                <path d="M31 20.5799L35 18V30" />
                <path d="M24 20V21" />
                <path d="M24 27V28" />
              </svg>
            </button>
          {/if}
          <button
            class="action-btn action-btn-surface action-btn-compact h-11 w-full px-0"
            type="button"
            on:click={() => applyRepeatedGroupEveryone(activeRepeatedGroup!)}
            disabled={participants.length === 0}
            title="Assign everyone to every serving"
            aria-label="Everyone on every serving"
          >
            <svg width="28" height="28" viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M30 30V18L38 30V18" />
              <path d="M10 30V18L18 30V18" />
              <path d="M24 20V21" />
              <path d="M24 27V28" />
            </svg>
          </button>
          <button
            class="action-btn action-btn-danger action-btn-compact h-11 w-full px-0"
            type="button"
            on:click={() => clearRepeatedGroupAssignments(activeRepeatedGroup!)}
            title="Clear all serving assignments"
            aria-label="Clear all serving assignments"
          >
            <svg width="26" height="26" viewBox="-4 0 32 32" fill="currentColor" aria-hidden="true">
              <path d="M13.688 9.219v-3.969c0-0.719-0.531-1.25-1.219-1.25h-0.938c-0.688 0-1.219 0.531-1.219 1.25v3.969c0 0.688 0.531 1.25 1.219 1.25h0.938c0.688 0 1.219-0.563 1.219-1.25zM8.406 9.969l-2.813-2.781c-0.469-0.469-1.281-0.469-1.75 0l-0.656 0.656c-0.469 0.469-0.469 1.281 0 1.75l2.781 2.813c0.469 0.469 1.313 0.469 1.781 0l0.656-0.656c0.469-0.469 0.469-1.313 0-1.781zM18.031 12.406l2.781-2.813c0.469-0.469 0.469-1.281 0-1.75l-0.656-0.656c-0.469-0.469-1.281-0.469-1.75 0l-2.813 2.781c-0.469 0.469-0.469 1.313 0 1.781l0.656 0.656c0.469 0.469 1.313 0.469 1.781 0zM1.25 17.688h3.969c0.688 0 1.25-0.531 1.25-1.219v-0.969c0-0.688-0.563-1.188-1.25-1.188h-3.969c-0.719 0-1.25 0.5-1.25 1.188v0.969c0 0.688 0.531 1.219 1.25 1.219zM18.781 17.688h3.969c0.719 0 1.25-0.531 1.25-1.219v-0.969c0-0.688-0.531-1.188-1.25-1.188h-3.969c-0.688 0-1.25 0.5-1.25 1.188v0.969c0 0.688 0.563 1.219 1.25 1.219zM5.594 24.781l2.813-2.781c0.469-0.469 0.469-1.281 0-1.75l-0.656-0.688c-0.469-0.469-1.313-0.469-1.781 0l-2.781 2.844c-0.469 0.469-0.469 1.281 0 1.75l0.656 0.625c0.469 0.469 1.281 0.469 1.75 0zM20.813 22.406l-2.781-2.844c-0.469-0.469-1.313-0.469-1.781 0l-0.656 0.688c-0.469 0.469-0.469 1.281 0 1.75l2.813 2.781c0.469 0.469 1.281 0.469 1.75 0l0.656-0.625c0.469-0.469 0.469-1.281 0-1.75zM13.688 26.75v-3.969c0-0.688-0.531-1.25-1.219-1.25h-0.938c-0.688 0-1.219 0.563-1.219 1.25v3.969c0 0.719 0.531 1.25 1.219 1.25h0.938c0.688 0 1.219-0.531 1.219-1.25z" />
            </svg>
          </button>
        </div>

        <div class="space-y-3">
          {#each activeRepeatedGroup.items as serving, index (serving.id)}
            <div
              class="rounded-xl border border-surface-700/80 bg-surface-900/55 p-3 modal-list-row"
              role="button"
              tabindex="0"
              on:click={() => toggleAssign(serving.id, identity.userId)}
              on:keydown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  toggleAssign(serving.id, identity.userId);
                }
              }}
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="font-semibold text-white">Serving #{index + 1}</p>
                  <p class="text-sm text-surface-200">{formatAmount(serving.line_price_cents)}</p>
                  {#if serving.discount_cents}
                    <p class="text-xs text-surface-300">
                      Discount: {formatAmount(serving.discount_cents)} · Net: {formatAmount(serving.line_price_cents - serving.discount_cents)}
                    </p>
                  {/if}
                </div>
                <span class="text-xs rounded-full border border-cyan-300/25 bg-cyan-500/10 px-2 py-1 text-cyan-100">
                  {servingAssignedCount(serving)} assigned
                </span>
              </div>

              <div class="mt-2 flex items-center gap-1 flex-wrap">
                {#if Object.values(serving.assigned || {}).some(Boolean)}
                  {#each Object.entries(serving.assigned || {}) as [uid, on]}
                    {#if on}
                      <Avatar
                        initials={room?.participants?.[uid]?.initials || initialsFromName(room?.participants?.[uid]?.name || uid)}
                        color={colorHex(room?.participants?.[uid]?.colorSeed)}
                        badge={initialsBadges[uid] ? String(initialsBadges[uid]) : undefined}
                        title={room?.participants?.[uid]?.name}
                        size={22}
                      />
                    {/if}
                  {/each}
                {:else}
                  <span class="text-xs text-surface-400">No one assigned yet.</span>
                {/if}
              </div>

              <div class="mt-3 flex gap-2 flex-wrap justify-end">
                <button
                  class={`action-btn ${serving.assigned?.[identity.userId] ? 'action-btn-danger' : 'action-btn-primary'} action-btn-compact`}
                  type="button"
                  title={serving.assigned?.[identity.userId] ? 'Remove me from this serving' : 'Assign me to this serving'}
                  on:click|stopPropagation={() => toggleAssign(serving.id, identity.userId)}
                >
                  {#if serving.assigned?.[identity.userId]}
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
                  <span class="hidden sm:inline ml-1">{serving.assigned?.[identity.userId] ? 'Unassign' : 'Me'}</span>
                </button>
                <button
                  class="action-btn action-btn-surface action-btn-compact"
                  type="button"
                  title="Assign participants"
                  on:click|stopPropagation={() => {
                    activeItemId = serving.id;
                    showAssign = true;
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
                  class="action-btn action-btn-surface action-btn-compact"
                  type="button"
                  title="Pull out as item"
                  on:click|stopPropagation={() => pullServingOutAsItem(activeRepeatedGroup!, serving)}
                >
                  <svg class="inline-block align-middle" width="14" height="14" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                    <g transform="translate(-60 -959)" fill="currentColor" fill-rule="evenodd">
                      <g transform="translate(56 160)">
                        <path d="M15.732,809.137 L21.547,803.322 C21.859,803.01 22.393,803.222 22.406,803.663 L22.444,805.029 C22.46,805.581 22.92,806 23.472,806 L23.25,806 C23.802,806 24,805.524 24,804.972 L24,801 C24,799.9 23.1,799 22,799 L18.483,799 C17.93,799 17,799.425 17,799.977 L17,799.98 C17,800.532 17.647,800.994 18.199,801.009 L19.733,801.05 C20.174,801.061 20.387,801.595 20.076,801.907 L14.289,807.723 C13.899,808.113 13.913,808.746 14.304,809.137 C14.694,809.527 15.341,809.528 15.732,809.137 M24,812.011 L24,817.015 C24,818.117 23.55,819 22.448,819 L6.44,819 C5.338,819 4,818.117 4,817.015 L4,801.007 C4,799.904 5.338,799 6.44,799 L11.444,799 C11.996,799 12.444,799.448 12.444,800 C12.444,800.553 11.996,801 11.444,801 L7.444,801 C6.892,801 6,801.458 6,802.011 L6,815.015 C6,816.117 7.338,817 8.44,817 L21.444,817 C21.996,817 22,816.563 22,816.011 L22,812.011 C22,811.458 22.447,811.011 23,811.011 C23.552,811.011 24,811.458 24,812.011" />
                      </g>
                    </g>
                  </svg>
                  <span class="hidden sm:inline ml-1">Pull out</span>
                </button>
                <button
                  class="action-btn action-btn-danger action-btn-compact"
                  type="button"
                  title="Delete serving"
                  on:click|stopPropagation={() => removeItem(serving.id)}
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
          {/each}
        </div>
      </div>
    </div>
  {/if}

  {#if showAssign && activeItemId && room}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold modal-title">Assign participants</h3>
        <p class="text-xs text-surface-300 modal-subtitle">Tap a person to toggle assignment for this item.</p>
        <div class="space-y-2">
          {#each participants as participant}
            <button
              class="w-full flex items-center justify-between rounded-xl border border-surface-800 px-4 py-3 modal-list-row"
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
                  <span class="text-xs px-2 py-1 rounded-full bg-cyan-500/20 text-cyan-100 border border-cyan-300/40 flex items-center gap-1">
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

  <ReceiptCropModal
    open={showReceiptCropModal}
    file={receiptCropSourceFile}
    busy={receiptUploading}
    on:cancel={closeReceiptCropModal}
    on:confirm={confirmReceiptCrop}
  />

  {#if showReceiptReview && receiptResult}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[80vh]">
        <h3 class="text-lg font-semibold modal-title">Review receipt</h3>
        <p class="text-sm text-surface-200 modal-subtitle">Edit aggregated lines before importing. Prices are gross (pre-discount).</p>
        <div class="flex items-center justify-between gap-3 flex-wrap rounded-xl border border-white/10 bg-white/5 px-3 py-2 ui-panel">
          <div class="text-sm text-white/80">Receipt Currency</div>
          <select
            class="input bg-white/5 border border-white/15 rounded-lg px-3 py-2 text-white text-sm"
            bind:value={receiptCurrencySelection}
            on:change={(e) => {
              const next = ((e.target as HTMLSelectElement).value || '').toUpperCase();
              receiptCurrencySelection = next || receiptCurrencySelection;
              parsedTaxInput = normalizeInputByCurrency(parsedTaxInput, receiptCurrencySelection);
              parsedTipInput = normalizeInputByCurrency(parsedTipInput, receiptCurrencySelection);
              parsedBillDiscountInput = normalizeInputByCurrency(parsedBillDiscountInput, receiptCurrencySelection);
              parsedBillChargesInput = normalizeInputByCurrency(parsedBillChargesInput, receiptCurrencySelection);
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
        <div class="rounded-xl border border-cyan-400/35 bg-cyan-500/10 p-3 ui-panel">
          <div class="flex items-center justify-between gap-3 flex-wrap">
            <div>
              <p class="text-sm font-semibold text-cyan-100">Need a second pass?</p>
              <p class="text-xs text-surface-300">
                If any lines look off, run a deeper retry before importing.
              </p>
            </div>
            {#if !receiptRetryConsumed}
              <button
                class="btn btn-outline shrink-0"
                type="button"
                on:click={retryReceiptParse}
                disabled={!receiptLastUploadedFile || receiptUploading}
              >
                {receiptUploading && receiptTryingAgain ? 'Trying again...' : 'Try Again'}
              </button>
            {:else if receiptOriginalSnapshot && receiptRetrySnapshot}
              <button
                class="btn btn-outline shrink-0"
                type="button"
                on:click={swapReceiptRetrySource}
                disabled={receiptUploading}
              >
                {receiptUsingRetryResult ? 'Use original result' : 'Use try-again result'}
              </button>
            {/if}
          </div>
          {#if receiptRetryStatus}
            <p class="mt-2 text-xs text-surface-300">{receiptRetryStatus}</p>
          {/if}
        </div>
        {#if receiptWarnings.length > 0}
          <div class="rounded-xl border border-amber-400/40 bg-amber-500/10 text-amber-100 p-3 text-sm space-y-1 ui-panel">
            <div class="font-semibold text-amber-200">Notes from parser</div>
            <ul class="list-disc list-inside space-y-1 text-amber-100/80">
              {#each receiptWarnings as w}
                <li>{w}</li>
              {/each}
            </ul>
          </div>
        {/if}
        {#if receiptFlaggedIndices.length > 0}
          <div class="rounded-xl border border-amber-400/40 bg-amber-500/10 p-3 space-y-2 ui-panel">
            <div class="flex items-center justify-between gap-2 flex-wrap">
              <p class="text-sm text-amber-100">
                {receiptFlaggedIndices.length} item{receiptFlaggedIndices.length === 1 ? '' : 's'} flagged for review
              </p>
              <div class="flex items-center gap-2">
                <button
                  class={`action-btn action-btn-compact ${showReceiptFlaggedOnly ? 'action-btn-primary' : 'action-btn-surface'}`}
                  type="button"
                  on:click={() => (showReceiptFlaggedOnly = !showReceiptFlaggedOnly)}
                >
                  {showReceiptFlaggedOnly ? 'Show all' : 'Flagged only'}
                </button>
                <button
                  class="action-btn action-btn-surface action-btn-compact"
                  type="button"
                  on:click={focusNextFlaggedReceiptItem}
                  disabled={receiptFlaggedIndices.length === 0}
                >
                  Next
                </button>
              </div>
            </div>
          </div>
        {/if}
        {#if receiptVisibleIndices.length === 0}
          <div class="rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 text-sm text-surface-300 ui-panel">
            No items to show.
          </div>
        {/if}
        {#each receiptVisibleIndices as index (index)}
          <div
            id={`receipt-review-item-${index}`}
            class={`glass-card room-item-card rounded-2xl p-4 ui-panel ${
              editableItemNeedsReview(index) ? 'border-amber-400/40 bg-amber-500/8' : 'border-surface-800'
            }`}
          >
            <div class="flex items-start justify-between gap-3">
              <div class="flex-1 min-w-0 pr-2">
                <div class="flex items-center gap-2 flex-wrap">
                  <p class="text-xs uppercase tracking-wide text-surface-300">Item {index + 1}</p>
                  {#if editableItemNeedsReview(index)}
                    <span class="text-[11px] rounded-full border border-amber-400/40 bg-amber-500/15 px-2 py-0.5 text-amber-200">
                      Needs review
                    </span>
                  {/if}
                </div>
                <p
                  class="mt-1 font-semibold text-white leading-tight break-words truncate"
                  style="font-size: clamp(13px, 4vw, 16px);"
                  title={editableItemDisplayParts(editableItems[index]).baseName}
                >
                  {editableItemDisplayParts(editableItems[index]).baseName}
                </p>
                {#if editableItemDisplayParts(editableItems[index]).addons.length > 0}
                  <div class="mt-1 flex flex-wrap gap-1">
                    {#each editableItemDisplayParts(editableItems[index]).addons as addon}
                      <span class="text-[11px] rounded-full border border-white/15 bg-white/5 px-2 py-0.5 text-surface-300">
                        + {addon.name}
                        {#if addon.price_cents > 0}
                          · {formatAmount(addon.price_cents, receiptCurrencySelection)}
                        {/if}
                      </span>
                    {/each}
                  </div>
                {/if}
                <p class="mt-1 text-sm text-surface-200 whitespace-nowrap">
                  {formatAmount(pricingPreviewFromEditable(editableItems[index]).grossTotal, receiptCurrencySelection)}
                </p>
                <p class="text-xs text-surface-300">
                  {#if pricingPreviewFromEditable(editableItems[index]).unit > 0}
                    {pricingPreviewFromEditable(editableItems[index]).qty} × {formatAmount(pricingPreviewFromEditable(editableItems[index]).unit, receiptCurrencySelection)}
                  {:else}
                    Qty {pricingPreviewFromEditable(editableItems[index]).qty}
                  {/if}
                  {#if pricingPreviewFromEditable(editableItems[index]).addonPerItem > 0}
                    · Add-ons {formatAmount(pricingPreviewFromEditable(editableItems[index]).addonPerItem, receiptCurrencySelection)} each
                  {/if}
                </p>
                {#if discountedUnitAndNetFromEditable(editableItems[index]).netTotal < pricingPreviewFromEditable(editableItems[index]).grossTotal}
                  <p class="text-xs text-surface-300">
                    Net after discount: {formatAmount(discountedUnitAndNetFromEditable(editableItems[index]).netTotal, receiptCurrencySelection)}
                  </p>
                {/if}
                {#if editableItemReviewAt(index).reasons.length > 0}
                  <p class="mt-1 text-xs text-amber-200">
                    {editableItemReviewAt(index).reasons.join(' • ')}
                  </p>
                {/if}
              </div>
              <div class="flex flex-col gap-2 items-end flex-shrink-0">
                <button
                  class={`action-btn action-btn-compact ${
                    receiptEditingIndex === index ? 'action-btn-primary' : 'action-btn-surface'
                  }`}
                  type="button"
                  on:click={() => {
                    if (receiptEditingIndex === index) {
                      closeReceiptItemEditor();
                    } else {
                      openReceiptItemEditor(index);
                    }
                  }}
                >
                  {receiptEditingIndex === index ? 'Done' : 'Edit'}
                </button>
              </div>
            </div>
            {#if receiptEditingIndex === index && editableItems[index]}
              <div class="mt-4 pt-4 border-t border-white/10 space-y-3">
                <ItemEditorFields
                  name={editableItems[index].name}
                  nameInput={(value) => setEditableItemField(index, 'name', value)}
                  namePlaceholder="Item name"
                  showQuantity={true}
                  allowTotalToggle={true}
                  quantity={editableItems[index].quantity}
                  unitPrice={editableItems[index].unitPrice}
                  linePrice={editableItems[index].linePrice}
                  discountCents={editableItems[index].discountCents}
                  discountPercent={editableItems[index].discountPercent}
                  discountMode={editableItemDiscountMode(editableItems[index])}
                  totalInputMode={editableItemTotalInputMode(editableItems[index])}
                  currencyLabel={symbolFor(receiptCurrencySelection) || receiptCurrencySelection}
                  priceStep={1 / factorFor(receiptCurrencySelection)}
                  addonCount={(editableItems[index].addons || []).length}
                  addonTotalPerItemLabel={formatAmount(editableItemAddonTotalCents(index), receiptCurrencySelection)}
                  pricingTotalEquation={`${pricingPreviewFromEditable(editableItems[index]).qty} × ${formatAmount(pricingPreviewFromEditable(editableItems[index]).unit, receiptCurrencySelection)} = ${formatAmount(pricingPreviewFromEditable(editableItems[index]).baseTotal, receiptCurrencySelection)}`}
                  pricingAddonEquation={`${formatAmount(pricingPreviewFromEditable(editableItems[index]).addonPerItem, receiptCurrencySelection)} × ${pricingPreviewFromEditable(editableItems[index]).qty} = ${formatAmount(pricingPreviewFromEditable(editableItems[index]).addonTotal, receiptCurrencySelection)}`}
                  showAddonEquation={pricingPreviewFromEditable(editableItems[index]).addonTotal > 0}
                  netUnitLabel={formatAmount(discountedUnitAndNetFromEditable(editableItems[index]).netUnit, receiptCurrencySelection)}
                  netTotalLabel={formatAmount(discountedUnitAndNetFromEditable(editableItems[index]).netTotal, receiptCurrencySelection)}
                  highlightName={editableItemReviewAt(index).name}
                  highlightQuantity={editableItemReviewAt(index).quantity}
                  highlightUnitPrice={editableItemReviewAt(index).unitPrice}
                  highlightTotal={editableItemReviewAt(index).linePrice}
                  highlightDiscount={editableItemReviewAt(index).discount}
                  highlightAddons={editableItemReviewAt(index).addons}
                  totalModeToggle={() => toggleEditableItemTotalInputMode(index)}
                  quantityInput={(value) => {
                    setEditableItemField(index, 'quantity', value);
                    queueMicrotask(() => recalcDerived(index, 'quantity'));
                  }}
                  unitPriceInput={(value) => {
                    setEditableItemField(index, 'unitPrice', value);
                    queueMicrotask(() => recalcDerived(index, 'unitPrice'));
                  }}
                  linePriceInput={(value) => {
                    setEditableItemField(index, 'linePrice', value);
                    queueMicrotask(() => recalcDerived(index, 'linePrice'));
                  }}
                  discountModeSelect={(mode) => setEditableItemDiscountMode(index, mode)}
                  discountValueInput={(value) => {
                    if (editableItemDiscountMode(editableItems[index]) === 'percent') {
                      setEditableItemField(index, 'discountPercent', value);
                    } else {
                      setEditableItemField(index, 'discountCents', value);
                    }
                    queueMicrotask(() => recalcDerived(index, 'discount'));
                  }}
                  manageAddons={() => openReceiptItemAddonsModal(index)}
                />
                <div class="flex justify-end gap-2 flex-wrap">
                  <button
                    class="action-btn action-btn-surface action-btn-compact"
                    on:click={() => openReceiptAttachModal(index)}
                    type="button"
                    disabled={editableItems.length < 2}
                    title="Attach this item under another item as an add-on"
                  >
                    {showReceiptAttachModal && receiptAttachSourceIndex === index ? 'Close attach' : 'Attach as add-on'}
                  </button>
                  <button
                    class="action-btn action-btn-danger action-btn-compact"
                    on:click={() => removeEditableItem(index, { rebalanceTaxAndTip: true })}
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
                {#if showReceiptAttachModal && receiptAttachSourceIndex === index}
                  <div class="rounded-xl border border-cyan-400/40 bg-cyan-500/10 p-3 space-y-2 ui-panel">
                    <p class="text-xs text-cyan-100">
                      Choose the parent item for "{editableItems[index].name || `Item ${index + 1}`}".
                    </p>
                    <div class="space-y-2 max-h-52 overflow-y-auto pr-1">
                      {#each editableItems as item, targetIndex}
                        {#if targetIndex !== index}
                          <button
                            class="w-full text-left rounded-xl border border-surface-700/80 bg-surface-900/55 px-3 py-2 modal-list-row"
                            type="button"
                            on:click={() => attachEditableItemAsAddon(index, targetIndex)}
                          >
                            <div class="flex items-center justify-between gap-3">
                              <div class="min-w-0">
                                <div class="font-medium text-white truncate">{item.name || `Item ${targetIndex + 1}`}</div>
                                <div class="text-xs text-surface-300">
                                  Current total: {formatAmount(pricingPreviewFromEditable(item).grossTotal, receiptCurrencySelection)}
                                </div>
                              </div>
                              <span class="text-xs rounded-lg border border-cyan-400/45 bg-cyan-500/15 px-2 py-1 text-cyan-100">
                                Attach
                              </span>
                            </div>
                          </button>
                        {/if}
                      {/each}
                    </div>
                    <button class="btn btn-outline w-full" type="button" on:click={closeReceiptAttachModal}>
                      Cancel
                    </button>
                  </div>
                {/if}
                {#if showReceiptItemAddonsModal && receiptItemAddonsIndex === index}
                  <div class="rounded-xl border border-surface-700/80 bg-surface-900/55 p-3 space-y-3 ui-panel">
                    <p class="text-xs text-surface-300">
                      Add-on costs are per item and rolled into this item total.
                    </p>
                    <div class="space-y-2 max-h-56 overflow-y-auto pr-1">
                      {#if (editableItems[index].addons || []).length === 0}
                        <div class="text-xs text-surface-400">
                          No add-ons yet. Add one below if needed.
                        </div>
                      {/if}
                      {#each editableItems[index].addons || [] as addon, addonIdx}
                        <div class="rounded-xl border border-surface-700/80 bg-surface-900/55 p-2 modal-list-row space-y-2">
                          <div class="grid grid-cols-[1fr_7rem] gap-2 items-center">
                            <input
                              class="input w-full"
                              value={addon.name}
                              placeholder="Add-on"
                              on:input={(event) => updateEditableItemAddon(index, addonIdx, 'name', (event.target as HTMLInputElement).value)}
                            />
                            <input
                              class="input w-full text-right"
                              value={addon.price}
                              placeholder={zeroPlaceholderByCurrency(receiptCurrencySelection)}
                              inputmode="decimal"
                              type="number"
                              min="0"
                              step={1 / factorFor(receiptCurrencySelection)}
                              on:input={(event) => updateEditableItemAddon(index, addonIdx, 'price', (event.target as HTMLInputElement).value)}
                            />
                          </div>
                          <div class="flex items-center justify-end gap-2 flex-wrap">
                            <button
                              class="action-btn action-btn-surface action-btn-compact"
                              type="button"
                              title="Create a standalone item from this add-on"
                              on:click={() => promoteEditableItemAddonToItem(index, addonIdx)}
                            >
                              Create item
                            </button>
                            <button
                              class="action-btn action-btn-danger action-btn-compact"
                              type="button"
                              title="Remove add-on"
                              on:click={() => removeEditableItemAddon(index, addonIdx)}
                            >
                              Remove
                            </button>
                          </div>
                        </div>
                      {/each}
                    </div>
                    <button class="btn btn-outline w-full" type="button" on:click={() => addEditableItemAddon(index)}>
                      Add Add-on
                    </button>
                    <div class="rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-2 text-sm flex items-center justify-between ui-inline-metric">
                      <span class="text-surface-300">Add-on total per item</span>
                      <span class="font-semibold text-white">
                        {formatAmount(editableItemAddonTotalCents(index), receiptCurrencySelection)}
                      </span>
                    </div>
                    <div class="rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-2 text-sm flex items-center justify-between ui-inline-metric">
                      <span class="text-surface-300">
                        Add-on total ({Math.max(1, Number.parseInt(editableItems[index].quantity || '1', 10) || 1)} item{Math.max(1, Number.parseInt(editableItems[index].quantity || '1', 10) || 1) === 1 ? '' : 's'})
                      </span>
                      <span class="font-semibold text-white">
                        {formatAmount(editableItemAddonExtendedTotalCents(index), receiptCurrencySelection)}
                      </span>
                    </div>
                    <button
                      class="btn btn-outline w-full"
                      type="button"
                      on:click={() => {
                        showReceiptItemAddonsModal = false;
                        receiptItemAddonsIndex = null;
                      }}
                    >
                      Close add-ons
                    </button>
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
        <div class="border border-surface-800 rounded-xl p-4 space-y-3 ui-panel">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">Receipt-level discount</p>
              <p class="text-xs text-surface-300">Stored as a bill-wide discount and split by item cost.</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-surface-300">
                {symbolFor(receiptCurrencySelection) || receiptCurrencySelection}
              </span>
              <input
                class="input w-28 text-right"
                type="number"
                min="0"
                step={1 / factorFor(receiptCurrencySelection)}
                inputmode="decimal"
                bind:value={parsedBillDiscountInput}
                placeholder={zeroPlaceholderByCurrency(receiptCurrencySelection)}
                on:blur={() => {
                  parsedBillDiscountInput = normalizeInputByCurrency(
                    parsedBillDiscountInput,
                    receiptCurrencySelection
                  );
                }}
              />
            </div>
          </div>
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">Receipt-level non-tip charges</p>
              <p class="text-xs text-surface-300">Service/admin/convenience fees, split by item cost, excluded from tip base.</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-surface-300">
                {symbolFor(receiptCurrencySelection) || receiptCurrencySelection}
              </span>
              <input
                class="input w-28 text-right"
                type="number"
                min="0"
                step={1 / factorFor(receiptCurrencySelection)}
                inputmode="decimal"
                bind:value={parsedBillChargesInput}
                placeholder={zeroPlaceholderByCurrency(receiptCurrencySelection)}
                on:blur={() => {
                  parsedBillChargesInput = normalizeInputByCurrency(
                    parsedBillChargesInput,
                    receiptCurrencySelection
                  );
                }}
              />
            </div>
          </div>
          <div class="text-xs text-surface-300">
            Effective receipt subtotal after discounts: {formatAmount(receiptSubtotalCents, receiptCurrencySelection)}
          </div>
          <div class="text-xs text-surface-300">
            Pre-discount items subtotal (tip base): {formatAmount(receiptGrossSubtotalCents, receiptCurrencySelection)}
          </div>
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">Tax</p>
              <p class="text-xs text-surface-300">
                Tax % from effective subtotal:
                {receiptSubtotalCents > 0 ? ` ${receiptTaxPercent.toFixed(2)}%` : ' --%'}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-surface-300">
                {symbolFor(receiptCurrencySelection) || receiptCurrencySelection}
              </span>
              <input
                class="input w-28 text-right"
                type="number"
                min="0"
                step={1 / factorFor(receiptCurrencySelection)}
                inputmode="decimal"
                bind:value={parsedTaxInput}
                placeholder={zeroPlaceholderByCurrency(receiptCurrencySelection)}
                on:blur={() => {
                  parsedTaxInput = normalizeInputByCurrency(parsedTaxInput, receiptCurrencySelection);
                }}
              />
            </div>
          </div>
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">Tip / gratuity</p>
              <p class="text-xs text-surface-300">
                Tip % from pre-discount items subtotal:
                {receiptGrossSubtotalCents > 0 ? ` ${receiptTipPercent.toFixed(2)}%` : ' --%'}
              </p>
              <p class="text-xs text-surface-400">
                Only include tip/gratuity already charged on the bill, not suggested tip options.
              </p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-surface-300">
                {symbolFor(receiptCurrencySelection) || receiptCurrencySelection}
              </span>
              <input
                class="input w-28 text-right"
                type="number"
                min="0"
                step={1 / factorFor(receiptCurrencySelection)}
                inputmode="decimal"
                bind:value={parsedTipInput}
                placeholder={zeroPlaceholderByCurrency(receiptCurrencySelection)}
                on:blur={() => {
                  parsedTipInput = normalizeInputByCurrency(parsedTipInput, receiptCurrencySelection);
                }}
              />
            </div>
          </div>
          <div class="mt-3 flex items-start justify-between gap-3">
            <div>
              <p class="text-sm font-semibold">Subtotal</p>
              <p class="text-xs text-surface-300">After discounts, before tax and tip.</p>
            </div>
            <div class="text-right text-sm">
              <div class="font-semibold">
                {formatAmount(receiptSubtotalCents, receiptCurrencySelection)}
              </div>
            </div>
          </div>
          <div class="mt-4 rounded-xl border border-cyan-400/35 bg-cyan-500/10 p-3 ui-panel">
            <div class="flex items-start justify-between gap-3">
              <div>
                <p class="text-sm font-semibold">Receipt total</p>
              </div>
              <div class="text-right">
                <div class="text-lg font-semibold text-cyan-100">
                  {formatAmount(receiptImportedTotalCents, receiptCurrencySelection)}
                </div>
              </div>
            </div>
          </div>
        </div>
        <button class="btn btn-primary w-full" on:click={confirmReceipt}>Import Items & Bill Totals</button>
        <button
          class="btn btn-outline w-full"
          on:click={() => {
            showReceiptReview = false;
            showReceiptItemAddonsModal = false;
            receiptItemAddonsIndex = null;
            showReceiptAttachModal = false;
            receiptAttachSourceIndex = null;
            receiptWarnings = [];
            warningBanner = false;
            editableItemReviewFlags = [];
            setReceiptEditingIndex(null);
            showReceiptFlaggedOnly = false;
            receiptReviewFocusIndex = 0;
            parsedTaxInput = '';
            parsedTipInput = '';
            parsedBillDiscountInput = '';
            parsedBillChargesInput = '';
            receiptCurrencySelection = (roomCurrency || DEFAULT_CURRENCY).toUpperCase();
            clearReceiptRetryState();
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  {/if}

  {#if showItemModal}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold modal-title">
          {itemModalMode === 'group' ? 'Edit repeated items' : itemModalMode === 'edit' ? 'Edit item' : 'Add item'}
        </h3>
        <p class="text-xs text-surface-300 modal-subtitle">
          {itemModalMode === 'group'
            ? 'Update shared details for every serving in this repeated-item group.'
            : 'Set pricing, discount, and add-ons in one place.'}
        </p>
        <ItemEditorFields
          name={itemForm.name}
          nameInput={(value) => {
            itemForm = { ...itemForm, name: value };
          }}
          namePlaceholder="Item name"
          showQuantity={itemModalMode !== 'edit'}
          allowTotalToggle={itemModalMode !== 'edit'}
          quantity={itemForm.quantity}
          unitPrice={itemForm.unitPrice}
          linePrice={itemForm.linePrice}
          discountCents={itemForm.discountCents}
          discountPercent={itemForm.discountPercent}
          discountMode={itemFormDiscountMode}
          totalInputMode={itemFormTotalInputMode}
          currencyLabel={symbolFor(roomCurrency) || roomCurrency}
          priceStep={1 / factorFor(roomCurrency)}
          addonCount={(itemForm.addons || []).length}
          addonTotalPerItemLabel={formatAmount(itemFormAddonTotalPreviewCents, roomCurrency)}
          pricingTotalEquation={`${itemFormPricingPreview.qty} × ${formatAmount(itemFormPricingPreview.unit, roomCurrency)} = ${formatAmount(itemFormPricingPreview.baseTotal, roomCurrency)}`}
          pricingAddonEquation={`${formatAmount(itemFormPricingPreview.addonPerItem, roomCurrency)} × ${itemFormPricingPreview.qty} = ${formatAmount(itemFormPricingPreview.addonTotal, roomCurrency)}`}
          showAddonEquation={itemFormPricingPreview.addonTotal > 0}
          netUnitLabel={formatAmount(itemFormNetPreview.netUnit)}
          netTotalLabel={formatAmount(itemFormNetPreview.netTotal)}
          totalModeToggle={() => {
            itemFormTotalInputMode = itemFormTotalInputMode === 'auto' ? 'manual' : 'auto';
            queueMicrotask(() => recalcItemForm('linePrice'));
          }}
          quantityInput={(value) => {
            itemForm = { ...itemForm, quantity: value };
            queueMicrotask(() => recalcItemForm('quantity'));
          }}
          unitPriceInput={(value) => {
            itemForm = { ...itemForm, unitPrice: value };
            queueMicrotask(() => recalcItemForm('unitPrice'));
          }}
          linePriceInput={(value) => {
            itemForm = { ...itemForm, linePrice: value };
            queueMicrotask(() => recalcItemForm('linePrice'));
          }}
          discountModeSelect={(mode) => setItemFormDiscountMode(mode)}
          discountValueInput={(value) => {
            if (itemFormDiscountMode === 'percent') {
              itemForm = { ...itemForm, discountPercent: value };
            } else {
              itemForm = { ...itemForm, discountCents: value };
            }
            queueMicrotask(() => recalcItemForm('discount'));
          }}
          manageAddons={() => (showItemAddonsModal = true)}
        />
        <div class="flex gap-3 modal-actions">
          <button
            class="btn btn-outline w-full"
            on:click={() => {
              showItemAddonsModal = false;
              showItemModal = false;
              itemModalGroupKey = null;
            }}
          >
            Cancel
          </button>
          <button
            class="btn btn-primary w-full"
            on:click={submitItemModal}
            disabled={
              !(itemModalMode === 'group' || itemModalMode === 'new'
                ? stripGeneratedItemNumberSuffix(splitNameAndAddonLabels(itemForm.name || '').baseName.trim())
                : splitNameAndAddonLabels(itemForm.name || '').baseName.trim())
            }
          >
            {itemModalMode === 'group' ? 'Save group' : itemModalMode === 'edit' ? 'Save' : 'Add'}
          </button>
        </div>
      </div>
      {#if showItemAddonsModal}
        <div class="modal-scrim">
          <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[75vh] overflow-y-auto">
            <h3 class="text-lg font-semibold modal-title">Item Add-ons</h3>
            <p class="text-xs text-surface-300 modal-subtitle">
              Add-on costs are per item and rolled into the item total.
            </p>
            <div class="space-y-2">
              {#if (itemForm.addons || []).length === 0}
                <div class="text-xs text-surface-400">
                  No add-ons yet. Add one below if needed.
                </div>
              {/if}
              {#each itemForm.addons as addon, idx}
                <div class="grid grid-cols-[1fr_7rem_auto] gap-2 items-center rounded-xl border border-surface-700/80 bg-surface-900/55 p-2 modal-list-row">
                  <input
                    class="input w-full"
                    value={addon.name}
                    placeholder="Add-on"
                    on:input={(event) =>
                      updateItemFormAddon(idx, 'name', (event.target as HTMLInputElement).value)}
                  />
                  <input
                    class="input w-full text-right"
                    value={addon.price}
                    placeholder={zeroPlaceholderByCurrency(roomCurrency)}
                    inputmode="decimal"
                    type="number"
                    min="0"
                    step={1 / factorFor(roomCurrency)}
                    on:input={(event) =>
                      updateItemFormAddon(idx, 'price', (event.target as HTMLInputElement).value)}
                  />
                  <button
                    class="action-btn action-btn-danger action-btn-compact"
                    type="button"
                    title="Remove add-on"
                    on:click={() => removeItemFormAddon(idx)}
                  >
                    Remove
                  </button>
                </div>
              {/each}
            </div>
            <button class="btn btn-outline w-full" type="button" on:click={addItemFormAddon}>
              Add Add-on
            </button>
            <div class="rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-2 text-sm flex items-center justify-between ui-inline-metric">
              <span class="text-surface-300">Add-on total per item</span>
              <span class="font-semibold text-white">{formatAmount(itemFormAddonTotalPreviewCents, roomCurrency)}</span>
            </div>
            <div class="rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-2 text-sm flex items-center justify-between ui-inline-metric">
              <span class="text-surface-300">
                Add-on total ({Math.max(1, Number.parseInt(itemForm.quantity || '1', 10) || 1)} item{Math.max(1, Number.parseInt(itemForm.quantity || '1', 10) || 1) === 1 ? '' : 's'})
              </span>
              <span class="font-semibold text-white">{formatAmount(itemFormAddonExtendedTotalPreviewCents, roomCurrency)}</span>
            </div>
            <div class="flex gap-3 modal-actions">
              <button class="btn btn-outline w-full" type="button" on:click={() => (showItemAddonsModal = false)}>
                Close
              </button>
              <button class="btn btn-primary w-full" type="button" on:click={() => (showItemAddonsModal = false)}>
                Done
              </button>
            </div>
          </div>
        </div>
      {/if}
    </div>
  {/if}

  {#if showBillSettingsModal}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold modal-title">Tax, Tip &amp; Adjustments</h3>
        <p class="text-xs text-surface-300 modal-subtitle">Tax uses discounted subtotal; tip uses pre-discount item subtotal. Adjustments are split by item cost.</p>
        <p class="text-xs text-surface-300">
          Tip base (pre-discount items): {formatAmount(preTaxGrossSubtotalCents || 0, roomCurrency)}
        </p>
        <p class="text-xs text-surface-300">
          Taxable subtotal after discounts: {formatAmount(preTaxNetSubtotalCents || 0, roomCurrency)}
        </p>
        <label class="block space-y-1 ui-input-stack">
          <span class="text-sm text-surface-200">
            Tax ({symbolFor(roomCurrency) || roomCurrency})
          </span>
          <input
            class="input w-full"
            bind:value={taxInput}
            inputmode="decimal"
            on:blur={() => {
              taxInput = normalizeRequiredInputByCurrency(taxInput, roomCurrency);
            }}
          />
          <p class="text-xs text-surface-400">
            ≈ {(taxPercent || 0).toFixed(2)}% of taxable subtotal
          </p>
        </label>
        <label class="block space-y-1 ui-input-stack">
          <span class="text-sm text-surface-200">
            Tip ({symbolFor(roomCurrency) || roomCurrency})
          </span>
          <input
            class="input w-full"
            bind:value={tipInput}
            inputmode="decimal"
            on:blur={() => {
              tipInput = normalizeRequiredInputByCurrency(tipInput, roomCurrency);
            }}
          />
          <p class="text-xs text-surface-400">
            ≈ {(tipPercent || 0).toFixed(2)}% of pre-discount items subtotal
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
        <label class="block space-y-1 ui-input-stack">
          <span class="text-sm text-surface-200">
            Bill-wide discount ({symbolFor(roomCurrency) || roomCurrency})
          </span>
          <input
            class="input w-full"
            type="number"
            min="0"
            step={1 / factorFor(roomCurrency)}
            inputmode="decimal"
            bind:value={billDiscountInput}
            on:blur={() => {
              billDiscountInput = normalizeRequiredInputByCurrency(billDiscountInput, roomCurrency);
            }}
          />
          <p class="text-xs text-surface-400">
            ≈ {(billDiscountPercent || 0).toFixed(2)}% of item subtotal
          </p>
        </label>
        <label class="block space-y-1 ui-input-stack">
          <span class="text-sm text-surface-200">
            Bill-wide non-tip charges ({symbolFor(roomCurrency) || roomCurrency})
          </span>
          <input
            class="input w-full"
            type="number"
            min="0"
            step={1 / factorFor(roomCurrency)}
            inputmode="decimal"
            bind:value={billChargesInput}
            on:blur={() => {
              billChargesInput = normalizeRequiredInputByCurrency(billChargesInput, roomCurrency);
            }}
          />
          <p class="text-xs text-surface-400">
            ≈ {(billChargesPercent || 0).toFixed(2)}% of item subtotal
          </p>
        </label>
        <div class="flex gap-3 modal-actions">
          <button class="btn btn-outline w-full" on:click={() => (showBillSettingsModal = false)}>Cancel</button>
          <button
            class="btn btn-primary w-full"
            type="button"
            disabled={!billSettingsDirty}
            on:click={saveBillSettings}
          >
            Save
          </button>
        </div>
        <div class="flex gap-3 modal-actions">
          <button class="btn w-full border-red-300/30 bg-red-500/20 text-red-100 hover:bg-red-500/30" type="button" on:click={clearBillSettingsInputs}>Clear</button>
          <button
            class="btn btn-outline w-full"
            type="button"
            disabled={!billSettingsDirty}
            on:click={syncBillSettingsInputsFromRoom}
          >
            Revert
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showSummary && summaryData}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[84vh]">
        <div class="flex items-center justify-between gap-3 flex-wrap">
          <h3 class="text-lg font-semibold modal-title">Summary</h3>
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
        <div class="flex items-center justify-between gap-2 rounded-xl border border-surface-700/80 bg-surface-900/45 px-3 py-2 ui-panel">
          <div class="text-xs text-surface-300">
            {#if summaryData.converted}
              {roomCurrency} → {summaryData.converted.currency} · rate {summaryData.converted.rate?.toFixed(4)}{summaryData.converted.asOf ? ` · ${summaryData.converted.asOf}` : ''}
            {:else}
              Showing {roomCurrency}
            {/if}
          </div>
          <div class="flex items-center gap-2">
            <button
              class={`action-btn action-btn-compact ${summaryUnsentOnly ? 'action-btn-primary' : 'action-btn-surface'}`}
              type="button"
              on:click={() => (summaryUnsentOnly = !summaryUnsentOnly)}
            >
              {summaryUnsentOnly ? 'Unsent only' : 'Show all'}
            </button>
            <button
              class={`action-btn action-btn-compact ${summaryChargeQueueActive ? 'action-btn-surface' : 'action-btn-primary'}`}
              type="button"
              on:click={startSummaryChargeAllUnsent}
              disabled={summaryChargeQueueActive || summaryUnsentRequestableCount === 0}
              title="Launch each unsent Venmo charge in sequence"
            >
              {#if summaryChargeQueueActive}
                Charging all ({summaryChargeQueue.length} left)
              {:else}
                Charge all unsent ({summaryUnsentRequestableCount})
              {/if}
            </button>
          </div>
        </div>
        {#if summaryChargeQueueActive && summaryChargeQueuePendingResume}
          <p class="text-xs text-cyan-200 px-1">Return from Venmo to auto-launch the next unsent charge.</p>
        {/if}
        <div class="rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 space-y-2 ui-panel">
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm text-surface-300">Bill subtotal</span>
            <span class="text-sm font-semibold text-white flex items-center gap-2">
              <span>{formatAmount(summaryData.net)}</span>
              {#if convertSummaryAmount(summaryData.net) !== null}
                <span class="text-surface-500">→</span>
                <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(summaryData.net)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
              {/if}
            </span>
          </div>
          <div class="h-px bg-surface-700/70"></div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm text-surface-300">Bill total before tip</span>
            <span class="text-sm font-semibold text-white flex items-center gap-2">
              <span>{formatAmount(summaryData.totalBeforeTip)}</span>
              {#if convertSummaryAmount(summaryData.totalBeforeTip) !== null}
                <span class="text-surface-500">→</span>
                <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(summaryData.totalBeforeTip)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
              {/if}
            </span>
          </div>
          <div class="h-px bg-surface-700/70"></div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm text-surface-300">Bill total</span>
            <span class="text-base font-semibold text-white flex items-center gap-2">
              <span>{formatAmount(summaryData.total)}</span>
              {#if convertSummaryAmount(summaryData.total) !== null}
                <span class="text-surface-500">→</span>
                <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(summaryData.total)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
              {/if}
            </span>
          </div>
        </div>
        {#if items.some((it) => Object.values(it.assigned || {}).filter(Boolean).length === 0) || participants.some((p) => !p.finished)}
          <div class="rounded-xl bg-amber-500/15 border border-amber-400/40 px-4 py-3 space-y-1 ui-panel">
            {#if items.some((it) => Object.values(it.assigned || {}).filter(Boolean).length === 0)}
              <p class="text-sm text-amber-100">Some items have not been assigned to anyone.</p>
            {/if}
            {#if participants.some((p) => !p.finished)}
              <p class="text-sm text-amber-100">Not everyone in the room is marked ready.</p>
            {/if}
          </div>
        {/if}
        <div class="space-y-3">
          {#if summaryVisiblePeople.length === 0}
            <div class="rounded-xl border border-surface-700/80 bg-surface-900/45 p-3 text-sm text-surface-300 ui-panel">
              No matching people for this filter.
            </div>
          {/if}
          {#each summaryVisiblePeople as person}
            <div class="border border-surface-800 rounded-xl p-3 space-y-3 summary-person-card ui-panel">
              <div class="space-y-2">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="w-3 h-3 rounded-full shrink-0" style={`background:${person.color};`}></span>
                  <p class="font-semibold truncate">{person.name}</p>
                </div>
                {#if person.id !== identity.userId && summaryPersonVenmoUrl(person)}
                  <div class="flex items-center justify-end gap-2">
                    <button
                      class={`action-btn action-btn-compact ${isSummaryRequestSent(person.id, person.total) ? 'action-btn-surface' : 'action-btn-primary'}`}
                      type="button"
                      on:click={() => setSummaryRequestSent(person.id, person.total, !isSummaryRequestSent(person.id, person.total))}
                      disabled={summaryChargeQueueActive}
                    >
                      {isSummaryRequestSent(person.id, person.total) ? 'Mark unsent' : 'Mark sent'}
                    </button>
                    <button
                      class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-lg border border-[#7cc6ff] bg-[#008CFF] px-3 py-1.5 text-xs font-semibold text-white shadow-sm"
                      type="button"
                      on:click={() => openSummaryPersonVenmoCharge(person)}
                      disabled={summaryChargeQueueActive}
                    >
                      <span class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-white/40 bg-white/20 text-[10px] font-black leading-none">V</span>
                      <span>Charge with Venmo</span>
                    </button>
                  </div>
                {/if}
              </div>

              <div class="space-y-1 text-sm text-surface-200">
                <p class="text-xs uppercase tracking-wide text-surface-300">Item share</p>
                {#if person.items.length === 0}
                  <p class="text-xs text-surface-400">No assigned items.</p>
                {/if}
                {#each person.items as item}
                  <div class="flex justify-between gap-3">
                    <span class="min-w-0 truncate">{summaryItemShareLabel(item)}</span>
                    <span class="shrink-0 flex items-center gap-2">
                      <span>{formatAmount(item.share_cents)}</span>
                      {#if convertSummaryAmount(item.share_cents) !== null}
                        <span class="text-surface-500">→</span>
                        <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(item.share_cents)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                      {/if}
                    </span>
                  </div>
                {/each}
                <div class="flex justify-between gap-3 font-semibold text-white">
                  <span>Item share total</span>
                  <span class="shrink-0 flex items-center gap-2">
                    <span>{formatAmount(person.itemsTotal)}</span>
                    {#if convertSummaryAmount(person.itemsTotal) !== null}
                      <span class="text-surface-500">→</span>
                      <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(person.itemsTotal)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                    {/if}
                  </span>
                </div>
                <div class="h-px bg-surface-700/70 my-1"></div>
                <div class="flex justify-between gap-3">
                  <span>Tax share</span>
                  <span class="shrink-0 flex items-center gap-2">
                    <span>{formatAmount(person.taxShare)}</span>
                    {#if convertSummaryAmount(person.taxShare) !== null}
                      <span class="text-surface-500">→</span>
                      <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(person.taxShare)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                    {/if}
                  </span>
                </div>
                <div class="flex justify-between gap-3">
                  <span>Tip share</span>
                  <span class="shrink-0 flex items-center gap-2">
                    <span>{formatAmount(person.tipShare)}</span>
                    {#if convertSummaryAmount(person.tipShare) !== null}
                      <span class="text-surface-500">→</span>
                      <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(person.tipShare)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                    {/if}
                  </span>
                </div>
                {#if person.billDiscountShare > 0}
                  <div class="flex justify-between gap-3">
                    <span>Bill-wide discount share</span>
                    <span class="shrink-0 flex items-center gap-2">
                      <span>-{formatAmount(person.billDiscountShare)}</span>
                      {#if convertSummaryAmount(person.billDiscountShare) !== null}
                        <span class="text-surface-500">→</span>
                        <span class="text-cyan-200">-{formatCurrency(convertSummaryAmount(person.billDiscountShare)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                      {/if}
                    </span>
                  </div>
                {/if}
                {#if person.billChargesShare > 0}
                  <div class="flex justify-between gap-3">
                    <span>Bill-wide non-tip charges share</span>
                    <span class="shrink-0 flex items-center gap-2">
                      <span>{formatAmount(person.billChargesShare)}</span>
                      {#if convertSummaryAmount(person.billChargesShare) !== null}
                        <span class="text-surface-500">→</span>
                        <span class="text-cyan-200">{formatCurrency(convertSummaryAmount(person.billChargesShare)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}</span>
                      {/if}
                    </span>
                  </div>
                {/if}
              </div>

              <div class="rounded-xl border border-cyan-300/40 bg-cyan-500/12 px-3 py-2">
                <div class="flex items-center justify-between gap-3 text-white">
                  <span class="text-xs uppercase tracking-wide text-cyan-100/90">Total</span>
                  <span class="text-lg font-semibold tabular-nums">{formatAmount(person.total)}</span>
                </div>
                {#if convertSummaryAmount(person.total) !== null}
                  <div class="mt-1 flex justify-end text-sm text-cyan-200">
                    {formatCurrency(convertSummaryAmount(person.total)!, targetCurrency, symbolFor(targetCurrency), exponentFor(targetCurrency))}
                  </div>
                {/if}
              </div>

            </div>
          {/each}
        </div>
        {#if summaryExportError}
          <div class="rounded-lg border border-error-400/40 bg-error-500/10 px-3 py-2 text-xs text-error-100">
            {summaryExportError}
          </div>
        {/if}
        <div class="flex gap-3 modal-actions">
          <button
            class="btn btn-outline w-full"
            type="button"
            on:click={exportSummaryPdf}
            disabled={summaryExporting}
          >
            {summaryExporting ? 'Preparing PDF...' : 'Export PDF'}
          </button>
          <button class="btn btn-primary w-full" type="button" on:click={() => (showSummary = false)}>
            Close
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showNameModal}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold modal-title">Edit profile</h3>
        <p class="text-xs text-surface-300 modal-subtitle">This updates how you appear in this room and on charge links.</p>
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Display name</span>
          <input class="input w-full" bind:value={nameInput} placeholder="Your name" />
        </label>
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Venmo username (optional)</span>
          <input class="input w-full" bind:value={venmoInput} placeholder="Venmo username (optional)" />
        </label>
        <p class="text-xs text-surface-300">Use username only (without @).</p>
        <div class="flex gap-3 modal-actions">
          <button class="btn btn-outline w-full" on:click={() => (showNameModal = false)}>Cancel</button>
          <button
            class="btn btn-primary w-full"
            on:click={() => {
              sendParticipantUpdate(identity.userId, nameInput, true, venmoInput);
              identity.name = nameInput.trim();
              identity.initials = initialsFromName(nameInput);
              identity.venmoUsername = normalizeVenmoUsername(venmoInput);
              localStorage.setItem(`room:${roomCode}:identity`, JSON.stringify(identity));
              rememberIdentityPrefs(identity.name, identity.venmoUsername);
              showNameModal = false;
            }}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if showRoomNameModal}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold modal-title">Rename restaurant</h3>
        <p class="text-xs text-surface-300 modal-subtitle">Update the bill name everyone sees.</p>
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Bill name</span>
          <input class="input w-full" bind:value={roomNameInput} placeholder="Restaurant name" />
        </label>
        <div class="flex gap-3 modal-actions">
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
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold modal-title">Add person</h3>
        <p class="text-xs text-surface-300 modal-subtitle">Select contacts or add someone new.</p>
        <input class="input w-full" bind:value={addPersonSearchFilter} placeholder="Search contacts..." />
        {#if filteredAddPersonContacts.length}
          <div class="rounded-xl border border-surface-700 bg-surface-900 p-2 space-y-1 ui-panel add-person-contact-list">
            {#each filteredAddPersonContacts as contact}
              <label class="flex items-center gap-2 px-2 py-1.5 rounded-lg cursor-pointer add-person-contact-row">
                <input type="checkbox" checked={addPersonSelectedContactIds.has(contact.id)} on:change={() => toggleContactSelection(contact.id)} />
                <span class="text-sm text-white">{contact.name}</span>
                {#if contact.venmoUsername}<span class="text-xs text-surface-300">@{contact.venmoUsername}</span>{/if}
              </label>
            {/each}
          </div>
        {:else if !contacts.length}
          <p class="text-xs text-surface-300">No saved contacts yet. Use "Manage contacts" to create some.</p>
        {:else}
          <p class="text-xs text-surface-300">No contacts match your search.</p>
        {/if}
        {#if addPersonSelectedContactIds.size}
          <button class="btn btn-primary w-full" on:click={addSelectedContactsToRoom}>Add ({addPersonSelectedContactIds.size})</button>
        {/if}
        <hr class="border-surface-700" />
        <p class="text-xs font-medium uppercase tracking-wide text-surface-300">Or add manually</p>
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Name</span>
          <input class="input w-full" bind:value={addPersonName} placeholder="Name" />
        </label>
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Venmo username (optional)</span>
          <input class="input w-full" bind:value={addPersonVenmoInput} placeholder="Venmo username (optional)" />
        </label>
        <div class="flex gap-3 modal-actions">
          <button class="btn btn-outline w-full" on:click={() => (showAddPersonModal = false)}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={addManualPersonToRoom}>Add</button>
        </div>
        <button class="text-xs text-cyan-300 underline mt-1 text-left" type="button" on:click={() => { showAddPersonModal = false; showContactsModal = true; }}>Manage contacts</button>
      </div>
    </div>
  {/if}

  <ContactsModal
    bind:open={showContactsModal}
    on:contactschange={() => { contacts = loadContacts(); }}
    on:close={() => { contacts = loadContacts(); }}
  />

  {#if showQrFullscreen && qrUrlFullscreen}
    <div class="fixed inset-0 z-[80] bg-black/90 p-4">
      <button class="absolute inset-0 h-full w-full cursor-default" type="button" aria-label="Close QR overlay" on:click={() => (showQrFullscreen = false)}></button>
      <div class="relative mx-auto flex h-full w-full max-w-xl flex-col items-center justify-center gap-4">
        <img
          src={qrUrlFullscreen}
          alt="Join room QR code"
          class="w-full rounded-2xl border border-white/20 bg-white p-4 shadow-2xl"
        />
        <button class="btn btn-outline w-full max-w-xs" type="button" on:click={() => (showQrFullscreen = false)}>
          Close QR
        </button>
      </div>
    </div>
  {/if}

  {#if showJoinPrompt}
    <div class="modal-scrim">
      <div class="glass-card bottom-sheet ui-bottom-sheet">
        <h3 class="text-lg font-semibold text-center modal-title">Join this room</h3>
        <p class="text-center text-surface-200 text-sm modal-subtitle">
          Select someone already in room {roomCode?.toUpperCase()} or enter a new name.
        </p>
        {#if roomParticipantsForJoin.length > 0}
          <div class="space-y-2 rounded-lg border border-surface-800 bg-surface-900/40 p-3 ui-panel">
            <p class="text-xs font-medium uppercase tracking-wide text-surface-300">People in room</p>
            <div class="space-y-2">
              {#each roomParticipantsForJoin as participant}
                <button
                  class={`w-full rounded-xl border px-3 py-2 text-left modal-list-row ${
                    joinNameInput.trim().toLowerCase() === participant.name.trim().toLowerCase()
                      ? 'border-cyan-300/55 bg-cyan-500/15'
                      : 'border-surface-700 bg-surface-950/50'
                  }`}
                  type="button"
                  on:click={() => {
                    joinPrefillLocked = true;
                    joinNameInput = participant.name;
                    joinVenmoInput = participant.venmoUsername || '';
                  }}
                >
                  <span class="flex items-center gap-3">
                    <span class="relative inline-flex">
                      <Avatar
                        initials={participant.initials}
                        color={colorHex(participant.colorSeed)}
                        size={32}
                        badge={initialsBadges[participant.id] ? String(initialsBadges[participant.id]) : undefined}
                        title={participant.name}
                      />
                      <span
                        class="absolute -right-0.5 -top-0.5 h-3 w-3 rounded-full border border-surface-950"
                        style={`background:${participant.present ? '#22c55e' : '#64748b'};`}
                        title={participant.present ? 'Active' : 'Inactive'}
                      ></span>
                    </span>
                    <span class="min-w-0">
                      <span class="block text-sm font-medium text-white">{participant.name}</span>
                      <span class="block text-xs text-surface-300">{participant.present ? 'Active now' : 'Inactive'}</span>
                    </span>
                  </span>
                </button>
              {/each}
            </div>
          </div>
        {/if}
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Your name</span>
          <input
            class="input w-full"
            bind:value={joinNameInput}
            placeholder="Your name"
            on:input={() => {
              joinPrefillLocked = true;
            }}
          />
        </label>
        <label class="block modal-field">
          <span class="text-xs font-medium uppercase tracking-wide text-surface-300">Venmo username (optional)</span>
          <input
            class="input w-full"
            bind:value={joinVenmoInput}
            placeholder="Venmo username (optional)"
            on:input={() => {
              joinPrefillLocked = true;
            }}
          />
        </label>
        {#if joinError}
          <div class="text-error-300 text-sm">{joinError}</div>
        {/if}
        <div class="flex gap-3 modal-actions">
          <button class="btn btn-outline w-full" on:click={() => goto('/')}>Cancel</button>
          <button class="btn btn-primary w-full" on:click={joinRoomWithName} disabled={!joinNameInput.trim()}>Join</button>
        </div>
      </div>
    </div>
  {/if}
</div>
