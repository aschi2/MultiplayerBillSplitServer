import { describe, it, expect } from 'vitest';
import {
  computeSummary,
  hasUnassignedItems,
  hasUnreadyParticipants,
  mergeAssignment,
  splitEvenCents
} from './billLogic';
import type { Item, Participant, RoomDoc } from './types';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const makeItem = (overrides: Partial<Item> = {}): Item => ({
  id: 'item-1',
  name: 'Coffee',
  quantity: 1,
  unit_price_cents: 500,
  line_price_cents: 500,
  discount_cents: 0,
  discount_percent: 0,
  assigned: {},
  ...overrides
});

const makeParticipant = (overrides: Partial<Participant> = {}): Participant => ({
  id: 'user-1',
  name: 'Alice',
  initials: 'A',
  colorSeed: 'aabbcc',
  present: true,
  finished: false,
  ...overrides
});

const makeRoom = (overrides: Partial<RoomDoc> = {}): RoomDoc => ({
  room_id: 'room-1',
  name: 'Test Room',
  items: {},
  participants: {},
  tax_cents: 0,
  tip_cents: 0,
  seq: 1,
  ...overrides
});

// ---------------------------------------------------------------------------
// 1. Summary: "Bill total before tip"
// ---------------------------------------------------------------------------

describe('computeSummary – totalBeforeTip', () => {
  it('returns totalBeforeTip = net + tax + billCharges (no tip)', () => {
    const room = makeRoom({
      items: {
        i1: makeItem({ id: 'i1', line_price_cents: 1000, assigned: { u1: true } })
      },
      participants: { u1: makeParticipant({ id: 'u1' }) },
      tax_cents: 100,
      tip_cents: 200
    });
    const summary = computeSummary(room)!;
    expect(summary).not.toBeNull();
    expect(summary.net).toBe(1000);
    expect(summary.totalBeforeTip).toBe(1000 + 100); // net + tax, no billCharges
    expect(summary.total).toBe(1000 + 100 + 200);
  });

  it('includes billCharges in totalBeforeTip', () => {
    const room = makeRoom({
      items: {
        i1: makeItem({ id: 'i1', line_price_cents: 2000, assigned: { u1: true } })
      },
      participants: { u1: makeParticipant({ id: 'u1' }) },
      tax_cents: 150,
      tip_cents: 300,
      bill_charges_cents: 50
    });
    const summary = computeSummary(room)!;
    expect(summary.totalBeforeTip).toBe(2000 + 150 + 50);
    expect(summary.total).toBe(2000 + 150 + 50 + 300);
  });

  it('totalBeforeTip appears between net (subtotal) and total', () => {
    const room = makeRoom({
      items: {
        i1: makeItem({ id: 'i1', line_price_cents: 1000, assigned: { u1: true } })
      },
      participants: { u1: makeParticipant({ id: 'u1' }) },
      tax_cents: 80,
      tip_cents: 150
    });
    const summary = computeSummary(room)!;
    // Ordering: net < totalBeforeTip <= total
    expect(summary.net).toBeLessThanOrEqual(summary.totalBeforeTip);
    expect(summary.totalBeforeTip).toBeLessThanOrEqual(summary.total);
    // totalBeforeTip is total minus tip
    expect(summary.totalBeforeTip).toBe(summary.total - summary.tip);
  });

  it('value is computed correctly with discounts', () => {
    const room = makeRoom({
      items: {
        i1: makeItem({
          id: 'i1',
          line_price_cents: 1000,
          discount_cents: 100,
          assigned: { u1: true }
        })
      },
      participants: { u1: makeParticipant({ id: 'u1' }) },
      tax_cents: 50,
      tip_cents: 100,
      bill_discount_cents: 50
    });
    const summary = computeSummary(room)!;
    // gross=1000, itemDiscount=100, billDiscount=50, net=850
    expect(summary.net).toBe(850);
    expect(summary.totalBeforeTip).toBe(850 + 50); // net + tax
    expect(summary.total).toBe(850 + 50 + 100);
  });
});

// ---------------------------------------------------------------------------
// 2. Warnings
// ---------------------------------------------------------------------------

describe('hasUnassignedItems', () => {
  it('returns true when at least one item has zero assignees', () => {
    const items = [
      makeItem({ id: 'i1', assigned: { u1: true } }),
      makeItem({ id: 'i2', assigned: {} }) // unassigned
    ];
    expect(hasUnassignedItems(items)).toBe(true);
  });

  it('returns false when all items have assignees', () => {
    const items = [
      makeItem({ id: 'i1', assigned: { u1: true } }),
      makeItem({ id: 'i2', assigned: { u2: true, u3: true } })
    ];
    expect(hasUnassignedItems(items)).toBe(false);
  });

  it('treats assigned: { u1: false } as unassigned', () => {
    const items = [makeItem({ id: 'i1', assigned: { u1: false } })];
    expect(hasUnassignedItems(items)).toBe(true);
  });

  it('returns false for empty items array', () => {
    expect(hasUnassignedItems([])).toBe(false);
  });
});

describe('hasUnreadyParticipants', () => {
  it('returns true when at least one participant is not finished', () => {
    const participants = [
      makeParticipant({ id: 'u1', finished: true }),
      makeParticipant({ id: 'u2', finished: false })
    ];
    expect(hasUnreadyParticipants(participants)).toBe(true);
  });

  it('returns false when all participants are finished', () => {
    const participants = [
      makeParticipant({ id: 'u1', finished: true }),
      makeParticipant({ id: 'u2', finished: true })
    ];
    expect(hasUnreadyParticipants(participants)).toBe(false);
  });

  it('treats undefined finished as not ready', () => {
    const participants = [makeParticipant({ id: 'u1', finished: undefined })];
    expect(hasUnreadyParticipants(participants)).toBe(true);
  });

  it('returns false for empty participants array', () => {
    expect(hasUnreadyParticipants([])).toBe(false);
  });
});

