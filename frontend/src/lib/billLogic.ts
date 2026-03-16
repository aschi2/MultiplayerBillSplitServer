/**
 * Pure logic functions extracted for testability.
 * These mirror the logic in the main +page.svelte component.
 */
import type { Item, Participant, RoomDoc } from './types';

export const splitEvenCents = (total: number, participantIds: string[]) => {
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

export type SummaryResult = {
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
  perPerson: SummaryPerson[];
};

export type SummaryPerson = {
  id: string;
  name: string;
  items: { item_id: string; name: string; share_cents: number; fraction_numerator: number; fraction_denominator: number }[];
  grossItemsTotal: number;
  itemsTotal: number;
  billDiscountShare: number;
  billChargesShare: number;
  taxShare: number;
  tipShare: number;
  total: number;
};

export const computeSummary = (room: RoomDoc): SummaryResult | null => {
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

  const perPerson = new Map<string, any>();

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
      items: person.items.sort((a: any, b: any) => a.name.localeCompare(b.name)),
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

/** Check if any item has zero assigned people */
export const hasUnassignedItems = (items: Item[]): boolean =>
  items.some((it) => Object.values(it.assigned || {}).filter(Boolean).length === 0);

/** Check if any participant is not marked finished/ready */
export const hasUnreadyParticipants = (participants: Participant[]): boolean =>
  participants.some((p) => !p.finished);

/** Additive bulk assign: merges target into existing assignments without removing others */
export const mergeAssignment = (
  currentAssigned: Record<string, boolean>,
  targetParticipantId: string
): Record<string, boolean> => ({
  ...(currentAssigned || {}),
  [targetParticipantId]: true
});
