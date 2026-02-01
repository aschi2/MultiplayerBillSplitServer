# CRDT + Realtime Design

## 1. Data model types

```ts
export type RoomDoc = {
  room_id: string;
  name: string;
  items: Record<string, Item>;
  participants: Record<string, Participant>;
  tax_cents: number;
  tip_cents: number;
  seq: number;
  updated_at: number;
};

export type Item = {
  id: string;
  name: string;
  quantity: number;
  unit_price_cents: number;
  line_price_cents: number;
  discount_cents: number;
  discount_percent: number;
  assigned: Record<string, boolean>;
  updated_at: number;
  raw_text?: string;
  warnings?: string[];
};

export type Participant = {
  id: string;
  name: string;
  initials: string;
  color_seed: string;
  updated_at: number;
};

export type Op = {
  id: string;
  actor_id: string;
  timestamp: number;
  kind: 'set_item' | 'remove_item' | 'set_participant' | 'assign_item' | 'set_tax_tip';
  payload: Record<string, unknown>;
};
```

## 2. WebSocket schema

Client → server:

```json
{ "type": "op", "op": { "id": "uuid", "actor_id": "...", "timestamp": 0, "kind": "set_item", "payload": { "item": { } } } }
{ "type": "resync", "last_seq": 12 }
```

Server → client:

```json
{ "type": "snapshot", "seq": 12, "doc": { } }
{ "type": "op", "seq": 13, "op": { } }
{ "type": "ops", "ops": [ { } ] }
{ "type": "ack", "seq": 13 }
```

## 3. CRDT / op-merge approach

- **LWW registers** for scalar fields: item properties, participant properties, tax/tip, using `timestamp`.
- **OR-Set** for items and participants: `set_item` adds or updates, `remove_item` adds a tombstone with timestamp. If a tombstone is newer than an add, the item stays removed.
- **Assignments** are an add/remove map keyed by user ID; `assign_item` uses LWW on the assignment entry.
- **Deterministic ordering** is client-side: sort by item ID for display or by creation timestamp stored in `meta.created_at`.

## 4. Redis key schema + resync

- `room:{roomId}:snapshot` → JSON-encoded `RoomDoc`
- `room:{roomId}:seq` → int
- `room:{roomId}:ops` → list of JSON entries `{ seq, op }`
- Keys share TTL = `ROOM_TTL_SECONDS`

Resync flow:

1. Client reconnects with `last_seq`.
2. Server loads ops with seq > last_seq and sends `ops`.
3. If snapshot missing, server sends empty `RoomDoc`.

## 5. Exact math & penny distribution

- All values stored in integer cents.
- Per-item final cents = `max(0, line_price_cents - discount_cents)`.
- Split item final cents evenly across assigned users; remainder pennies distributed deterministically.
- Tax & tip allocated proportional to each participant pre-tax subtotal.
- Deterministic remainder distribution order = sort user IDs by `hash(roomId + itemId + userId)` and allocate +1 cent in that order until remainder is zero.
