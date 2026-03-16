package server

import "testing"

func TestConsolidateTaggedModifierRowsMergesIntoParentAddon(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "1/2 Chicken Plate",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(1800),
				LinePriceCents: intPtr(1800),
				RawText:        strPtr("1/2 Chicken Plate $18.00"),
			},
			{
				Name:           "Breast",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(300),
				LinePriceCents: intPtr(300),
				RawText:        strPtr("Breast $3.00"),
			},
		},
	}
	tags := []ReceiptModifierTag{
		{Index: 1, Role: "modifier", TargetIndex: intPtr(0), Confidence: 0.91},
	}

	merged := consolidateTaggedModifierRows(result, tags)
	if merged != 1 {
		t.Fatalf("expected 1 merged row, got %d", merged)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 remaining item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.LinePriceCents == nil || *item.LinePriceCents != 2100 {
		t.Fatalf("expected updated line price 2100, got %+v", item.LinePriceCents)
	}
	if len(item.Addons) != 1 {
		t.Fatalf("expected 1 addon, got %d", len(item.Addons))
	}
	if got := item.Addons[0].Name; got != "Breast" {
		t.Fatalf("expected addon name Breast, got %q", got)
	}
	if item.Addons[0].PriceCents == nil || *item.Addons[0].PriceCents != 300 {
		t.Fatalf("expected addon price 300, got %+v", item.Addons[0].PriceCents)
	}
}

func TestConsolidateTaggedModifierRowsSkipsLowConfidence(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "Plate",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(1700),
				LinePriceCents: intPtr(1700),
			},
			{
				Name:           "2x Thigh",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(400),
				LinePriceCents: intPtr(400),
			},
		},
	}
	tags := []ReceiptModifierTag{
		{Index: 1, Role: "modifier", TargetIndex: intPtr(0), Confidence: 0.45},
	}

	merged := consolidateTaggedModifierRows(result, tags)
	if merged != 0 {
		t.Fatalf("expected no merge for low confidence tag, got %d", merged)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected item count unchanged, got %d", len(result.Items))
	}
}

func strPtr(v string) *string {
	value := v
	return &value
}
