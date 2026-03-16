<script lang="ts">
  import ItemPricingEditor from '$lib/components/ItemPricingEditor.svelte';

  export let name = '';
  export let nameLabel = 'Item Name';
  export let namePlaceholder = 'Item name';
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
  export let highlightName = false;
  export let highlightQuantity = false;
  export let highlightUnitPrice = false;
  export let highlightTotal = false;
  export let highlightDiscount = false;
  export let highlightAddons = false;
  export let nameInput: (value: string) => void = () => {};
  export let quantityInput: (value: string) => void = () => {};
  export let unitPriceInput: (value: string) => void = () => {};
  export let linePriceInput: (value: string) => void = () => {};
  export let discountValueInput: (value: string) => void = () => {};
  export let discountModeSelect: (mode: 'amount' | 'percent') => void = () => {};
  export let totalModeToggle: () => void = () => {};
  export let manageAddons: () => void = () => {};
</script>

<label class="block space-y-1 modal-field">
  <span class="text-sm text-surface-200">{nameLabel}</span>
  <input
    class={`input w-full ${highlightName ? 'border-amber-400/65 bg-amber-900/20 ring-1 ring-amber-400/20' : ''}`}
    value={name}
    placeholder={namePlaceholder}
    on:input={(event) => nameInput((event.target as HTMLInputElement).value)}
  />
</label>

<ItemPricingEditor
  {showQuantity}
  {allowTotalToggle}
  {quantity}
  {unitPrice}
  {linePrice}
  {discountCents}
  {discountPercent}
  {discountMode}
  {totalInputMode}
  {currencyLabel}
  {priceStep}
  {addonCount}
  {addonTotalPerItemLabel}
  {pricingTotalEquation}
  {pricingAddonEquation}
  {showAddonEquation}
  {netUnitLabel}
  {netTotalLabel}
  {highlightQuantity}
  {highlightUnitPrice}
  {highlightTotal}
  {highlightDiscount}
  {highlightAddons}
  {quantityInput}
  {unitPriceInput}
  {linePriceInput}
  {discountValueInput}
  {discountModeSelect}
  {totalModeToggle}
  {manageAddons}
/>
