package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type groundTruthAddon struct {
	NameToken  string
	PriceCents int
}

type groundTruthItem struct {
	Key       string
	LineCents int
	Addons    []groundTruthAddon
}

type groundTruthEval struct {
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
	AddonExpectedTotal        int     `json:"addon_expected_total"`
	AddonParsedTotalOnMatches int     `json:"addon_parsed_total_on_matches"`
	AddonMatched              int     `json:"addon_matched"`
	AddonPrecisionOnMatched   float64 `json:"addon_precision_on_matched"`
	AddonRecall               float64 `json:"addon_recall"`
	SubtotalCents             *int    `json:"subtotal_cents,omitempty"`
	TaxCents                  *int    `json:"tax_cents,omitempty"`
	TotalCents                *int    `json:"total_cents,omitempty"`
	SubtotalAbsDiff           *int    `json:"subtotal_abs_diff,omitempty"`
	TaxAbsDiff                *int    `json:"tax_abs_diff,omitempty"`
	TotalAbsDiff              *int    `json:"total_abs_diff,omitempty"`
	WarningsCount             int     `json:"warnings_count"`
	ConsolidationWarningSeen  bool    `json:"consolidation_warning_seen"`
}

func TestReceiptGroundTruthPerformance(t *testing.T) {
	if os.Getenv("RUN_RECEIPT_GT_EVAL") == "" {
		t.Skip("set RUN_RECEIPT_GT_EVAL=1 to run")
	}
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is required")
	}

	imagePath := strings.TrimSpace(os.Getenv("RECEIPT_IMAGE_PATH"))
	if imagePath == "" {
		imagePath = "/var/folders/p1/_wt8wkwd59sf_kwgs_qpz8dr0000gn/T/codex-clipboard-JzpE6i.png"
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	contentType := http.DetectContentType(data)
	if normalized, normalizedType, rotated := normalizeReceiptImageOrientation(data, contentType); rotated {
		data = normalized
		contentType = normalizedType
	}

	groundTruth := []groundTruthItem{
		{Key: "fried_green_tomatoes", LineCents: 800, Addons: []groundTruthAddon{{NameToken: "ranch", PriceCents: 0}}},
		{Key: "diet_coke", LineCents: 300},
		{Key: "sapporo_12oz", LineCents: 600},
		{
			Key:       "half_chicken_plate",
			LineCents: 1800,
			Addons: []groundTruthAddon{
				{NameToken: "breast", PriceCents: 300},
				{NameToken: "thigh", PriceCents: 200},
				{NameToken: "wing", PriceCents: 0},
				{NameToken: "slaw", PriceCents: 0},
				{NameToken: "mac cheese", PriceCents: 50},
			},
		},
		{
			Key:       "dark_plate",
			LineCents: 1700,
			Addons: []groundTruthAddon{
				{NameToken: "thigh", PriceCents: 400},
				{NameToken: "leg", PriceCents: 0},
				{NameToken: "mac cheese", PriceCents: 50},
				{NameToken: "fries", PriceCents: 50},
			},
		},
		{
			Key:       "pecan_pie",
			LineCents: 700,
			Addons: []groundTruthAddon{
				{NameToken: "no ice cream", PriceCents: 0},
				{NameToken: "cold", PriceCents: 0},
			},
		},
		{
			Key:       "coconut_pie",
			LineCents: 700,
			Addons: []groundTruthAddon{
				{NameToken: "no ice cream", PriceCents: 0},
				{NameToken: "cold", PriceCents: 0},
			},
		},
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
						appendReceiptWarningUnique(parsed, fmt.Sprintf("Consolidated %d likely modifier row%s into add-ons.", merged, map[bool]string{true: "", false: "s"}[merged == 1]))
						normalizeReceiptParseResult(parsed)
					}
				}
			}

			eval := evaluateAgainstGroundTruth(parsed, groundTruth)
			eval.Mode = tc.mode
			eval.PrimaryModel = tc.primaryModel
			eval.FallbackModel = tc.fallbackModel
			eval.UsedModel = usedModel
			eval.UsedParseFallback = usedFallback

			if blob, err := json.Marshal(eval); err == nil {
				t.Logf("ground_truth_eval %s", blob)
			}
			if os.Getenv("RECEIPT_DEBUG_ITEMS") != "" {
				if itemBlob, err := json.Marshal(parsed.Items); err == nil {
					t.Logf("ground_truth_items %s", itemBlob)
				}
			}
		})
	}
}

func parseReceiptWithFallback(
	ctx context.Context,
	apiKey string,
	data []byte,
	contentType string,
	primaryModel string,
	fallbackModel string,
	parseTemperature float64,
) (*ReceiptParseResult, string, bool, error) {
	result, err := callGeminiReceiptParseWithModel(ctx, apiKey, data, contentType, primaryModel, parseTemperature)
	if err == nil {
		return result, primaryModel, false, nil
	}
	primaryErr := err
	if fallbackModel == "" || strings.EqualFold(primaryModel, fallbackModel) {
		return nil, "", false, fmt.Errorf("%s failed (%v)", primaryModel, primaryErr)
	}
	result, err = callGeminiReceiptParseWithModel(ctx, apiKey, data, contentType, fallbackModel, parseTemperature)
	if err == nil {
		return result, fallbackModel, true, nil
	}
	return nil, "", false, fmt.Errorf("%s failed (%v); %s fallback failed (%v)", primaryModel, primaryErr, fallbackModel, err)
}

