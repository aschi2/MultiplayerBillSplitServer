<script lang="ts">
  export let showQuantity = true;
  export let allowTotalToggle = true;
  export let quantity = '1';
  export let unitPrice = '0';
  export let linePrice = '0';
  export let discountCents = '0';
  export let discountPercent = '0';
  export let discountMode: 'amount' | 'percent' = 'amount';
  export let totalInputMode: 'auto' | 'manual' = 'auto';
  export let currencyLabel = '$';
  export let priceStep = 0.01;
  export let addonCount = 0;
  export let addonTotalPerItemLabel = '';
  export let pricingTotalEquation = '';
  export let pricingAddonEquation = '';
  export let showAddonEquation = false;
  export let netUnitLabel = '';
  export let netTotalLabel = '';
  export let highlightQuantity = false;
  export let highlightUnitPrice = false;
  export let highlightTotal = false;
  export let highlightDiscount = false;
  export let highlightAddons = false;
  export let quantityInput: (value: string) => void = () => {};
  export let unitPriceInput: (value: string) => void = () => {};
  export let linePriceInput: (value: string) => void = () => {};
  export let discountValueInput: (value: string) => void = () => {};
  export let discountModeSelect: (mode: 'amount' | 'percent') => void = () => {};
  export let totalModeToggle: () => void = () => {};
  export let manageAddons: () => void = () => {};
</script>

<div
  class={`rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-3 space-y-3 pricing-block ${
    highlightQuantity || highlightUnitPrice || highlightTotal ? 'attention-block' : ''
  }`}
