package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type simpleReceiptExpectedItem struct {
	Key       string
	LineCents int
	Quantity  float64
}

type simpleReceiptEval struct {
	Mode                      string  `json:"mode"`
	PrimaryModel              string  `json:"primary_model"`
	FallbackModel             string  `json:"fallback_model"`
	UsedModel                 string  `json:"used_model"`
	UsedParseFallback         bool    `json:"used_parse_fallback"`
	ItemCount                 int     `json:"item_count"`
	BaseTP                    int     `json:"base_tp"`
	BasePrecision             float64 `json:"base_precision"`
	BaseRecall                float64 `json:"base_recall"`
	BaseLinePriceExactMatches int     `json:"base_line_price_exact_matches"`
	BaseQuantityExactMatches  int     `json:"base_quantity_exact_matches"`
	ParsedAddonTotal          int     `json:"parsed_addon_total"`
	AddonFalsePositiveCount   int     `json:"addon_false_positive_count"`
	SubtotalCents             *int    `json:"subtotal_cents,omitempty"`
	TaxCents                  *int    `json:"tax_cents,omitempty"`
	TipCents                  *int    `json:"tip_cents,omitempty"`
	TotalCents                *int    `json:"total_cents,omitempty"`
	SubtotalAbsDiff           *int    `json:"subtotal_abs_diff,omitempty"`
	TaxAbsDiff                *int    `json:"tax_abs_diff,omitempty"`
	TotalAbsDiff              *int    `json:"total_abs_diff,omitempty"`
	TipUnexpected             bool    `json:"tip_unexpected"`
	WarningsCount             int     `json:"warnings_count"`
	ConsolidationWarningSeen  bool    `json:"consolidation_warning_seen"`
}

func TestReceiptSimpleFixturePerformance(t *testing.T) {
	if os.Getenv("RUN_RECEIPT_SIMPLE_EVAL") == "" {
		t.Skip("set RUN_RECEIPT_SIMPLE_EVAL=1 to run")
	}
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is required")
	}

	imagePath := strings.TrimSpace(os.Getenv("RECEIPT_SIMPLE_IMAGE_PATH"))
	if imagePath == "" {
		imagePath = "/Users/austin/codex-projects/MultiplayerBillSplitServer/output/playwright/din-tai-fung-simple-fixture.png"
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	contentType := httpDetectContentType(data)
	if normalized, normalizedType, rotated := normalizeReceiptImageOrientation(data, contentType); rotated {
		data = normalized
		contentType = normalizedType
	}

	groundTruth := []simpleReceiptExpectedItem{
		{Key: "cucumber_salad", LineCents: 950, Quantity: 1},
		{Key: "wood_ear_mushrooms_vinegar", LineCents: 950, Quantity: 1},
		{Key: "sweet_sour_pork_baby_back_ribs", LineCents: 5100, Quantity: 3},
		{Key: "pork_xiao_long_bao", LineCents: 3700, Quantity: 2},
		{Key: "vegan_dumplings", LineCents: 1800, Quantity: 1},
		{Key: "shrimp_pork_spicy_wontons", LineCents: 3400, Quantity: 2},
		{Key: "vegan_spicy_wontons", LineCents: 1700, Quantity: 1},
		{Key: "string_beans_garlic", LineCents: 1700, Quantity: 1},
		{Key: "taiwanese_cabbage_garlic", LineCents: 1600, Quantity: 1},
		{Key: "noodles_sesame_sauce", LineCents: 1300, Quantity: 1},
		{Key: "vegan_noodles_sesame_sauce", LineCents: 1300, Quantity: 1},
		{Key: "pork_chop_fried_rice", LineCents: 2100, Quantity: 1},
		{Key: "chocolate_mochi_xlb", LineCents: 1400, Quantity: 1},
		{Key: "sesame_mochi_xlb", LineCents: 1250, Quantity: 1},
		{Key: "side_sea_salt_cream", LineCents: 150, Quantity: 1},
	}

	cases := []struct {
		mode               string
		primaryModel       string
		fallbackModel      string
		parseTemperature   float64
		preferHighAccuracy bool
	}{
		{mode: "standard", primaryModel: geminiModelPrimary, fallbackModel: geminiModelFallback, parseTemperature: geminiReceiptTemperatureStandard, preferHighAccuracy: false},
		{mode: "accurate", primaryModel: geminiModelRetryPrimary, fallbackModel: geminiModelRetryFallback, parseTemperature: geminiReceiptTemperatureRetry, preferHighAccuracy: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			parsed, usedModel, usedFallback, err := parseReceiptWithFallback(
				ctx,
				apiKey,
				data,
				contentType,
				tc.primaryModel,
				tc.fallbackModel,
				tc.parseTemperature,
			)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			normalizeReceiptParseResult(parsed)
			if shouldRunModifierTagging(parsed, tc.preferHighAccuracy) {
				if tags, tagErr := callGeminiModifierTagging(ctx, apiKey, tc.primaryModel, tc.fallbackModel, parsed); tagErr == nil {
					merged := consolidateTaggedModifierRows(parsed, tags)
					if merged > 0 {
						appendReceiptWarningUnique(parsed, "Consolidated modifier rows into add-ons.")
						normalizeReceiptParseResult(parsed)
					}
				}
			}

			eval := evaluateSimpleReceiptAgainstGroundTruth(parsed, groundTruth)
			eval.Mode = tc.mode
			eval.PrimaryModel = tc.primaryModel
			eval.FallbackModel = tc.fallbackModel
			eval.UsedModel = usedModel
			eval.UsedParseFallback = usedFallback

			if blob, err := json.Marshal(eval); err == nil {
				t.Logf("simple_receipt_eval %s", blob)
			}
			if os.Getenv("RECEIPT_DEBUG_ITEMS") != "" {
				if itemBlob, err := json.Marshal(parsed.Items); err == nil {
					t.Logf("simple_receipt_items %s", itemBlob)
				}
			}
		})
	}
}

