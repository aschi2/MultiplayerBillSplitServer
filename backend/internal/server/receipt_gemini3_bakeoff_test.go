package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGemini3ReceiptBakeoff(t *testing.T) {
	if os.Getenv("RUN_GEMINI3_BAKEOFF") == "" {
		t.Skip("set RUN_GEMINI3_BAKEOFF=1 to run")
	}
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is required")
	}

	imagePath := strings.TrimSpace(os.Getenv("RECEIPT_IMAGE_PATH"))
	if imagePath == "" {
		imagePath = "/var/folders/p1/_wt8wkwd59sf_kwgs_qpz8dr0000gn/T/codex-clipboard-5ZH24g.png"
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	contentType := http.DetectContentType(data)
	normalized, normalizedType, rotated := normalizeReceiptImageOrientation(data, contentType)
	data = normalized
	contentType = normalizedType

	maxOutputTokens := 8192
	if raw := strings.TrimSpace(os.Getenv("RECEIPT_MAX_OUTPUT_TOKENS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxOutputTokens = parsed
		}
	}
	timeoutSec := 90
	if raw := strings.TrimSpace(os.Getenv("RECEIPT_TIMEOUT_SEC")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutSec = parsed
		}
	}
	reqTimeout := time.Duration(timeoutSec) * time.Second
	clientTimeout := reqTimeout + (10 * time.Second)

	models := []string{
		"gemini-flash-lite-latest",
		"gemini-2.5-flash-lite-preview-09-2025",
		"gemini-2.5-flash",
		"gemini-3-flash-preview",
		"gemini-3-pro-preview",
		"gemini-3.1-pro-preview",
		"gemini-3.1-pro-preview-customtools",
	}
	if raw := strings.TrimSpace(os.Getenv("RECEIPT_MODELS")); raw != "" {
		parsed := make([]string, 0, 4)
		for _, part := range strings.Split(raw, ",") {
			model := strings.TrimSpace(part)
			if model == "" {
				continue
			}
			parsed = append(parsed, model)
		}
		if len(parsed) > 0 {
			models = parsed
		}
	}

	type modelResult struct {
		Model           string   `json:"model"`
		HTTPStatus      int      `json:"http_status"`
		Error           string   `json:"error,omitempty"`
		FinishReasons   []string `json:"finish_reasons,omitempty"`
		PromptTokens    int      `json:"prompt_tokens"`
		CandidateTokens int      `json:"candidate_tokens"`
		TotalTokens     int      `json:"total_tokens"`
		RawChars        int      `json:"raw_chars"`
		ItemCount       int      `json:"item_count"`
		PricedCount     int      `json:"priced_item_count"`
		SubtotalCents   *int     `json:"subtotal_cents,omitempty"`
		LinesTotalCents int      `json:"item_lines_total_cents"`
		SubtotalAbsDiff *int     `json:"subtotal_abs_diff,omitempty"`
		TaxCents        *int     `json:"tax_cents,omitempty"`
		TipCents        *int     `json:"tip_cents,omitempty"`
		TotalCents      *int     `json:"total_cents,omitempty"`
		WarningCount    int      `json:"warning_count"`
		Rotated         bool     `json:"rotated_for_parse"`
	}

	results := make([]modelResult, 0, len(models))

	for _, model := range models {
		result := modelResult{
			Model:   model,
			Rotated: rotated,
		}
		payload, err := buildGeminiRequest(data, contentType, model, geminiReceiptTemperatureStandard)
		if err != nil {
			result.Error = fmt.Sprintf("build request: %v", err)
			results = append(results, result)
			continue
		}
		payload, err = overrideMaxOutputTokens(payload, maxOutputTokens)
		if err != nil {
			result.Error = fmt.Sprintf("set max tokens: %v", err)
			results = append(results, result)
			continue
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
		ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
		if err != nil {
			cancel()
			result.Error = fmt.Sprintf("new request: %v", err)
			results = append(results, result)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)

		resp, err := (&http.Client{Timeout: clientTimeout}).Do(req)
		if err != nil {
			cancel()
			result.Error = fmt.Sprintf("http do: %v", err)
			results = append(results, result)
			continue
		}
		result.HTTPStatus = resp.StatusCode
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			result.Error = fmt.Sprintf("read body: %v", err)
			results = append(results, result)
			continue
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			var errResp struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &errResp) == nil && strings.TrimSpace(errResp.Error.Message) != "" {
				result.Error = errResp.Error.Message
			} else {
				result.Error = fmt.Sprintf("status %d", resp.StatusCode)
			}
			results = append(results, result)
			continue
		}

		var response struct {
			Candidates    []geminiCandidate `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
				TotalTokenCount      int `json:"totalTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			result.Error = fmt.Sprintf("decode response: %v", err)
			results = append(results, result)
			continue
		}

		result.PromptTokens = response.UsageMetadata.PromptTokenCount
		result.CandidateTokens = response.UsageMetadata.CandidatesTokenCount
		result.TotalTokens = response.UsageMetadata.TotalTokenCount
		result.FinishReasons = uniqueReasons(response.Candidates)

		raw := firstGeminiText(response.Candidates)
		result.RawChars = len(raw)
		if strings.TrimSpace(raw) == "" {
			result.Error = "empty candidate text"
			results = append(results, result)
			continue
		}

		content := cleanModelJSON(raw)
		var parsed ReceiptParseResult
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			result.Error = fmt.Sprintf("result json parse: %v", err)
			results = append(results, result)
			continue
		}
		normalizeReceiptParseResult(&parsed)
		result.ItemCount = len(parsed.Items)
		result.SubtotalCents = parsed.SubtotalCents
		result.TaxCents = parsed.TaxCents
		result.TipCents = parsed.TipCents
		result.TotalCents = parsed.TotalCents
		result.WarningCount = len(parsed.Warnings)

		priced := 0
		linesTotal := 0
		for _, item := range parsed.Items {
			line := 0
			if item.LinePriceCents != nil && *item.LinePriceCents > 0 {
				line = *item.LinePriceCents
			} else if item.UnitPriceCents != nil && *item.UnitPriceCents > 0 {
				qty := 1.0
				if item.Quantity != nil && *item.Quantity > 0 {
					qty = *item.Quantity
				}
				line = int(float64(*item.UnitPriceCents) * qty)
			}
			if line > 0 {
				priced++
				linesTotal += line
			}
		}
		result.PricedCount = priced
		result.LinesTotalCents = linesTotal
		if parsed.SubtotalCents != nil {
			diff := linesTotal - *parsed.SubtotalCents
			if diff < 0 {
				diff = -diff
			}
			result.SubtotalAbsDiff = &diff
		}
		if os.Getenv("RECEIPT_DEBUG_ITEMS") != "" {
			itemDump := make([]map[string]any, 0, len(parsed.Items))
			for _, item := range parsed.Items {
				entry := map[string]any{
					"name":             item.Name,
					"quantity":         item.Quantity,
					"unit_price_cents": item.UnitPriceCents,
					"line_price_cents": item.LinePriceCents,
					"discount_cents":   item.DiscountCents,
					"addon_count":      len(item.Addons),
				}
				if len(item.Addons) > 0 {
					addons := make([]map[string]any, 0, len(item.Addons))
					for _, addon := range item.Addons {
						addons = append(addons, map[string]any{
							"name":        addon.Name,
							"price_cents": addon.PriceCents,
						})
					}
					entry["addons"] = addons
				}
				itemDump = append(itemDump, entry)
			}
			if blob, err := json.Marshal(itemDump); err == nil {
				t.Logf("model_items %s %s", model, blob)
			}
		}

		results = append(results, result)
	}

	for _, result := range results {
		line, _ := json.Marshal(result)
		t.Logf("model_result %s", line)
	}
}

func overrideMaxOutputTokens(payload []byte, maxOutputTokens int) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	config, _ := body["generation_config"].(map[string]any)
	if config == nil {
		config = map[string]any{}
	}
	config["max_output_tokens"] = maxOutputTokens
	body["generation_config"] = config
	return json.Marshal(body)
}

func uniqueReasons(candidates []geminiCandidate) []string {
	reasons := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		reason := strings.TrimSpace(candidate.FinishReason)
		if reason == "" {
			continue
		}
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return reasons
}
