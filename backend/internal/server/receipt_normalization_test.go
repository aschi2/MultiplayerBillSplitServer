package server

import "testing"

func TestNormalizeReceiptParseResultRepairsLinePriceFromRawTextWhenSubtotalImproves(t *testing.T) {
	qty := 3.0
	line := 3700
	unit := 1233
	subtotal := 5100
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "Sweet & Sour Pork Baby Back Ribs",
				Quantity:       &qty,
				UnitPriceCents: &unit,
				LinePriceCents: &line,
				RawText:        stringPtr("3 Sweet & Sour Pork Baby Back Ribs $51.00"),
			},
		},
		SubtotalCents: &subtotal,
	}

	normalizeReceiptParseResult(result)

	if got := receiptItemLineCents(result.Items[0]); got != 5100 {
		t.Fatalf("expected repaired line total 5100, got %d", got)
	}
	if result.Items[0].UnitPriceCents == nil || *result.Items[0].UnitPriceCents != 1700 {
		t.Fatalf("expected repaired unit price 1700, got %+v", result.Items[0].UnitPriceCents)
	}
}

func TestNormalizeReceiptParseResultKeepsParsedTipTotals(t *testing.T) {
	qty := 1.0
	line := 28400
	unit := 28400
	subtotal := 28400
	tax := 2201
	tip := 5112
	total := 35713
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "Receipt subtotal proxy",
				Quantity:       &qty,
				UnitPriceCents: &unit,
				LinePriceCents: &line,
				RawText:        stringPtr("Subtotal proxy $284.00"),
			},
		},
		SubtotalCents: &subtotal,
		TaxCents:      &tax,
		TipCents:      &tip,
		TotalCents:    &total,
	}

	normalizeReceiptParseResult(result)

	if result.TipCents == nil || *result.TipCents != 5112 {
		t.Fatalf("expected parsed tip to remain 5112, got %+v", result.TipCents)
	}
	if result.TotalCents == nil || *result.TotalCents != 35713 {
		t.Fatalf("expected parsed total to remain 35713, got %+v", result.TotalCents)
	}
}

func stringPtr(value string) *string {
	v := value
	return &v
}
