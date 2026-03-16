export type Participant = {
  id: string;
  name: string;
  initials: string;
  colorSeed: string;
  venmoUsername?: string;
  present: boolean;
  finished?: boolean;
};

export type ItemAddonMeta = {
  name: string;
  price_cents: number;
  raw_text?: string | null;
};

export type ItemMeta = {
  addons?: ItemAddonMeta[];
  repeated_group_excluded?: boolean;
  [key: string]: any;
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
  sort_order?: number | null;
  raw_text?: string;
  meta?: ItemMeta;
};

export type RoomDoc = {
  room_id: string;
  name: string;
  items: Record<string, Item>;
  participants: Record<string, Participant>;
  tax_cents: number;
  tip_cents: number;
  bill_discount_cents?: number;
  bill_charges_cents?: number;
  currency?: string;
  target_currency?: string;
  seq: number;
};

export type ReceiptItem = {
  name: string;
  quantity: number | null;
  unit_price_cents: number | null;
  line_price_cents: number | null;
  discount_cents: number | null;
  discount_percent: number | null;
  addons?: ReceiptAddon[] | null;
  raw_text: string | null;
};

export type ReceiptAddon = {
  name: string;
  price_cents: number | null;
  raw_text: string | null;
};

export type ReceiptParseResult = {
  merchant?: string | null;
  items: ReceiptItem[];
  subtotal_cents: number | null;
  bill_discount_cents?: number | null;
  bill_charges_cents?: number | null;
  tax_cents: number | null;
  tip_cents?: number | null;
  total_cents: number | null;
  currency?: string | null;
  fees?: string[];
  warnings: string[];
  confidence: number;
  unparsed_lines?: string[];
};