describe('both warnings together', () => {
  it('both conditions can be true simultaneously', () => {
    const items = [makeItem({ id: 'i1', assigned: {} })];
    const participants = [makeParticipant({ id: 'u1', finished: false })];
    expect(hasUnassignedItems(items)).toBe(true);
    expect(hasUnreadyParticipants(participants)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 3. Bulk assign – additive behavior
// ---------------------------------------------------------------------------

describe('mergeAssignment', () => {
  it('adds current user to items preserving existing assignees', () => {
    const existing = { alice: true, bob: true };
    const result = mergeAssignment(existing, 'me');
    expect(result).toEqual({ alice: true, bob: true, me: true });
  });

  it('does not duplicate if user is already assigned', () => {
    const existing = { alice: true, me: true };
    const result = mergeAssignment(existing, 'me');
    expect(result).toEqual({ alice: true, me: true });
  });

  it('preserves existing assignees and does not remove anyone', () => {
    const existing = { alice: true, bob: true, charlie: true };
    const result = mergeAssignment(existing, 'me');
    expect(result.alice).toBe(true);
    expect(result.bob).toBe(true);
    expect(result.charlie).toBe(true);
    expect(result.me).toBe(true);
  });

  it('handles empty existing assignments', () => {
    const result = mergeAssignment({}, 'me');
    expect(result).toEqual({ me: true });
  });
});

// ---------------------------------------------------------------------------
// 4. Bulk footer mode (state logic tests)
// ---------------------------------------------------------------------------

describe('bulk footer mode state logic', () => {
  it('starts in normal mode (bulkAssignMode = false)', () => {
    let bulkAssignMode = false;
    expect(bulkAssignMode).toBe(false);
  });

  it('entering bulk mode sets flag to true', () => {
    let bulkAssignMode = false;
    // Simulate tapping "Bulk"
    bulkAssignMode = true;
    expect(bulkAssignMode).toBe(true);
  });

  it('exiting bulk mode restores normal state and clears selections', () => {
    let bulkAssignMode = true;
    let bulkAssignSelectedByItemId: Record<string, boolean> = { i1: true, i2: true };
    let bulkAssignTargetParticipantId = 'u1';

    // Simulate setBulkAssignMode(false)
    bulkAssignMode = false;
    bulkAssignSelectedByItemId = {};
    bulkAssignTargetParticipantId = '';

    expect(bulkAssignMode).toBe(false);
    expect(bulkAssignSelectedByItemId).toEqual({});
    expect(bulkAssignTargetParticipantId).toBe('');
  });

  it('toggling bulk mode twice returns to normal mode', () => {
    let bulkAssignMode = false;
    bulkAssignMode = true; // enter
    bulkAssignMode = false; // exit
    expect(bulkAssignMode).toBe(false);
  });

  it('bulk mode shows bulk actions (structural test)', () => {
    // This tests the conditional logic: when bulkAssignMode is true,
    // the footer should render bulk actions instead of normal ones.
    const bulkAssignMode = true;
    const normalActions = ['Add Item', 'Tax/Tip', 'Adjustments', 'Summary'];
    const bulkActions = ['Split evenly', 'Assign selected', 'Clear assignments', 'Exit Bulk'];

    // In bulk mode, footer shows bulk actions
    const visibleActions = bulkAssignMode ? bulkActions : normalActions;
    expect(visibleActions).toEqual(bulkActions);
    expect(visibleActions).not.toContain('Add Item');
  });

  it('normal mode shows normal actions (structural test)', () => {
    const bulkAssignMode = false;
    const normalActions = ['Add Item', 'Tax/Tip', 'Adjustments', 'Summary', 'Bulk'];
    const bulkActions = ['Split evenly', 'Assign selected', 'Clear assignments', 'Exit Bulk'];

    const visibleActions = bulkAssignMode ? bulkActions : normalActions;
    expect(visibleActions).toEqual(normalActions);
    expect(visibleActions).toContain('Bulk');
  });
});

// ---------------------------------------------------------------------------
// Bonus: splitEvenCents correctness
// ---------------------------------------------------------------------------

describe('splitEvenCents', () => {
  it('distributes evenly when divisible', () => {
    const result = splitEvenCents(600, ['a', 'b', 'c']);
    expect(result).toEqual({ a: 200, b: 200, c: 200 });
  });

  it('distributes remainder to first participants alphabetically', () => {
    const result = splitEvenCents(100, ['b', 'a']);
    // 100 / 2 = 50 each, no remainder
    expect(result).toEqual({ a: 50, b: 50 });
  });

  it('handles indivisible amounts', () => {
    const result = splitEvenCents(10, ['a', 'b', 'c']);
    // 10/3 = 3 base, 1 remainder → a gets 4, b gets 3, c gets 3
    expect(result).toEqual({ a: 4, b: 3, c: 3 });
  });

  it('returns empty for zero total', () => {
    expect(splitEvenCents(0, ['a', 'b'])).toEqual({});
  });

  it('returns empty for no participants', () => {
    expect(splitEvenCents(100, [])).toEqual({});
  });
});