func evaluateSimpleReceiptAgainstGroundTruth(result *ReceiptParseResult, groundTruth []simpleReceiptExpectedItem) simpleReceiptEval {
	eval := simpleReceiptEval{
		ItemCount: len(result.Items),
	}
	if result == nil {
		return eval
	}

	const expectedSubtotal = 28400
	const expectedTax = 2201
	const expectedTotal = 30601

	eval.SubtotalCents = result.SubtotalCents
	eval.TaxCents = result.TaxCents
	eval.TipCents = result.TipCents
	eval.TotalCents = result.TotalCents
	eval.WarningsCount = len(result.Warnings)
	for _, warning := range result.Warnings {
		lower := strings.ToLower(warning)
		if strings.Contains(lower, "consolidated") && strings.Contains(lower, "add-ons") {
			eval.ConsolidationWarningSeen = true
			break
		}
	}
	if result.SubtotalCents != nil {
		diff := *result.SubtotalCents - expectedSubtotal
		if diff < 0 {
			diff = -diff
		}
		eval.SubtotalAbsDiff = &diff
	}
	if result.TaxCents != nil {
		diff := *result.TaxCents - expectedTax
		if diff < 0 {
			diff = -diff
		}
		eval.TaxAbsDiff = &diff
	}
	if result.TotalCents != nil {
		diff := *result.TotalCents - expectedTotal
		if diff < 0 {
			diff = -diff
		}
		eval.TotalAbsDiff = &diff
	}
	if result.TipCents != nil && *result.TipCents > 0 {
		eval.TipUnexpected = true
	}

	byKey := map[string][]ReceiptItem{}
	parsedAddonTotal := 0
	for _, item := range result.Items {
		parsedAddonTotal += len(item.Addons)
		key := classifySimpleReceiptBaseKey(item)
		if key == "" {
			continue
		}
		byKey[key] = append(byKey[key], item)
	}

	expectedCount := len(groundTruth)
	tp := 0
	lineExact := 0
	qtyExact := 0
	for _, expected := range groundTruth {
		parsedList := byKey[expected.Key]
		if len(parsedList) == 0 {
			continue
		}
		tp++
		first := parsedList[0]
		if receiptItemLineCents(first) == expected.LineCents {
			lineExact++
		}
		if quantityMatches(first.Quantity, expected.Quantity) {
			qtyExact++
		}
	}

	eval.BaseTP = tp
	eval.BaseLinePriceExactMatches = lineExact
	eval.BaseQuantityExactMatches = qtyExact
	eval.ParsedAddonTotal = parsedAddonTotal
	eval.AddonFalsePositiveCount = parsedAddonTotal
	if len(result.Items) > 0 {
		eval.BasePrecision = float64(tp) / float64(len(result.Items))
	}
	if expectedCount > 0 {
		eval.BaseRecall = float64(tp) / float64(expectedCount)
	}
	return eval
}

func classifySimpleReceiptBaseKey(item ReceiptItem) string {
	text := normalizeForGroundTruth(strings.TrimSpace(item.Name + " " + ptrString(item.RawText)))
	switch {
	case containsAllTokens(text, "cucumber", "salad"):
		return "cucumber_salad"
	case containsAllTokens(text, "wood", "ear", "mushrooms", "vinegar"):
		return "wood_ear_mushrooms_vinegar"
	case containsAllTokens(text, "sweet", "sour", "pork", "baby", "back", "ribs"):
		return "sweet_sour_pork_baby_back_ribs"
	case containsAllTokens(text, "pork", "xiao", "long", "bao"):
		return "pork_xiao_long_bao"
	case containsAllTokens(text, "vegan", "dumplings"):
		return "vegan_dumplings"
	case containsAllTokens(text, "shrimp", "pork", "spicy", "wontons"):
		return "shrimp_pork_spicy_wontons"
	case containsAllTokens(text, "vegan", "spicy", "wontons"):
		return "vegan_spicy_wontons"
	case containsAllTokens(text, "string", "beans", "garlic"):
		return "string_beans_garlic"
	case containsAllTokens(text, "taiwanese", "cabbage", "garlic"):
		return "taiwanese_cabbage_garlic"
	case containsAllTokens(text, "vegan", "noodles", "sesame", "sauce"):
		return "vegan_noodles_sesame_sauce"
	case containsAllTokens(text, "noodles", "sesame", "sauce"):
		return "noodles_sesame_sauce"
	case containsAllTokens(text, "pork", "chop", "fried", "rice"):
		return "pork_chop_fried_rice"
	case containsAllTokens(text, "chocolate", "mochi", "xlb"):
		return "chocolate_mochi_xlb"
	case containsAllTokens(text, "sesame", "mochi", "xlb"):
		return "sesame_mochi_xlb"
	case containsAllTokens(text, "side", "sea", "salt", "cream"):
		return "side_sea_salt_cream"
	default:
		return ""
	}
}

func quantityMatches(quantity *float64, expected float64) bool {
	if quantity == nil {
		return expected == 1
	}
	diff := *quantity - expected
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.001
}

func httpDetectContentType(data []byte) string {
	return strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0])
}
