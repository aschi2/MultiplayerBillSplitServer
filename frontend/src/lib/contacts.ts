import { loadFriendGroups } from './friendGroups';

// ── Types ──────────────────────────────────────────────────────────────

export type Contact = {
  id: string;
  name: string;
  venmoUsername?: string;
  savedAt: number;
  lastUsedAt: number;
};

export type RecentPerson = {
  name: string;
  venmoUsername?: string;
  lastSeenAt: number;
};

// ── Constants ──────────────────────────────────────────────────────────

const CONTACTS_STORAGE_KEY = 'divvi_contacts_v1';
const RECENT_STORAGE_KEY = 'divvi_recent_v1';
const MIGRATION_FLAG_KEY = 'divvi_contacts_migrated';
const MAX_CONTACTS = 200;
const MAX_RECENT = 50;

// ── Helpers ────────────────────────────────────────────────────────────

const hasStorage = () => typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';

export const normalizeVenmoUsername = (value: string | null | undefined): string =>
  (value || '')
    .trim()
    .replace(/^@+/, '')
    .replace(/\s+/g, '');

export const generateContactId = (): string =>
  `contact-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;

const normalizeContact = (value: Partial<Contact> | null | undefined): Contact | null => {
  if (!value) return null;
  const id = `${value.id || ''}`.trim();
  const name = `${value.name || ''}`.trim();
  if (!id || !name) return null;
  const now = Date.now();
  const savedAtRaw = Number(value.savedAt);
  const lastUsedAtRaw = Number(value.lastUsedAt);
  return {
    id,
    name,
    venmoUsername: normalizeVenmoUsername(value.venmoUsername) || undefined,
    savedAt: Number.isFinite(savedAtRaw) && savedAtRaw > 0 ? Math.floor(savedAtRaw) : now,
    lastUsedAt: Number.isFinite(lastUsedAtRaw) && lastUsedAtRaw > 0 ? Math.floor(lastUsedAtRaw) : now
  };
};

const normalizeRecent = (value: Partial<RecentPerson> | null | undefined): RecentPerson | null => {
  if (!value) return null;
  const name = `${value.name || ''}`.trim();
  if (!name) return null;
  const now = Date.now();
  const lastSeenAtRaw = Number(value.lastSeenAt);
  return {
    name,
    venmoUsername: normalizeVenmoUsername(value.venmoUsername) || undefined,
    lastSeenAt: Number.isFinite(lastSeenAtRaw) && lastSeenAtRaw > 0 ? Math.floor(lastSeenAtRaw) : now
  };
};

// ── Contacts CRUD ──────────────────────────────────────────────────────

export const loadContacts = (): Contact[] => {
  if (!hasStorage()) return [];
  try {
    const raw = window.localStorage.getItem(CONTACTS_STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // Deduplicate by name (case-insensitive), keeping the newest
    const seen = new Map<string, Contact>();
    for (const item of parsed) {
      const c = normalizeContact(item);
      if (!c) continue;
      const key = c.name.toLowerCase();
      const existing = seen.get(key);
      if (!existing || c.lastUsedAt > existing.lastUsedAt) {
        seen.set(key, c);
      }
    }
    return Array.from(seen.values())
      .sort((a, b) => b.lastUsedAt - a.lastUsedAt)
      .slice(0, MAX_CONTACTS);
  } catch {
    return [];
  }
};

export const saveContacts = (contacts: Contact[]): void => {
  if (!hasStorage()) return;
  const normalized = (Array.isArray(contacts) ? contacts : [])
    .map((c) => normalizeContact(c))
    .filter((c): c is Contact => !!c);
  // Deduplicate by name
  const seen = new Map<string, Contact>();
  for (const c of normalized) {
    const key = c.name.toLowerCase();
    const existing = seen.get(key);
    if (!existing || c.lastUsedAt > existing.lastUsedAt) {
      seen.set(key, c);
    }
  }
  const final = Array.from(seen.values())
    .sort((a, b) => b.lastUsedAt - a.lastUsedAt)
    .slice(0, MAX_CONTACTS);
  window.localStorage.setItem(CONTACTS_STORAGE_KEY, JSON.stringify(final));
};

export const addContact = (name: string, venmoUsername?: string): Contact | null => {
  const trimmed = name.trim();
  if (!trimmed) return null;
  const contacts = loadContacts();
  // Check if already exists
  const key = trimmed.toLowerCase();
  const existing = contacts.find((c) => c.name.toLowerCase() === key);
  if (existing) {
    // Update venmo if provided
    if (venmoUsername !== undefined) {
      existing.venmoUsername = normalizeVenmoUsername(venmoUsername) || undefined;
      existing.lastUsedAt = Date.now();
    }
    saveContacts(contacts);
    return existing;
  }
  const now = Date.now();
  const contact: Contact = {
    id: generateContactId(),
    name: trimmed,
    venmoUsername: normalizeVenmoUsername(venmoUsername) || undefined,
    savedAt: now,
    lastUsedAt: now
  };
  saveContacts([contact, ...contacts]);
  return contact;
};

export const updateContact = (id: string, updates: { name?: string; venmoUsername?: string }): void => {
  const contacts = loadContacts();
  const idx = contacts.findIndex((c) => c.id === id);
  if (idx < 0) return;
  if (updates.name !== undefined) {
    const trimmed = updates.name.trim();
    if (trimmed) contacts[idx].name = trimmed;
  }
  if (updates.venmoUsername !== undefined) {
    contacts[idx].venmoUsername = normalizeVenmoUsername(updates.venmoUsername) || undefined;
  }
  contacts[idx].lastUsedAt = Date.now();
  saveContacts(contacts);
};

export const deleteContact = (id: string): void => {
  const contacts = loadContacts();
  saveContacts(contacts.filter((c) => c.id !== id));
};

export const touchContact = (id: string): void => {
  const contacts = loadContacts();
  const contact = contacts.find((c) => c.id === id);
  if (!contact) return;
  contact.lastUsedAt = Date.now();
  saveContacts(contacts);
};

// ── Recent People LRU ──────────────────────────────────────────────────

export const loadRecentPeople = (): RecentPerson[] => {
  if (!hasStorage()) return [];
  try {
    const raw = window.localStorage.getItem(RECENT_STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    const seen = new Map<string, RecentPerson>();
    for (const item of parsed) {
      const r = normalizeRecent(item);
      if (!r) continue;
      const key = r.name.toLowerCase();
      const existing = seen.get(key);
      if (!existing || r.lastSeenAt > existing.lastSeenAt) {
        seen.set(key, r);
      }
    }
    return Array.from(seen.values())
      .sort((a, b) => b.lastSeenAt - a.lastSeenAt)
      .slice(0, MAX_RECENT);
  } catch {
    return [];
  }
};

const saveRecentPeople = (recent: RecentPerson[]): void => {
  if (!hasStorage()) return;
  const normalized = (Array.isArray(recent) ? recent : [])
    .map((r) => normalizeRecent(r))
    .filter((r): r is RecentPerson => !!r);
  const seen = new Map<string, RecentPerson>();
  for (const r of normalized) {
    const key = r.name.toLowerCase();
    const existing = seen.get(key);
    if (!existing || r.lastSeenAt > existing.lastSeenAt) {
      seen.set(key, r);
    }
  }
  const final = Array.from(seen.values())
    .sort((a, b) => b.lastSeenAt - a.lastSeenAt)
    .slice(0, MAX_RECENT);
  window.localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(final));
};

// Throttle: skip writes if last write was within 60s
let lastTrackTs = 0;

export const trackRecentPeople = (people: Array<{ name: string; venmoUsername?: string }>): void => {
  const now = Date.now();
  if (now - lastTrackTs < 60_000) return;
  lastTrackTs = now;

  const incoming = people
    .map((p) => ({
      name: p.name.trim(),
      venmoUsername: normalizeVenmoUsername(p.venmoUsername) || undefined,
      lastSeenAt: now
    }))
    .filter((p) => p.name);

  if (incoming.length === 0) return;

  const existing = loadRecentPeople();
  const merged = new Map<string, RecentPerson>();
  // Add existing first
  for (const r of existing) {
    merged.set(r.name.toLowerCase(), r);
  }
  // Upsert incoming
  for (const r of incoming) {
    const key = r.name.toLowerCase();
    const prev = merged.get(key);
    merged.set(key, {
      name: r.name,
      venmoUsername: r.venmoUsername || prev?.venmoUsername,
      lastSeenAt: r.lastSeenAt
    });
  }
  saveRecentPeople(Array.from(merged.values()));
};

export const promoteRecentToContact = (name: string): Contact | null => {
  const trimmed = name.trim();
  if (!trimmed) return null;
  const recent = loadRecentPeople();
  const key = trimmed.toLowerCase();
  const person = recent.find((r) => r.name.toLowerCase() === key);
  if (!person) return null;
  return addContact(person.name, person.venmoUsername);
};

// ── Migration ──────────────────────────────────────────────────────────

export const migrateFromFriendGroups = (): void => {
  if (!hasStorage()) return;
  if (localStorage.getItem(MIGRATION_FLAG_KEY)) return;

  const groups = loadFriendGroups();
  if (groups.length === 0) {
    localStorage.setItem(MIGRATION_FLAG_KEY, '1');
    return;
  }

  const existing = loadContacts();
  const existingNames = new Set(existing.map((c) => c.name.trim().toLowerCase()));
  const now = Date.now();
  const migrated: Contact[] = [];

  for (const group of groups) {
    for (const member of group.members) {
      const memberName = member.name.trim();
      const memberKey = memberName.toLowerCase();
      if (!memberName || existingNames.has(memberKey)) continue;
      existingNames.add(memberKey);
      migrated.push({
        id: generateContactId(),
        name: memberName,
        venmoUsername: normalizeVenmoUsername(member.venmoUsername) || undefined,
        savedAt: now,
        lastUsedAt: group.updatedAt || now
      });
    }
  }

  saveContacts([...existing, ...migrated]);
  localStorage.setItem(MIGRATION_FLAG_KEY, '1');
};
