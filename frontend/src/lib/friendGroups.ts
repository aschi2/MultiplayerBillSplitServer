export type FriendGroupMember = {
  name: string;
  venmoUsername?: string;
};

export type FriendGroup = {
  id: string;
  name: string;
  members: FriendGroupMember[];
  createdAt: number;
  updatedAt: number;
};

const FRIEND_GROUPS_STORAGE_KEY = 'divvi_friend_groups_v1';
const MAX_GROUPS = 24;
const MAX_MEMBERS_PER_GROUP = 32;

const normalizeVenmoUsername = (value: string | null | undefined) =>
  (value || '')
    .trim()
    .replace(/^@+/, '')
    .replace(/\s+/g, '');

const normalizeMember = (value: Partial<FriendGroupMember> | null | undefined): FriendGroupMember | null => {
  const name = `${value?.name || ''}`.trim();
  if (!name) return null;
  const venmoUsername = normalizeVenmoUsername(value?.venmoUsername || '');
  return { name, venmoUsername: venmoUsername || undefined };
};

const normalizeGroup = (value: Partial<FriendGroup> | null | undefined): FriendGroup | null => {
  if (!value) return null;
  const id = `${value.id || ''}`.trim();
  const name = `${value.name || ''}`.trim();
  if (!id || !name) return null;
  const members = (Array.isArray(value.members) ? value.members : [])
    .map((member) => normalizeMember(member))
    .filter((member): member is FriendGroupMember => !!member)
    .slice(0, MAX_MEMBERS_PER_GROUP);
  const now = Date.now();
  const createdAtRaw = Number(value.createdAt);
  const updatedAtRaw = Number(value.updatedAt);
  return {
    id,
    name,
    members,
    createdAt: Number.isFinite(createdAtRaw) && createdAtRaw > 0 ? Math.floor(createdAtRaw) : now,
    updatedAt: Number.isFinite(updatedAtRaw) && updatedAtRaw > 0 ? Math.floor(updatedAtRaw) : now
  };
};

const dedupeMembersByName = (members: FriendGroupMember[]) => {
  const seen = new Set<string>();
  const deduped: FriendGroupMember[] = [];
  members.forEach((member) => {
    const key = member.name.trim().toLowerCase();
    if (!key || seen.has(key)) return;
    seen.add(key);
    deduped.push(member);
  });
  return deduped;
};

const hasStorage = () => typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';

export const loadFriendGroups = (): FriendGroup[] => {
  if (!hasStorage()) return [];
  try {
    const raw = window.localStorage.getItem(FRIEND_GROUPS_STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map((group) => normalizeGroup(group))
      .filter((group): group is FriendGroup => !!group)
      .slice(0, MAX_GROUPS)
      .sort((a, b) => b.updatedAt - a.updatedAt);
  } catch {
    return [];
  }
};

export const saveFriendGroups = (groups: FriendGroup[]) => {
  if (!hasStorage()) return;
  const normalized = (Array.isArray(groups) ? groups : [])
    .map((group) => normalizeGroup(group))
    .filter((group): group is FriendGroup => !!group)
    .map((group) => ({
      ...group,
      members: dedupeMembersByName(group.members).slice(0, MAX_MEMBERS_PER_GROUP),
      updatedAt: Math.max(group.createdAt, group.updatedAt)
    }))
    .slice(0, MAX_GROUPS);
  window.localStorage.setItem(FRIEND_GROUPS_STORAGE_KEY, JSON.stringify(normalized));
};

export const generateFriendGroupId = () =>
  `group-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;