func evaluateAgainstGroundTruth(result *ReceiptParseResult, groundTruth []groundTruthItem) groundTruthEval {
	eval := groundTruthEval{
		ItemCount: len(result.Items),
	}
	if result == nil {
		return eval
	}

	const expectedSubtotal = 7650
	const expectedTax = 709
	const expectedTotal = 8359

	eval.SubtotalCents = result.SubtotalCents
	eval.TaxCents = result.TaxCents
	eval.TotalCents = result.TotalCents
	eval.WarningsCount = len(result.Warnings)
	for _, warning := range result.Warnings {
		if strings.Contains(strings.ToLower(warning), "consolidated") && strings.Contains(strings.ToLower(warning), "add-ons") {
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

	byKey := map[string][]ReceiptItem{}
	for _, item := range result.Items {
		key := classifyGroundTruthBaseKey(item)
		if key == "" {
			continue
		}
		byKey[key] = append(byKey[key], item)
	}

	expectedCount := len(groundTruth)
	tp := 0
	lineExact := 0
	totalExpectedAddons := 0
	totalMatchedAddons := 0
	totalParsedAddonsOnMatched := 0
	for _, expected := range groundTruth {
		totalExpectedAddons += len(expected.Addons)
		parsedList := byKey[expected.Key]
		if len(parsedList) == 0 {
			continue
		}
		tp++
		first := parsedList[0]
		if line := receiptItemLineCents(first); line == expected.LineCents {
			lineExact++
		}
		totalParsedAddonsOnMatched += len(first.Addons)
		totalMatchedAddons += countMatchedExpectedAddons(first.Addons, expected.Addons)
	}

	eval.BaseTP = tp
	eval.BaseLinePriceExactMatches = lineExact
	eval.AddonExpectedTotal = totalExpectedAddons
	eval.AddonMatched = totalMatchedAddons
	eval.AddonParsedTotalOnMatches = totalParsedAddonsOnMatched

	if len(result.Items) > 0 {
		eval.BasePrecision = float64(tp) / float64(len(result.Items))
	}
	if expectedCount > 0 {
		eval.BaseRecall = float64(tp) / float64(expectedCount)
	}
	if totalParsedAddonsOnMatched > 0 {
		eval.AddonPrecisionOnMatched = float64(totalMatchedAddons) / float64(totalParsedAddonsOnMatched)
	}
	if totalExpectedAddons > 0 {
		eval.AddonRecall = float64(totalMatchedAddons) / float64(totalExpectedAddons)
	}
	return eval
}

func classifyGroundTruthBaseKey(item ReceiptItem) string {
	text := normalizeForGroundTruth(strings.TrimSpace(item.Name + " " + ptrString(item.RawText)))
	switch {
	case containsAllTokens(text, "fried", "green", "tomatoes"):
		return "fried_green_tomatoes"
	case containsAllTokens(text, "diet", "coke"):
		return "diet_coke"
	case containsAllTokens(text, "sapporo"):
		return "sapporo_12oz"
	case containsAllTokens(text, "coconut", "pie"):
		return "coconut_pie"
	case containsAllTokens(text, "pecan", "pie"):
		return "pecan_pie"
	case containsAllTokens(text, "dark", "plate") || containsAllTokens(text, "3", "piece", "dark"):
		return "dark_plate"
	case containsAllTokens(text, "chicken", "plate") && (containsAllTokens(text, "1", "2") || strings.Contains(text, "half")):
		return "half_chicken_plate"
	default:
		return ""
	}
}

func countMatchedExpectedAddons(parsed []ReceiptAddon, expected []groundTruthAddon) int {
	if len(parsed) == 0 || len(expected) == 0 {
		return 0
	}
	used := make([]bool, len(parsed))
	matched := 0
	for _, want := range expected {
		found := -1
		for i, addon := range parsed {
			if used[i] {
				continue
			}
			if !containsAllTokens(normalizeForGroundTruth(addon.Name+" "+ptrString(addon.RawText)), strings.Fields(normalizeForGroundTruth(want.NameToken))...) {
				continue
			}
			price := 0
			if addon.PriceCents != nil {
				price = *addon.PriceCents
			}
			if want.PriceCents == 0 {
				if price == 0 {
					found = i
					break
				}
				continue
			}
			if price == want.PriceCents || price == want.PriceCents-1 || price == want.PriceCents+1 {
				found = i
				break
			}
		}
		if found >= 0 {
			used[found] = true
			matched++
		}
	}
	return matched
}

func containsAllTokens(text string, tokens ...string) bool {
	for _, token := range tokens {
		token = normalizeForGroundTruth(token)
		if token == "" {
			continue
		}
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func normalizeForGroundTruth(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacements := []string{
		"&", " ",
		"/", " ",
		"-", " ",
		"(", " ",
		")", " ",
		".", " ",
		",", " ",
		"$", " ",
		":", " ",
		";", " ",
	}
	replacer := strings.NewReplacer(replacements...)
	s = replacer.Replace(s)
	s = strings.ReplaceAll(s, "1/2", "1 2")
	s = strings.Join(strings.Fields(s), " ")
	return s
}
