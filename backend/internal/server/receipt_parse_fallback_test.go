package server

import (
	"strings"
	"testing"
)

func TestBackfillSparseReceiptItemsFromDenseGuestRows(t *testing.T) {
	raw := strings.Join([]string{
		"6 PRE FIXE LUNCH MBRS 930.00",
		"Guest Number 1 $1 BON 12.50",
		"Guest Number 2 COFFEE 19.00",
		"TROPICAL STORM 13.00",
		"Guest NARDIN DISTRICT LEM 10.50",
		"Guest Number 4 $1 BON 12.50",
		"Guest Number 5 $1 BON 12.50",
		"Guest Number 6 COFFEE 19.00",
		"tip 205.80",
		"SUBTOTAL 1,234.80",
		"TAX 79.75",
		"TOTAL 1,314.55",
	}, "\n")
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "PRE FIXE LUNCH MBRS",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(93000),
				LinePriceCents: intPtr(93000),
				RawText:        &raw,
			},
		},
		SubtotalCents: intPtr(123480),
		TaxCents:      intPtr(7975),
		TotalCents:    intPtr(131455),
	}

	normalizeReceiptParseResult(result)

	if len(result.Items) < 6 {
		t.Fatalf("expected dense fallback to recover multiple items, got %d", len(result.Items))
	}

	bonCount := 0
	coffeeCount := 0
	hasPrefixMeal := false
	for _, item := range result.Items {
		nameLower := strings.ToLower(item.Name)
		if strings.Contains(nameLower, "tip") || strings.Contains(nameLower, "subtotal") ||
			strings.Contains(nameLower, "tax") || strings.Contains(nameLower, "total") {
			t.Fatalf("summary line incorrectly emitted as item: %q", item.Name)
		}
		if strings.Contains(nameLower, "bon") {
			bonCount++
		}
		if strings.Contains(nameLower, "coffee") {
			coffeeCount++
		}
		if strings.Contains(nameLower, "pre fixe") {
			hasPrefixMeal = true
			if item.LinePriceCents == nil || *item.LinePriceCents != 93000 {
				t.Fatalf("expected pre-fixe line price to stay 93000, got %+v", item.LinePriceCents)
			}
		}
	}
	if bonCount < 3 {
		t.Fatalf("expected at least 3 BON rows to be preserved, got %d", bonCount)
	}
	if coffeeCount < 2 {
		t.Fatalf("expected at least 2 COFFEE rows to be preserved, got %d", coffeeCount)
	}
	if !hasPrefixMeal {
		t.Fatal("expected pre-fixe line to be preserved")
	}
}

func TestBackfillSparseReceiptItemsSkipsWhenAlreadyMultiple(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "Coffee",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(300),
				LinePriceCents: intPtr(300),
			},
			{
				Name:           "Bagel",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(450),
				LinePriceCents: intPtr(450),
			},
		},
	}

	backfillSparseReceiptItems(result)
	if got := len(result.Items); got != 2 {
		t.Fatalf("expected existing multi-item parse to be unchanged, got %d items", got)
	}
}

func TestBackfillSparseReceiptItemsSkipsWhenStructuredEnough(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{Name: "Item 1", Quantity: float64Ptr(1), UnitPriceCents: intPtr(100), LinePriceCents: intPtr(100)},
			{Name: "Item 2", Quantity: float64Ptr(1), UnitPriceCents: intPtr(200), LinePriceCents: intPtr(200)},
			{Name: "Item 3", Quantity: float64Ptr(1), UnitPriceCents: intPtr(300), LinePriceCents: intPtr(300)},
			{Name: "Item 4", Quantity: float64Ptr(1), UnitPriceCents: intPtr(400), LinePriceCents: intPtr(400)},
			{Name: "Item 5", Quantity: float64Ptr(1), UnitPriceCents: intPtr(500), LinePriceCents: intPtr(500)},
		},
		UnparsedLines: []string{
			"6 PRE FIXE LUNCH MBRS 930.00",
			"Guest Number 1 $1 BON 12.50",
			"Guest Number 2 COFFEE 19.00",
		},
	}

	backfillSparseReceiptItems(result)

	if got := len(result.Items); got != 5 {
		t.Fatalf("expected structured parse to remain unchanged, got %d items", got)
	}
}