>
  <div class="flex items-center justify-between gap-3">
    <p class="text-sm text-surface-200">Pricing</p>
    {#if allowTotalToggle}
      <button
        class="action-btn action-btn-surface action-btn-compact"
        type="button"
        on:click={totalModeToggle}
      >
        {totalInputMode === 'auto' ? 'Edit total' : 'Auto total'}
      </button>
    {/if}
  </div>
  {#if showQuantity}
    <div class="grid grid-cols-2 gap-3">
      <label class="block">
        <span class="text-sm text-surface-200">Quantity</span>
        <input
          class={`input w-full ${highlightQuantity ? 'input-attention' : ''}`}
          value={quantity}
          placeholder="1"
          inputmode="numeric"
          type="number"
          min="1"
          step="1"
          on:input={(event) => quantityInput((event.target as HTMLInputElement).value)}
        />
      </label>
      <label class="block">
        <span class="text-sm text-surface-200">Unit Price ({currencyLabel})</span>
        <input
          class={`input w-full ${highlightUnitPrice ? 'input-attention' : ''}`}
          value={unitPrice}
          placeholder="0.00"
          inputmode="decimal"
          type="number"
          min="0"
          step={priceStep}
          on:input={(event) => unitPriceInput((event.target as HTMLInputElement).value)}
        />
      </label>
    </div>
    {#if totalInputMode === 'manual'}
      <label class="block">
        <span class="text-sm text-surface-200">Total ({currencyLabel})</span>
        <input
          class={`input w-full ${highlightTotal ? 'input-attention' : ''}`}
          value={linePrice}
          placeholder="0.00"
          inputmode="decimal"
          type="number"
          min="0"
          step={priceStep}
          on:input={(event) => linePriceInput((event.target as HTMLInputElement).value)}
        />
      </label>
    {/if}
    <div class="rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-2 text-xs space-y-1">
      <div class="flex items-center justify-between">
        <span class="text-surface-300">Total</span>
        <span class="text-white">{pricingTotalEquation}</span>
      </div>
      {#if showAddonEquation}
        <div class="flex items-center justify-between">
          <span class="text-surface-300">Add-ons</span>
          <span class="text-white">{pricingAddonEquation}</span>
        </div>
      {/if}
    </div>
  {:else}
    <label class="block">
      <span class="text-sm text-surface-200">Unit Price ({currencyLabel})</span>
      <input
        class="input w-full"
        value={unitPrice}
        placeholder="9.99"
        inputmode="decimal"
        type="number"
        min="0"
        step={priceStep}
        on:input={(event) => unitPriceInput((event.target as HTMLInputElement).value)}
      />
    </label>
  {/if}
</div>

<div class="rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-3 space-y-3 discount-block">
  <label class="block">
    <span class="text-sm text-surface-200">Discount</span>
    <div class="mt-1 flex items-center gap-2">
      <input
        class={`input flex-1 ${highlightDiscount ? 'input-attention' : ''}`}
        value={discountMode === 'percent' ? discountPercent : discountCents}
        placeholder={discountMode === 'percent' ? '0' : '0.00'}
        inputmode="decimal"
        type="number"
        min="0"
        step={discountMode === 'percent' ? 0.01 : priceStep}
        on:input={(event) => discountValueInput((event.target as HTMLInputElement).value)}
      />
      <div class="discount-mode-toggle">
        <button
          class={`discount-mode-btn ${discountMode === 'amount' ? 'is-active' : ''}`}
          type="button"
          on:click={() => discountModeSelect('amount')}
          aria-pressed={discountMode === 'amount'}
        >
          {currencyLabel}
        </button>
        <button
          class={`discount-mode-btn ${discountMode === 'percent' ? 'is-active' : ''}`}
          type="button"
          on:click={() => discountModeSelect('percent')}
          aria-pressed={discountMode === 'percent'}
        >
          %
        </button>
      </div>
    </div>
  </label>
</div>

<div
  class={`rounded-lg border border-surface-700 bg-surface-900/60 px-3 py-2 addons-block ${
    highlightAddons ? 'attention-block' : ''
  }`}
>
  <div class="flex items-center justify-between gap-3">
    <div>
      <p class="text-sm text-surface-200">Add-ons</p>
      <p class="text-xs text-surface-400">
        {addonCount} add-on{addonCount === 1 ? '' : 's'} · per-item total {addonTotalPerItemLabel}
      </p>
    </div>
    <button class="btn btn-outline px-3 py-1 text-xs" type="button" on:click={manageAddons}>
      Manage
    </button>
  </div>
</div>

<div class="rounded-lg bg-surface-800/70 border border-surface-700 px-3 py-2 text-sm flex items-center justify-between net-preview-block">
  <div class="flex flex-col gap-1 w-full">
    <div class="flex items-center justify-between">
      <span class="text-surface-300">Final per item</span>
      <span class="font-semibold text-white">{netUnitLabel}</span>
    </div>
    <div class="flex items-center justify-between">
      <span class="text-surface-300">Final line total</span>
      <span class="font-semibold text-white">{netTotalLabel}</span>
    </div>
  </div>
</div>

<style>
  .pricing-block,
  .discount-block,
  .addons-block,
  .net-preview-block {
    border-color: rgba(103, 232, 249, 0.42);
    box-shadow: 0 10px 24px rgba(2, 6, 23, 0.28);
  }

  .attention-block {
    border-color: rgba(245, 158, 11, 0.58);
    box-shadow:
      inset 0 1px 0 rgba(254, 240, 138, 0.1),
      0 12px 24px rgba(120, 53, 15, 0.22);
    background-image:
      linear-gradient(160deg, rgba(245, 158, 11, 0.12), transparent 44%),
      linear-gradient(180deg, rgba(15, 23, 42, 0.74), rgba(2, 6, 23, 0.66));
  }

  .input-attention {
    border-color: rgba(245, 158, 11, 0.65);
    box-shadow:
      inset 0 1px 0 rgba(254, 240, 138, 0.12),
      0 0 0 1px rgba(245, 158, 11, 0.15);
    background-color: rgba(120, 53, 15, 0.18);
  }

  .discount-mode-toggle {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    align-items: center;
    width: 4.65rem;
    gap: 0.2rem;
    border-radius: 0.75rem;
    padding: 0.2rem;
    border: 1px solid rgba(56, 189, 248, 0.38);
    background:
      linear-gradient(180deg, rgba(15, 23, 42, 0.9), rgba(2, 6, 23, 0.85)),
      linear-gradient(120deg, rgba(56, 189, 248, 0.08), transparent);
    backdrop-filter: blur(6px);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }

  .discount-mode-btn {
    min-height: 2.1rem;
    min-width: 0;
    border: none;
    border-radius: 0.6rem;
    padding: 0 0.4rem;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    color: rgb(203 213 225);
    background: rgba(15, 23, 42, 0.5);
    box-shadow: inset 0 0 0 1px rgba(56, 189, 248, 0.18);
  }

  .discount-mode-btn.is-active {
    color: rgb(239 246 255);
    text-shadow: none;
    background: linear-gradient(135deg, rgba(56, 189, 248, 0.3), rgba(45, 212, 191, 0.24));
    box-shadow: inset 0 0 0 1px rgba(125, 211, 252, 0.48);
  }
</style>
