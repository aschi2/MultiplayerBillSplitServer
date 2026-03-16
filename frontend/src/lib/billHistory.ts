export type BillHistoryEntry = {
  roomCode: string;
  billName: string;
  joinedName: string;
  joinedVenmoUsername: string;
  joinedColorSeed: string;
  shareCents: number | null;
  currency: string;
  targetCurrency: string;
  totalCents: number | null;
  convertedShareCents: number | null;
  convertedTotalCents: number | null;
  updatedAt: number;
  ttlSecondsRemaining: number | null;
  ttlFetchedAt: number | null;
};

const BILL_HISTORY_COOKIE = 'divvi_bill_history';
const ONE_YEAR_SECONDS = 60 * 60 * 24 * 365;
const MAX_ENTRIES = 24;
const MAX_COOKIE_ENCODED_BYTES = 3800;

const normalizeVenmoUsername = (value: string) =>
  (value || '')
    .trim()
    .replace(/^@+/, '')
    .replace(/\s+/g, '');

const parseCookieMap = () => {
  const out: Record<string, string> = {};
  if (typeof document === 'undefined') return out;
  const source = document.cookie || '';
  source.split(';').forEach((part) => {
    const raw = part.trim();
    if (!raw) return;
    const eq = raw.indexOf('=');
    if (eq <= 0) return;
    const key = raw.slice(0, eq).trim();
    const value = raw.slice(eq + 1);
    if (!key) return;
    out[key] = value;
  });
  return out;
};

const decodeCookieValue = (value: string | undefined) => {
  if (!value) return '';
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
};

const writeCookie = (key: string, value: string) => {
  if (typeof document === 'undefined') return;
  document.cookie = `${key}=${encodeURIComponent(value)}; Max-Age=${ONE_YEAR_SECONDS}; Path=/; SameSite=Lax`;
};

const normalizeEntry = (value: any): BillHistoryEntry | null => {
  if (!value || typeof value !== 'object') return null;
  const roomCode = `${value.roomCode || ''}`.trim().toUpperCase();
  if (!roomCode) return null;
  const billName = `${value.billName || ''}`.trim();
  const joinedName = `${value.joinedName || ''}`.trim();
  const joinedVenmoUsername = normalizeVenmoUsername(`${value.joinedVenmoUsername || ''}`);
  const joinedColorSeed = `${value.joinedColorSeed || ''}`.trim();
  const currency = `${value.currency || 'USD'}`.trim().toUpperCase() || 'USD';
  const targetCurrency = `${value.targetCurrency || currency || 'USD'}`.trim().toUpperCase() || currency || 'USD';
  const updatedAtRaw = Number(value.updatedAt || Date.now());
  const updatedAt = Number.isFinite(updatedAtRaw) && updatedAtRaw > 0 ? Math.floor(updatedAtRaw) : Date.now();
  const ttlSecondsRaw = Number(value.ttlSecondsRemaining);
  const ttlSecondsRemaining =
    Number.isFinite(ttlSecondsRaw) && ttlSecondsRaw >= 0 ? Math.floor(ttlSecondsRaw) : null;
  const ttlFetchedAtRaw = Number(value.ttlFetchedAt);
  const ttlFetchedAt = Number.isFinite(ttlFetchedAtRaw) && ttlFetchedAtRaw > 0 ? Math.floor(ttlFetchedAtRaw) : null;
  const shareRaw = value.shareCents;
  const shareCents =
    shareRaw === null || shareRaw === undefined
      ? null
      : Number.isFinite(Number(shareRaw))
        ? Math.max(0, Math.round(Number(shareRaw)))
        : null;
  const totalRaw = value.totalCents;
  const totalCents =
    totalRaw === null || totalRaw === undefined
      ? null
      : Number.isFinite(Number(totalRaw))
        ? Math.max(0, Math.round(Number(totalRaw)))
        : null;
  const convertedShareRaw = value.convertedShareCents;
  const convertedShareCents =
    convertedShareRaw === null || convertedShareRaw === undefined
      ? null
      : Number.isFinite(Number(convertedShareRaw))
        ? Math.max(0, Math.round(Number(convertedShareRaw)))
        : null;
  const convertedTotalRaw = value.convertedTotalCents;
  const convertedTotalCents =
    convertedTotalRaw === null || convertedTotalRaw === undefined
      ? null
      : Number.isFinite(Number(convertedTotalRaw))
        ? Math.max(0, Math.round(Number(convertedTotalRaw)))
        : null;
  return {
    roomCode,
    billName,
    joinedName,
    joinedVenmoUsername,
    joinedColorSeed,
    shareCents,
    currency,
    targetCurrency,
    totalCents,
    convertedShareCents,
    convertedTotalCents,
    updatedAt,
    ttlSecondsRemaining,
    ttlFetchedAt
  };
};

const sortByMostRecent = (entries: BillHistoryEntry[]) =>
  [...entries].sort((a, b) => b.updatedAt - a.updatedAt || a.roomCode.localeCompare(b.roomCode));