func TestBackfillSparseReceiptItemsSkipsWhenAddonsExist(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "Chicken Plate",
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(1800),
				LinePriceCents: intPtr(1800),
				Addons: []ReceiptAddon{
					{Name: "Breast", PriceCents: intPtr(300)},
				},
			},
		},
		UnparsedLines: []string{
			"Guest Number 1 $1 BON 12.50",
			"Guest Number 2 COFFEE 19.00",
		},
	}

	backfillSparseReceiptItems(result)

	if got := len(result.Items); got != 1 {
		t.Fatalf("expected parse with addons to remain unchanged, got %d items", got)
	}
	if got := len(result.Items[0].Addons); got != 1 {
		t.Fatalf("expected addons to remain unchanged, got %d", got)
	}
}

func TestNormalizeSubtotalFromTotalAndTaxSubtractsTip(t *testing.T) {
	result := &ReceiptParseResult{
		SubtotalCents: intPtr(1234),
		TaxCents:      intPtr(7975),
		TipCents:      intPtr(20580),
		TotalCents:    intPtr(131455),
	}

	normalizeSubtotalFromTotalAndTax(result)

	if result.SubtotalCents == nil {
		t.Fatal("expected subtotal to remain populated")
	}
	if got := *result.SubtotalCents; got != 102900 {
		t.Fatalf("expected subtotal repair to subtract tax and tip, got %d", got)
	}
}

func TestNormalizeFallbackItemNameStripsTrailingCurrencySymbol(t *testing.T) {
	got := normalizeFallbackItemName("Guest Number 1 POMME SI BON $")
	if got != "POMME SI BON" {
		t.Fatalf("expected trailing currency marker to be removed, got %q", got)
	}
}

func TestNormalizeReceiptParseResultStripsTrailingCurrencySymbols(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name: "CLUB 33 COFFEE $",
				Addons: []ReceiptAddon{
					{Name: "SIDE MAC & CHEESE $"},
					{Name: "NO ICE CREAM"},
				},
				Quantity:       float64Ptr(1),
				UnitPriceCents: intPtr(1900),
				LinePriceCents: intPtr(1900),
			},
		},
	}

	normalizeReceiptParseResult(result)

	if got := result.Items[0].Name; got != "CLUB 33 COFFEE" {
		t.Fatalf("expected item name cleaned, got %q", got)
	}
	if got := result.Items[0].Addons[0].Name; got != "SIDE MAC & CHEESE" {
		t.Fatalf("expected addon name cleaned, got %q", got)
	}
	if got := result.Items[0].Addons[1].Name; got != "NO ICE CREAM" {
		t.Fatalf("expected plain addon name unchanged, got %q", got)
	}
}

func TestReceiptParseNeedsQualityFallbackForTinyNoTotals(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{Name: "Fried Green Tomatoes", LinePriceCents: intPtr(800)},
			{Name: "**APP**", LinePriceCents: intPtr(300)},
			{Name: "", LinePriceCents: nil},
		},
	}
	if !receiptParseNeedsQualityFallback(result) {
		t.Fatal("expected quality fallback trigger for tiny parse without totals")
	}
}

func TestReceiptParseNeedsQualityFallbackSkipsSmallCompleteParse(t *testing.T) {
	result := &ReceiptParseResult{
		Items: []ReceiptItem{
			{Name: "Coffee", LinePriceCents: intPtr(500)},
			{Name: "Bagel", LinePriceCents: intPtr(700)},
		},
		SubtotalCents: intPtr(1200),
		TaxCents:      intPtr(96),
		TotalCents:    intPtr(1296),
	}
	if receiptParseNeedsQualityFallback(result) {
		t.Fatal("did not expect quality fallback trigger for complete small parse")
	}
}

func TestReceiptParseQualityScorePrefersStructuredResult(t *testing.T) {
	poor := &ReceiptParseResult{
		Items: []ReceiptItem{
			{Name: "Fried Green Tomatoes", LinePriceCents: intPtr(800)},
			{Name: "", LinePriceCents: nil},
		},
	}
	better := &ReceiptParseResult{
		Items: []ReceiptItem{
			{
				Name:           "1/2 Chicken Plate",
				LinePriceCents: intPtr(1800),
				Addons:         []ReceiptAddon{{Name: "Breast", PriceCents: intPtr(300)}},
			},
			{Name: "Diet Coke", LinePriceCents: intPtr(300)},
		},
		SubtotalCents: intPtr(2100),
		TaxCents:      intPtr(168),
		TotalCents:    intPtr(2268),
		Currency:      "USD",
	}
	if !(receiptParseQualityScore(better) > receiptParseQualityScore(poor)) {
		t.Fatal("expected structured parse to score higher than sparse parse")
	}
}

func float64Ptr(v float64) *float64 {
	value := v
	return &value
}
