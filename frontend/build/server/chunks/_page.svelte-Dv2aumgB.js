import { O as ensure_array_like, P as bind_props, Q as attr } from './index2-W7louMnP.js';
import { k as fallback } from './utils2-UOC6HMdI.js';
import { e as escape_html } from './context-R2425nfV.js';

const initialsFromName = (name) => {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0][0]?.toUpperCase() ?? "?";
  return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase();
};
function Avatar($$renderer, $$props) {
  let initials = fallback($$props["initials"], "?");
  let color = fallback($$props["color"], "#94a3b8");
  let size = fallback($$props["size"], 40);
  let badge = fallback($$props["badge"], null);
  let title = $$props["title"];
  $$renderer.push(`<div class="relative inline-block"${attr("title", title || initials)}${attr("aria-label", title || initials)} role="img"><svg${attr("width", size)}${attr("height", size)}${attr("viewBox", `0 0 ${size} ${size}`)} class="rounded-full shadow-sm"><circle${attr("cx", size / 2)}${attr("cy", size / 2)}${attr("r", size / 2)}${attr("fill", color)}></circle><text x="50%" y="50%" dominant-baseline="middle" text-anchor="middle" fill="white"${attr("font-size", size * 0.45)} font-family="'Inter', sans-serif">${escape_html(initials)}</text></svg> `);
  if (badge) {
    $$renderer.push("<!--[-->");
    $$renderer.push(`<span class="absolute -bottom-1 -right-1 min-w-[16px] h-[16px] px-[4px] inline-flex items-center justify-center rounded-full bg-primary-400 text-white text-[10px] leading-none font-semibold shadow" aria-hidden="true">${escape_html(badge)}</span>`);
  } else {
    $$renderer.push("<!--[!-->");
  }
  $$renderer.push(`<!--]--></div>`);
  bind_props($$props, { initials, color, size, badge, title });
}
const COMMON_CURRENCIES = [
  { code: "USD", symbol: "$", exponent: 2, flag: "🇺🇸" },
  { code: "EUR", symbol: "€", exponent: 2, flag: "🇪🇺" },
  { code: "GBP", symbol: "£", exponent: 2, flag: "🇬🇧" },
  { code: "JPY", symbol: "¥", exponent: 0, flag: "🇯🇵" },
  { code: "CAD", symbol: "$", exponent: 2, flag: "🇨🇦" },
  { code: "AUD", symbol: "$", exponent: 2, flag: "🇦🇺" },
  { code: "CHF", symbol: "Fr", exponent: 2, flag: "🇨🇭" },
  { code: "CNY", symbol: "¥", exponent: 2, flag: "🇨🇳" },
  { code: "KRW", symbol: "₩", exponent: 0, flag: "🇰🇷" },
  { code: "MXN", symbol: "$", exponent: 2, flag: "🇲🇽" },
  { code: "SGD", symbol: "$", exponent: 2, flag: "🇸🇬" },
  { code: "HKD", symbol: "$", exponent: 2, flag: "🇭🇰" },
  { code: "INR", symbol: "₹", exponent: 2, flag: "🇮🇳" },
  { code: "SEK", symbol: "kr", exponent: 2, flag: "🇸🇪" },
  { code: "NOK", symbol: "kr", exponent: 2, flag: "🇳🇴" }
];
const EXPONENTS = COMMON_CURRENCIES.reduce((acc, c) => {
  acc[c.code] = c.exponent;
  return acc;
}, {});
COMMON_CURRENCIES.reduce((acc, c) => {
  acc[c.code] = c.symbol;
  return acc;
}, {});
COMMON_CURRENCIES.reduce((acc, c) => {
  acc[c.code] = c.flag;
  return acc;
}, {});
const DEFAULT_CURRENCY = "USD";
function _page($$renderer, $$props) {
  $$renderer.component(($$renderer2) => {
    let data = $$props["data"];
    let roomCode = data.roomCode;
    let identity = { userId: "", name: "", initials: "" };
    let parsedTaxInput = "";
    let receiptCurrencySelection = DEFAULT_CURRENCY;
    let roomCurrency = DEFAULT_CURRENCY;
    let editableItems = [];
    let items = [];
    let participants = [];
    let initialsCounts = {};
    let initialsBadges = {};
    const colorHex = (seed) => {
      return "#94a3b8";
    };
    const exponentFor = (code) => EXPONENTS[code] ?? 2;
    const factorFor = (code) => Math.pow(10, exponentFor(code));
    const toCentsInput = (value, code = roomCurrency) => {
      const num = Number.parseFloat(value || "");
      if (!Number.isFinite(num)) return 0;
      return Math.max(0, Math.round(num * factorFor(code)));
    };
    const parseByCurrency = (value, code = roomCurrency) => {
      const exp = exponentFor(code);
      const factor = Math.pow(10, exp);
      const num = Number.parseFloat("");
      if (!Number.isFinite(num)) return 0;
      return Math.max(0, Math.round(num * factor));
    };
    const subtotalFromEditable = (list) => {
      if (!list?.length) return 0;
      return list.reduce(
        (sum, item) => {
          const qty = Math.max(1, Number.parseInt(item.quantity || "1", 10) || 1);
          const unit = toCentsInput(item.unitPrice, receiptCurrencySelection);
          const line = toCentsInput(item.linePrice, receiptCurrencySelection);
          const gross = line || unit * qty;
          const discountPct = Number.parseFloat(item.discountPercent || "0") || 0;
          const discountCentsPerUnit = toCentsInput(item.discountCents, receiptCurrencySelection) || (unit ? Math.round(unit * (discountPct / 100)) : 0);
          const net = Math.max(0, gross - discountCentsPerUnit * qty);
          return sum + net;
        },
        0
      );
    };
    const preTaxSubtotal = (list) => {
      if (!list?.length) return 0;
      const subtotal = list.reduce(
        (sum, it) => {
          const qty = it.quantity || 1;
          const gross = Number(it.line_price_cents || 0);
          const discount = Number(it.discount_cents || 0) * qty;
          const net = Math.max(0, gross - discount);
          return sum + net;
        },
        0
      );
      return Number.isFinite(subtotal) ? Math.max(0, Math.round(subtotal)) : 0;
    };
    items = [];
    (() => {
      const map = {};
      items.forEach((item) => {
        Object.entries(item.assigned || {}).forEach(([uid, on]) => {
          if (on) map[uid] = true;
        });
      });
      return map;
    })();
    participants = [];
    initialsCounts = participants.reduce(
      (acc, p) => {
        const init = p.initials || initialsFromName(p.name);
        acc[init] = (acc[init] || 0) + 1;
        return acc;
      },
      {}
    );
    initialsBadges = (() => {
      const seen = {};
      const badges = {};
      participants.forEach((p) => {
        const init = p.initials || initialsFromName(p.name);
        seen[init] = (seen[init] || 0) + 1;
        if ((initialsCounts[init] || 0) > 1) {
          badges[p.id] = String(seen[init]);
        }
      });
      return badges;
    })();
    subtotalFromEditable(editableItems);
    parseByCurrency(parsedTaxInput, receiptCurrencySelection);
    preTaxSubtotal(items);
    $$renderer2.push(`<div class="min-h-screen bg-surface-900 text-surface-50 pb-24 relative">`);
    {
      $$renderer2.push("<!--[-->");
      $$renderer2.push(`<div class="absolute top-3 inset-x-0 flex justify-center pointer-events-none z-50 px-4"><div class="flex items-center gap-3 rounded-xl border border-warning-500/60 bg-warning-900/95 text-warning-50 text-xs px-3 py-2 shadow-xl pointer-events-auto"><span>⚠️</span> <span class="flex-1">${escape_html("Connecting…")}</span> <button class="action-btn action-btn-surface action-btn-compact" type="button">Retry</button></div></div>`);
    }
    $$renderer2.push(`<!--]--> <header class="px-6 pt-6 pb-4 space-y-3"><div class="accent-gradient rounded-3xl p-5 shadow-2xl flex flex-col md:flex-row md:items-center md:justify-center gap-4"><div class="text-center flex-1 order-1"><p class="text-sm text-white/70">Restaurant</p> <h1 class="text-2xl font-semibold text-white">${escape_html("Shared Bill")}</h1> <div class="flex items-center justify-center gap-3 mt-2 text-sm text-white/80 flex-wrap"><span class="rounded-full bg-black/20 px-3 py-1 font-mono">Bill code: ${escape_html(roomCode?.toUpperCase())}</span> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--></div></div> <div class="flex flex-col items-center justify-center order-2 md:order-3 md:w-48">`);
    Avatar($$renderer2, {
      initials: identity.initials,
      color: colorHex(),
      size: 60,
      badge: initialsBadges[identity.userId] ? String(initialsBadges[identity.userId]) : void 0,
      title: identity.name
    });
    $$renderer2.push(`<!----> <div class="text-sm text-white mt-2">${escape_html("You")}</div> <div class="flex items-center justify-center gap-2 mt-3 w-full"><button class="action-btn action-btn-surface action-btn-compact text-xs sm:text-sm flex-1"><svg class="inline-block align-middle" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#eab308"><path d="M12 20h9"></path><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5Z"></path></svg> <span class="inline ml-1">Change name</span></button></div> <div class="flex items-center justify-center gap-2 mt-2 w-full"><button class="action-btn action-btn-surface action-btn-compact text-xs sm:text-sm flex-1"><svg class="inline-block align-middle" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round"><path d="M20 12l-8-8H6a2 2 0 0 0-2 2v6l8 8a2 2 0 0 0 2.8 0l5.2-5.2A2 2 0 0 0 20 12Z"></path><path d="M7.5 7.5h.01"></path></svg> <span class="inline-block ml-1 whitespace-normal leading-tight text-left"><span class="block">Rename</span> <span class="block">restaurant</span></span></button> <button class="action-btn action-btn-primary action-btn-compact text-xs sm:text-sm flex-1"><svg class="inline-block align-middle" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round" style="color:#22c55e"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg> <span class="inline-block ml-1 whitespace-normal leading-tight text-left"><span class="block">Add</span> <span class="block">person</span></span></button></div></div></div> <div class="flex flex-col md:flex-row gap-3 items-center justify-center mt-2"><div class="flex items-center gap-2"><span class="text-sm text-white/70">Bill currency:</span> `);
    $$renderer2.select(
      {
        class: "input bg-white/5 border border-white/15 rounded-lg px-3 py-2 text-white text-sm",
        value: roomCurrency
      },
      ($$renderer3) => {
        $$renderer3.push(`<!--[-->`);
        const each_array = ensure_array_like(COMMON_CURRENCIES);
        for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
          let c = each_array[$$index];
          $$renderer3.option({ value: c.code }, ($$renderer4) => {
            $$renderer4.push(`${escape_html(c.flag)} ${escape_html(c.code)} ${escape_html(c.symbol)}`);
          });
        }
        $$renderer3.push(`<!--]-->`);
      }
    );
    $$renderer2.push(`</div></div> <div class="flex gap-3 overflow-x-auto pt-2 justify-center">`);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--></div></header> <main class="px-6 space-y-4">`);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> <section class="space-y-3"><div class="flex items-center justify-between"><h2 class="text-lg font-semibold">Items</h2> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--></div> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--></section></main> <div class="fixed bottom-0 inset-x-0 bg-surface-900/90 border-t border-surface-800 px-6 py-3 flex gap-2 backdrop-blur"><button class="btn btn-primary flex-1">Add Item</button> <button class="btn btn-outline flex-1">Tax/Tip</button> <button class="btn btn-outline flex-1">Summary</button></div> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--> `);
    {
      $$renderer2.push("<!--[!-->");
    }
    $$renderer2.push(`<!--]--></div>`);
    bind_props($$props, { data });
  });
}

export { _page as default };
//# sourceMappingURL=_page.svelte-Dv2aumgB.js.map