const dedupeByRoomCode = (entries: BillHistoryEntry[]) => {
  const byRoom = new Map<string, BillHistoryEntry>();
  sortByMostRecent(entries).forEach((entry) => {
    if (!byRoom.has(entry.roomCode)) {
      byRoom.set(entry.roomCode, entry);
    }
  });
  return sortByMostRecent(Array.from(byRoom.values()));
};

const trimToCookieBudget = (entries: BillHistoryEntry[]) => {
  const trimmed = [...entries];
  while (trimmed.length > 0) {
    const encodedLength = encodeURIComponent(JSON.stringify(trimmed)).length;
    if (encodedLength <= MAX_COOKIE_ENCODED_BYTES) {
      break;
    }
    trimmed.pop();
  }
  return trimmed;
};

export const loadBillHistory = (): BillHistoryEntry[] => {
  const map = parseCookieMap();
  const rawValue = decodeCookieValue(map[BILL_HISTORY_COOKIE]);
  if (!rawValue) return [];
  try {
    const parsed = JSON.parse(rawValue);
    if (!Array.isArray(parsed)) return [];
    return dedupeByRoomCode(
      parsed
        .map((entry) => normalizeEntry(entry))
        .filter((entry): entry is BillHistoryEntry => !!entry)
    );
  } catch {
    return [];
  }
};

export const saveBillHistory = (entries: BillHistoryEntry[]) => {
  const normalized = dedupeByRoomCode(
    (entries || [])
      .map((entry) => normalizeEntry(entry))
      .filter((entry): entry is BillHistoryEntry => !!entry)
  ).slice(0, MAX_ENTRIES);
  const bounded = trimToCookieBudget(normalized);
  writeCookie(BILL_HISTORY_COOKIE, JSON.stringify(bounded));
};

export const upsertBillHistoryEntry = (entry: Partial<BillHistoryEntry> & { roomCode: string }) => {
  const normalized = normalizeEntry({
    roomCode: entry.roomCode,
    billName: entry.billName || '',
    joinedName: entry.joinedName || '',
    joinedVenmoUsername: entry.joinedVenmoUsername || '',
    joinedColorSeed: entry.joinedColorSeed || '',
    shareCents: entry.shareCents ?? null,
    currency: entry.currency || 'USD',
    targetCurrency: entry.targetCurrency || entry.currency || 'USD',
    totalCents: entry.totalCents ?? null,
    convertedShareCents: entry.convertedShareCents ?? null,
    convertedTotalCents: entry.convertedTotalCents ?? null,
    updatedAt: entry.updatedAt || Date.now(),
    ttlSecondsRemaining: entry.ttlSecondsRemaining ?? null,
    ttlFetchedAt: entry.ttlFetchedAt ?? null
  });
  if (!normalized) return;

  const current = loadBillHistory();
  const existing = current.find((item) => item.roomCode === normalized.roomCode);
  const merged: BillHistoryEntry = existing
    ? {
        roomCode: existing.roomCode,
        billName: normalized.billName || existing.billName,
        joinedName: normalized.joinedName || existing.joinedName,
        joinedVenmoUsername: normalized.joinedVenmoUsername || existing.joinedVenmoUsername,
        joinedColorSeed: normalized.joinedColorSeed || existing.joinedColorSeed,
        shareCents:
          entry.shareCents === null || entry.shareCents === undefined
            ? existing.shareCents
            : normalized.shareCents,
        currency: normalized.currency || existing.currency,
        targetCurrency: normalized.targetCurrency || existing.targetCurrency || normalized.currency,
        totalCents:
          entry.totalCents === null || entry.totalCents === undefined
            ? existing.totalCents
            : normalized.totalCents,
        convertedShareCents:
          entry.convertedShareCents === null || entry.convertedShareCents === undefined
            ? existing.convertedShareCents
            : normalized.convertedShareCents,
        convertedTotalCents:
          entry.convertedTotalCents === null || entry.convertedTotalCents === undefined
            ? existing.convertedTotalCents
            : normalized.convertedTotalCents,
        updatedAt: normalized.updatedAt || Date.now(),
        ttlSecondsRemaining:
          entry.ttlSecondsRemaining === null || entry.ttlSecondsRemaining === undefined
            ? existing.ttlSecondsRemaining
            : normalized.ttlSecondsRemaining,
        ttlFetchedAt:
          entry.ttlFetchedAt === null || entry.ttlFetchedAt === undefined
            ? existing.ttlFetchedAt
            : normalized.ttlFetchedAt
      }
    : normalized;

  const withoutCurrent = current.filter((item) => item.roomCode !== merged.roomCode);
  saveBillHistory([merged, ...withoutCurrent]);
};

export const removeBillHistoryRoom = (roomCode: string) => {
  const normalizedRoomCode = `${roomCode || ''}`.trim().toUpperCase();
  if (!normalizedRoomCode) return;
  const current = loadBillHistory().filter((entry) => entry.roomCode !== normalizedRoomCode);
  saveBillHistory(current);
};
