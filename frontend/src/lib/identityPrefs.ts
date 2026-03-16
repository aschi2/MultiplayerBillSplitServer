export type IdentityPrefs = {
  name: string;
  venmoUsername: string;
};

const NAME_COOKIE = 'divvi_name';
const VENMO_COOKIE = 'divvi_venmo';
const ONE_YEAR_SECONDS = 60 * 60 * 24 * 365;

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

export const loadIdentityPrefs = (): IdentityPrefs => {
  const map = parseCookieMap();
  const name = decodeCookieValue(map[NAME_COOKIE]).trim();
  const venmoUsername = normalizeVenmoUsername(decodeCookieValue(map[VENMO_COOKIE]));
  return { name, venmoUsername };
};

export const saveIdentityPrefs = (name: string, venmoUsername: string) => {
  writeCookie(NAME_COOKIE, (name || '').trim());
  writeCookie(VENMO_COOKIE, normalizeVenmoUsername(venmoUsername || ''));
};

