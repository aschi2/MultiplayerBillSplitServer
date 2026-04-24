package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	geminiModelPrimary  = "gemini-2.5-flash-lite"
	geminiModelFallback = "gemini-2.5-flash-lite"

	// Try-again/high-accuracy pass uses a stronger model; fallback keeps
	// availability high when preview tiers are unavailable.
	geminiModelRetryPrimary  = "gemini-3.1-pro-preview"
	geminiModelRetryFallback = "gemini-2.5-flash-lite"

	// Tuned from live hard/simple receipt runs.
	// Retry uses thinkingLevel=LOW which consumes some of the output budget for
	// reasoning, AND a strict response schema. The cap must accommodate both
	// the thinking and the full JSON for long receipts; the previous 4000 cap
	// frequently truncated mid-JSON on receipts with many items, especially
	// when fallback to flash-lite kicked in.
	geminiReceiptMaxOutputTokensStandard = 6800
	geminiReceiptMaxOutputTokensRetry    = 12000
	geminiReceiptTemperatureStandard     = 0.0
	geminiReceiptTemperatureRetry        = 0.0
)

type ReceiptParseResult struct {
	Merchant          string        `json:"merchant,omitempty"`
	Items             []ReceiptItem `json:"items"`
	SubtotalCents     *int          `json:"subtotal_cents"`
	BillDiscountCents *int          `json:"bill_discount_cents"`
	BillChargesCents  *int          `json:"bill_charges_cents"`
	TaxCents          *int          `json:"tax_cents"`
	TipCents          *int          `json:"tip_cents"`
	TotalCents        *int          `json:"total_cents"`
	Currency          string        `json:"currency,omitempty"`
	Fees              []string      `json:"fees,omitempty"`
	Warnings          []string      `json:"warnings"`
	Confidence        float64       `json:"confidence"`
	UnparsedLines     []string      `json:"unparsed_lines,omitempty"`
}

type ReceiptItem struct {
	Name            string         `json:"name"`
	Quantity        *float64       `json:"quantity"`
	UnitPriceCents  *int           `json:"unit_price_cents"`
	LinePriceCents  *int           `json:"line_price_cents"`
	DiscountCents   *int           `json:"discount_cents"`
	DiscountPercent *float64       `json:"discount_percent"`
	Addons          []ReceiptAddon `json:"addons,omitempty"`
	RawText         *string        `json:"raw_text"`
}

type ReceiptAddon struct {
	Name       string  `json:"name"`
	PriceCents *int    `json:"price_cents"`
	RawText    *string `json:"raw_text"`
}

type geminiCandidate struct {
	FinishReason string `json:"finishReason"`
	Content      struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"content"`
}

type ReceiptModifierTag struct {
	Index       int     `json:"index"`
	Role        string  `json:"role"`
	TargetIndex *int    `json:"target_index"`
	Confidence  float64 `json:"confidence"`
}

func callOpenAIReceiptParse(ctx context.Context, apiKey string, image []byte, contentType string) (*ReceiptParseResult, error) {
	payload, err := buildOpenAIRequest(image, contentType)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai error: status %d", resp.StatusCode)
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if len(response.Choices) == 0 {
		return nil, errors.New("no response from OpenAI")
	}
	content := response.Choices[0].Message.Content
	var result ReceiptParseResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, err
	}
	result.Currency = normalizeCurrencyCode(result.Currency)
	return &result, nil
}

func callGeminiReceiptParseWithModel(ctx context.Context, apiKey string, image []byte, contentType, model string, temperature float64) (*ReceiptParseResult, error) {
	payload, err := buildGeminiRequest(image, contentType, model, temperature)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	// Dense receipt OCR can legitimately take longer for higher-cap parses.
	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("%s error: %s", model, errResp.Error.Message)
		}
		return nil, fmt.Errorf("%s error: status %d", model, resp.StatusCode)
	}

	var response struct {
		Candidates     []geminiCandidate `json:"candidates"`
		PromptFeedback struct {
			BlockReason        string `json:"blockReason"`
			BlockReasonMessage string `json:"blockReasonMessage"`
		} `json:"promptFeedback"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	raw := firstGeminiText(response.Candidates)
	if strings.TrimSpace(raw) == "" {
		detail := strings.TrimSpace(response.PromptFeedback.BlockReasonMessage)
		if detail == "" {
			detail = strings.TrimSpace(response.PromptFeedback.BlockReason)
		}
		if detail == "" {
			reasons := make([]string, 0, len(response.Candidates))
			for _, candidate := range response.Candidates {
				reason := strings.TrimSpace(candidate.FinishReason)
				if reason == "" {
					continue
				}
				seen := false
				for _, existing := range reasons {
					if existing == reason {
						seen = true
						break
					}
				}
				if !seen {
					reasons = append(reasons, reason)
				}
			}
			if len(reasons) > 0 {
				detail = "finish_reason=" + strings.Join(reasons, ",")
			}
		}
		if detail == "" {
			detail = "empty candidate text"
		}
		return nil, fmt.Errorf("no response from %s (%s)", model, detail)
	}
	content := cleanModelJSON(raw)
	result, err := decodeReceiptParseResult(content)
	if err != nil {
		return nil, err
	}
	result.Currency = normalizeCurrencyCode(result.Currency)
	return result, nil
}

func callGeminiReceiptParse(ctx context.Context, apiKey string, image []byte, contentType string) (*ReceiptParseResult, error) {
	return callGeminiReceiptParseWithModel(
		ctx,
		apiKey,
		image,
		contentType,
		geminiModelPrimary,
		geminiReceiptTemperatureStandard,
	)
}

func callGeminiModifierTagging(ctx context.Context, apiKey, primaryModel, fallbackModel string, result *ReceiptParseResult) ([]ReceiptModifierTag, error) {
	tags, err := callGeminiModifierTaggingWithModel(ctx, apiKey, primaryModel, result)
	if err == nil {
		return tags, nil
	}
	primaryErr := err
	if fallbackModel == "" || strings.EqualFold(primaryModel, fallbackModel) {
		return nil, primaryErr
	}
	tags, err = callGeminiModifierTaggingWithModel(ctx, apiKey, fallbackModel, result)
	if err == nil {
		return tags, nil
	}
	return nil, fmt.Errorf(
		"%s modifier-tagging failed (%v); %s fallback failed (%v)",
		primaryModel,
		primaryErr,
		fallbackModel,
		err,
	)
}

func callGeminiModifierTaggingWithModel(ctx context.Context, apiKey, model string, result *ReceiptParseResult) ([]ReceiptModifierTag, error) {
	payload, err := buildGeminiModifierTaggingRequest(result)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("%s error: %s", model, errResp.Error.Message)
		}
		return nil, fmt.Errorf("%s error: status %d", model, resp.StatusCode)
	}

	var response struct {
		Candidates []geminiCandidate `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	raw := firstGeminiText(response.Candidates)
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("no modifier-tagging response from %s", model)
	}

	content := cleanModelJSON(raw)
	var parsed struct {
		Rows []ReceiptModifierTag `json:"rows"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, err
	}

	rows := make([]ReceiptModifierTag, 0, len(parsed.Rows))
	for _, row := range parsed.Rows {
		role := strings.ToLower(strings.TrimSpace(row.Role))
		if role != "modifier" {
			role = "base"
		}
		confidence := row.Confidence
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		rows = append(rows, ReceiptModifierTag{
			Index:       row.Index,
			Role:        role,
			TargetIndex: row.TargetIndex,
			Confidence:  confidence,
		})
	}
	return rows, nil
}

func buildGeminiModifierTaggingRequest(result *ReceiptParseResult) ([]byte, error) {
	if result == nil {
		return nil, errors.New("nil receipt result")
	}
	rows := make([]map[string]any, 0, len(result.Items))
	for idx, item := range result.Items {
		row := map[string]any{
			"index": idx,
			"name":  strings.TrimSpace(item.Name),
		}
		if item.Quantity != nil {
			row["quantity"] = *item.Quantity
		} else {
			row["quantity"] = nil
		}
		if item.UnitPriceCents != nil {
			row["unit_price_cents"] = *item.UnitPriceCents
		} else {
			row["unit_price_cents"] = nil
		}
		if item.LinePriceCents != nil {
			row["line_price_cents"] = *item.LinePriceCents
		} else {
			row["line_price_cents"] = nil
		}
		raw := strings.TrimSpace(ptrString(item.RawText))
		if raw == "" {
			row["raw_text"] = nil
		} else {
			row["raw_text"] = raw
		}
		rows = append(rows, row)
	}
	rowsJSON, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	unparsed := make([]string, 0, 12)
	for _, line := range result.UnparsedLines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		unparsed = append(unparsed, clean)
		if len(unparsed) >= 12 {
			break
		}
	}
	unparsedJSON, err := json.Marshal(unparsed)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]any{
				{
					"text": strings.Join([]string{
						"You classify parsed receipt rows as either base or modifier.",
						"Return ONLY JSON in the form: {\"rows\":[{\"index\":0,\"role\":\"base|modifier\",\"target_index\":null|number,\"confidence\":0..1}]}.",
						"Do not invent rows; use only provided indexes.",
						"Use raw_text and row context to decide likely modifier rows.",
						"2x / x2 / multiplier markers alone are NOT enough to classify a row as modifier.",
						"If uncertain, mark base and set lower confidence.",
						"target_index is required only when role=modifier and should normally point to a nearby earlier base row.",
					}, " "),
				},
			},
		},
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"text": "Classify these parsed rows:\nrows=" + string(rowsJSON) + "\nunparsed_lines=" + string(unparsedJSON),
					},
				},
			},
		},
		"generation_config": map[string]any{
			"temperature":        0.0,
			"max_output_tokens":  2048,
			"response_mime_type": "application/json",
		},
	}
	return json.Marshal(body)
}

func buildOpenAIRequest(image []byte, contentType string) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(image)
	schema := `{
  "merchant": "string or null",
  "items": [
    {
      "name": "string",
      "quantity": "number or null",
      "unit_price_cents": "int or null",
      "line_price_cents": "int or null",
      "discount_cents": "int or null",
      "discount_percent": "number or null",
      "addons": [
        {
          "name": "string",
          "price_cents": "int or null",
          "raw_text": "string or null"
        }
      ]
    }
  ],
  "subtotal_cents": "int or null",
  "bill_discount_cents": "int or null",
  "bill_charges_cents": "int or null",
  "tax_cents": "int or null",
  "tip_cents": "int or null",
  "total_cents": "int or null",
  "currency": "string or null",
  "fees": "array of strings",
  "warnings": "array of strings",
  "confidence": "number between 0 and 1",
  "unparsed_lines": "array of strings"
}`
	supported := strings.Join(supportedCurrencyCodes(), ", ")
	accuracyGuidance := strings.Join([]string{
		"Work slowly and carefully; prioritize correctness over recall.",
		"Use a two-pass process internally: (1) orient and identify the purchased-item section, (2) extract items and reconcile arithmetic.",
		"Before extracting, evaluate receipt orientation across 0/90/180/270 rotations and use the orientation that yields the most coherent item-price lines.",
		"If the image includes a print-preview screen, webpage chrome, side thumbnails, or table borders, ignore those and parse only the largest actual receipt block.",
		"If this is a photo/screenshot of another screen or printed journal page, ignore UI chrome, side rails, borders, and non-receipt overlays; parse only the receipt body.",
		"Treat only purchasable item lines as items. Exclude summary, tax, total, payment, loyalty, store metadata, and timestamp/ID lines.",
		"Do not stop after the first expensive line item; continue extracting all itemized purchasable lines until subtotal/tax/tip/total/payment section begins.",
		"When an item name and its amount are split across adjacent lines, merge them into one item (attach a price-only line to the nearest valid item line).",
		"For POS/restaurant journals with guest headers (e.g., 'Guest Number', 'Guest #', seat markers), treat those headers as context only and still extract each priced food/drink line beneath them.",
		"For guest/seat-prefixed rows (e.g., 'Guest Number N ... ITEM ... PRICE'), parse ITEM as name and the row's rightmost price as line_price_cents.",
		"For club/journal layouts where a pre-fixe line is followed by many guest rows, do not stop at the pre-fixe line; each subsequent priced guest row is an item.",
		"If multiple rows repeat a prefix like 'Guest Number' or '$1 BON', emit one item per priced row and preserve repeats via separate rows or quantity.",
		"Do not invent or duplicate items. Include an item only when supported by explicit receipt text, and duplicate only when clearly repeated as separate lines or explicit quantity markers.",
		"Prefer explicit quantity markers (x2, qty, multipliers) when present.",
		"If both unit price and line total are available and line total is a clean multiple of unit price, use per-unit unit_price_cents and set quantity = line_total / unit_price.",
		"If a line has discount semantics (e.g., OFF, discount/coupon labels, 割引, 値引, negative amount) and is adjacent to an item line, attach it to that item as discount_cents rather than creating a separate item.",
		"If a discount line contains a percentage marker (e.g., '30%OFF', '３０％ＯＦＦ') adjacent to an item, treat it as item-level discount for that specific adjacent item, not bill-level discount.",
		"If a line is clearly an add-on/modifier for the previous item (e.g., '+ chicken', 'add bacon', 'extra cheese'), attach it to that item via the addons array instead of a separate item.",
		"When add-ons are attached, roll their price into the parent item's line_price_cents (gross/pre-discount) so totals remain correct without separate add-on items.",
		"If a candidate line is primarily a discount label (sale/discount/coupon/off style wording) and does not identify a concrete product, do not emit it as an item; treat it as a discount adjustment.",
		"Do not output standalone negative-priced items for coupons/discount adjustments when they clearly apply to an item. Attach them to that item using discount_cents (per-unit) and keep line_price_cents as gross pre-discount.",
		"If a discount clearly applies to the whole receipt (order-level or bill-level), set bill_discount_cents to the total discount amount in non-negative cents.",
		"Do not double-count discounts: represent each discount either as item discount_cents or as bill_discount_cents, not both.",
		"If a non-tip, bill-wide fee/charge appears (service fee, admin fee, convenience fee, surcharge), set bill_charges_cents to the total of those charges in non-negative cents.",
		"If a tip/gratuity is already applied and included in the charged amount, set tip_cents to that already-applied amount in non-negative cents.",
		"Do NOT treat suggested tip options (for example 18%/20%/22% recommendations) as tip_cents unless a specific option is explicitly selected/applied.",
		"Do not include bill-wide fees/charges as purchasable items.",
		"If a discount line cannot be linked to a specific item with reasonable confidence, do not invent a negative item; leave item discounts unchanged and add a warning about an unallocated discount.",
		"If clues conflict, choose the more conservative interpretation and add a warning.",
		"Perform a consistency check against subtotal/total when available; if mismatched, prefer warnings over speculative item creation.",
		"If uncertain between interpretations, prefer fewer assumptions and add a warning.",
		"Keep output compact and deterministic: do not emit explanatory prose anywhere; fill warnings/unparsed_lines only when necessary.",
		"Never emit duplicate warnings or duplicate unparsed_lines entries.",
		"Cap warnings at 12 short entries and unparsed_lines at 20 short entries; prefer null over speculative long text.",
		"For multi-line menu structures, treat modifier/add-on rows (e.g., 'no onion', 'extra cheese', side swaps) as addons on the nearest prior purchasable base item instead of separate items.",
		"If several add-ons belong to one base item, keep one parent item and attach each modifier in addons; do not duplicate addon text in the item name.",
		"If an add-on has a clear price, set addon.price_cents and include that amount in the parent line_price_cents gross total.",
		"If a row is clearly app-only/modifier text (for example sauce notes marked APP ONLY), attach it to the nearest prior purchasable base item instead of emitting a standalone base item.",
		"If a row names a beverage (for example soda/beer/wine/coffee), keep it as a standalone base item and never fold it into an adjacent food item's addons.",
		"Do not let app-only modifier rows suppress nearby standalone beverage rows with explicit prices.",
	}, " ")
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "You are a receipt parser. Return ONLY valid JSON that matches the schema. Do not include markdown. IMPORTANT: all prices must be integers in cents (e.g., $5.99 -> 599). Detect quantities from markers like 'x', 'qty', leading numbers, and do NOT merge identical items—list each line separately OR set quantity accordingly. If items repeat as separate lines, set quantity to the count. Keep line_price_cents as the gross line amount before discounts; discount_cents is per-unit; bill_discount_cents is whole-receipt discount amount (non-negative, applied once); bill_charges_cents is non-tip bill-wide fee/charge total (non-negative, applied once); tip_cents is only for already-applied tip/gratuity, never suggested tip options. Use the addons array to represent item modifiers (e.g., '+ chicken') and avoid emitting those as standalone items when clearly attached. Use valid JSON escaping only (never emit invalid escapes like \\%). If you are uncertain, set the field to null and add a warning. " + accuracyGuidance,
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Parse this receipt and return JSON with the schema: " + schema + " Use best-effort extraction for prices and discounts; do not leave prices null if a number is present. Currency: set `currency` to an ISO 4217 code. If the receipt does not explicitly show a currency symbol/code, infer the most likely country/locale from context clues (address, language, phone numbers, tax labels, merchant name, etc.) and choose the corresponding currency. Only use one of these supported codes: " + supported + ". If you cannot infer confidently OR the inferred currency is not in the supported list, set currency to null and add a warning. Do not wrap the JSON in markdown or code fences. Apply this extraction policy strictly: " + accuracyGuidance,
					},
					{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:" + contentType + ";base64," + encoded,
						},
					},
				},
			},
		},
		"temperature": 0.0,
		"max_tokens":  1500,
	}
	return json.Marshal(body)
}

func buildGeminiRequest(image []byte, contentType, model string, temperature float64) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(image)
	supported := strings.Join(supportedCurrencyCodes(), ", ")
	systemPrompt, userPrompt := geminiReceiptPrompts(model, supported)
	parseTemperature := temperature
	if parseTemperature < 0 {
		parseTemperature = 0
	} else if parseTemperature > 2 {
		parseTemperature = 2
	}
	generationConfig := map[string]any{
		"temperature":        parseTemperature,
		"max_output_tokens":  geminiReceiptMaxOutputTokensForModel(model),
		"response_mime_type": "application/json",
	}
	if isGeminiRetryModel(model) {
		generationConfig["response_schema"] = geminiReceiptResponseSchema()
		generationConfig["thinkingConfig"] = map[string]any{
			"thinkingLevel": "LOW",
		}
	} else {
		generationConfig["thinkingConfig"] = map[string]any{
			"thinkingBudget": 0,
		}
	}
	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]any{
				{
					"text": systemPrompt,
				},
			},
		},
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"text": userPrompt,
					},
					{
						"inline_data": map[string]any{
							"mime_type": contentType,
							"data":      encoded,
						},
					},
				},
			},
		},
		"generation_config": generationConfig,
	}
	return json.Marshal(body)
}

func geminiReceiptPrompts(model, supported string) (string, string) {
	baseRules := []string{
		"You are a receipt parser.",
		"Return ONLY valid JSON that matches the response schema. Do not include markdown, code fences, or explanatory text.",
		"All prices must be integers in cents (for example $5.99 -> 599).",
		"Detect quantity from explicit markers like leading numbers, x2, qty, or multipliers.",
		"Keep line_price_cents as the row's gross line amount before discounts.",
		"bill_discount_cents is a whole-receipt discount amount in non-negative cents.",
		"bill_charges_cents is a non-tip bill-wide fee/charge total in non-negative cents.",
		"tip_cents is only for already-applied tip/gratuity, never suggested tip options.",
		"Always set raw_text on each emitted item to the exact source row(s) used.",
		"Do not leave raw_text null when a visible item row can be read.",
		"Do not leave line_price_cents null when a visible row amount is present.",
		"Set currency to a supported ISO 4217 code only when explicit or strongly implied by context. Supported codes: " + supported + ".",
	}
	standardRules := []string{
		"First identify the receipt body and mentally crop away table/background clutter.",
		"If the receipt is rotated, internally orient it before reading.",
		"For simple restaurant receipts, each priced row before subtotal/tax/total is usually one standalone item.",
		"Use the price printed on the same row as the item. Do not shift a price to the previous or next item unless there is a clear price-only continuation row immediately attached.",
		"Preserve visible row order.",
		"Attach addons only when a row is clearly subordinate to the prior base item rather than a normal dish or beverage row.",
		"Ignore payment metadata, approval/device IDs, loyalty text, and suggested tip tables.",
		"If subtotal/tax/total are present, do a quick arithmetic check, but prefer same-row item-price alignment over speculative reassignment.",
		"If uncertain, make the fewest assumptions and add a short warning.",
	}
	retryRules := []string{
		"Work slowly and prioritize correctness over recall.",
		"Use two internal passes: first orient/crop and find the purchased-item section, then extract rows and reconcile arithmetic.",
		"Evaluate 0/90/180/270 rotations internally and use the orientation with the most coherent item-price rows.",
		"For photographed long receipts on dark backgrounds, ignore the surroundings and read only the paper receipt.",
		"Each priced pre-summary row is normally one standalone item. Preserve visible order and do not stop after the first expensive line.",
		"Use the rightmost monetary value on the same purchased row as that row's line total unless a clearer adjacent continuation line shows otherwise.",
		"Do not shift prices to neighboring rows just to make subtotal math work; reread the row instead.",
		"When an item name and amount are split across adjacent lines, merge them into one item.",
		"Treat headers like Guest, Seat, Ordered, Table, timestamp, payment, and card-reader metadata as non-item context.",
		"Attach addons only when clearly subordinate. Rows that look like ordinary dishes, sides, or beverages remain standalone items.",
		"Do not treat suggested tip options as tip_cents unless a tip was explicitly applied.",
		"If extracted items do not reconcile with subtotal/tax/total, reread prices and quantities before finalizing.",
		"If uncertain, prefer fewer assumptions and add a short warning.",
	}
	rules := append([]string{}, baseRules...)
	if isGeminiRetryModel(model) {
		rules = append(rules, retryRules...)
	} else {
		rules = append(rules, standardRules...)
	}
	systemPrompt := strings.Join(rules, " ")
	userPrompt := strings.Join([]string{
		"Parse this receipt and return only raw JSON.",
		"Return one object with keys merchant, items, subtotal_cents, bill_discount_cents, bill_charges_cents, tax_cents, tip_cents, total_cents, currency, fees, warnings, confidence, and unparsed_lines.",
		"Each item should include name, quantity, unit_price_cents, line_price_cents, discount_cents, discount_percent, addons, and raw_text.",
		"Each addon should include name, price_cents, and raw_text.",
	}, " ")
	if isGeminiRetryModel(model) {
		userPrompt += " Match the response schema exactly."
	}
	return systemPrompt, userPrompt
}

func geminiReceiptMaxOutputTokensForModel(model string) int {
	if isGeminiRetryModel(model) {
		return geminiReceiptMaxOutputTokensRetry
	}
	return geminiReceiptMaxOutputTokensStandard
}

func isGeminiRetryModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), geminiModelRetryPrimary)
}

func geminiReceiptResponseSchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"merchant":            geminiNullableSchema("STRING"),
			"items":               geminiReceiptItemsSchema(),
			"subtotal_cents":      geminiNullableSchema("INTEGER"),
			"bill_discount_cents": geminiNullableSchema("INTEGER"),
			"bill_charges_cents":  geminiNullableSchema("INTEGER"),
			"tax_cents":           geminiNullableSchema("INTEGER"),
			"tip_cents":           geminiNullableSchema("INTEGER"),
			"total_cents":         geminiNullableSchema("INTEGER"),
			"currency":            geminiNullableSchema("STRING"),
			"fees":                geminiStringArraySchema(false),
			"warnings":            geminiStringArraySchema(false),
			"confidence": map[string]any{
				"type":     "NUMBER",
				"nullable": true,
				"minimum":  0,
				"maximum":  1,
			},
			"unparsed_lines": geminiStringArraySchema(false),
		},
		"required": []string{"items", "warnings"},
	}
}

func geminiReceiptItemsSchema() map[string]any {
	return map[string]any{
		"type": "ARRAY",
		"items": map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"name":             map[string]any{"type": "STRING"},
				"quantity":         geminiNullableSchema("NUMBER"),
				"unit_price_cents": geminiNullableSchema("INTEGER"),
				"line_price_cents": geminiNullableSchema("INTEGER"),
				"discount_cents":   geminiNullableSchema("INTEGER"),
				"discount_percent": geminiNullableSchema("NUMBER"),
				"addons": map[string]any{
					"type": "ARRAY",
					"items": map[string]any{
						"type": "OBJECT",
						"properties": map[string]any{
							"name":        map[string]any{"type": "STRING"},
							"price_cents": geminiNullableSchema("INTEGER"),
							"raw_text":    geminiNullableSchema("STRING"),
						},
						"required": []string{"name"},
					},
				},
				"raw_text": geminiNullableSchema("STRING"),
			},
			"required": []string{"name"},
		},
	}
}

func geminiNullableSchema(schemaType string) map[string]any {
	return map[string]any{
		"type":     schemaType,
		"nullable": true,
	}
}

func geminiStringArraySchema(nullable bool) map[string]any {
	return map[string]any{
		"type":     "ARRAY",
		"nullable": nullable,
		"items": map[string]any{
			"type": "STRING",
		},
	}
}

func decodeReceiptParseResult(content string) (*ReceiptParseResult, error) {
	type receiptParseResultDecode struct {
		Merchant          json.RawMessage `json:"merchant"`
		Items             []ReceiptItem   `json:"items"`
		SubtotalCents     *int            `json:"subtotal_cents"`
		BillDiscountCents *int            `json:"bill_discount_cents"`
		BillChargesCents  *int            `json:"bill_charges_cents"`
		TaxCents          *int            `json:"tax_cents"`
		TipCents          *int            `json:"tip_cents"`
		TotalCents        *int            `json:"total_cents"`
		Currency          string          `json:"currency"`
		Fees              []string        `json:"fees"`
		Warnings          []string        `json:"warnings"`
		Confidence        float64         `json:"confidence"`
		UnparsedLines     []string        `json:"unparsed_lines"`
	}
	var decoded receiptParseResultDecode
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return nil, err
	}
	return &ReceiptParseResult{
		Merchant:          decodeMerchantField(decoded.Merchant),
		Items:             decoded.Items,
		SubtotalCents:     decoded.SubtotalCents,
		BillDiscountCents: decoded.BillDiscountCents,
		BillChargesCents:  decoded.BillChargesCents,
		TaxCents:          decoded.TaxCents,
		TipCents:          decoded.TipCents,
		TotalCents:        decoded.TotalCents,
		Currency:          decoded.Currency,
		Fees:              decoded.Fees,
		Warnings:          decoded.Warnings,
		Confidence:        decoded.Confidence,
		UnparsedLines:     decoded.UnparsedLines,
	}, nil
}

func decodeMerchantField(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	for _, key := range []string{"name", "merchant", "title"} {
		if value, ok := object[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	for _, value := range object {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func cleanModelJSON(raw string) string {
	content := strings.TrimSpace(raw)
	if strings.HasPrefix(content, "```") {
		// strip code fences like ```json ... ```
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		if idx := strings.LastIndex(content, "```"); idx >= 0 {
			content = content[:idx]
		}
		content = strings.TrimSpace(content)
	}
	if extracted, ok := extractFirstJSONObject(content); ok {
		return sanitizeInvalidJSONEscapes(removeDanglingCommas(extracted))
	}
	if repaired, ok := repairTruncatedJSONObject(content); ok {
		return sanitizeInvalidJSONEscapes(removeDanglingCommas(repaired))
	}
	return sanitizeInvalidJSONEscapes(removeDanglingCommas(content))
}

// sanitizeInvalidJSONEscapes removes invalid escape sequences inside JSON strings.
// Some LLMs occasionally emit things like "\\%" (i.e. "\%") which is not valid JSON.
// We only touch escapes while "in string" to avoid changing structure.
func sanitizeInvalidJSONEscapes(s string) string {
	// Fast-path: no backslash, nothing to sanitize.
	if !strings.Contains(s, `\`) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		ch := s[i]

		if !inString {
			if ch == '"' {
				inString = true
			}
			b.WriteByte(ch)
			continue
		}

		// inString
		if escape {
			escape = false
			switch ch {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				// valid one-char escape
				b.WriteByte(ch)
				continue
			case 'u':
				// Keep the unicode escape prefix. The following 4 chars will be copied as-is.
				b.WriteByte(ch)
				continue
			default:
				// Invalid escape (e.g. \%). Drop the backslash and keep the character.
				// Note: the backslash has already been written to the builder, so we need
				// to remove it. Since strings.Builder can't "unwrite", we avoid writing
				// the backslash in the first place (handled below).
				//
				// This branch is kept for completeness but should be unreachable.
				b.WriteByte(ch)
				continue
			}
		}

		if ch == '\\' {
			// Peek the next byte to decide if this is a valid escape.
			if i+1 >= len(s) {
				// Trailing backslash inside a string would break JSON; drop it.
				continue
			}
			next := s[i+1]
			valid := next == '"' || next == '\\' || next == '/' ||
				next == 'b' || next == 'f' || next == 'n' || next == 'r' || next == 't' || next == 'u'
			if !valid {
				// Skip the backslash; keep the next character as literal.
				continue
			}
			escape = true
			b.WriteByte(ch)
			continue
		}
		if ch == '"' {
			inString = false
		}
		b.WriteByte(ch)
	}

	return b.String()
}

func firstGeminiText(candidates []geminiCandidate) string {
	best := ""
	for _, candidate := range candidates {
		parts := make([]string, 0, len(candidate.Content.Parts))
		for _, part := range candidate.Content.Parts {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		joined := strings.TrimSpace(strings.Join(parts, "\n"))
		if len(joined) > len(best) {
			best = joined
		}
	}
	return best
}

// extractFirstJSONObject finds the first complete JSON object in the string.
func extractFirstJSONObject(content string) (string, bool) {
	start := strings.Index(content, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1], true
			}
		}
	}
	return "", false
}

func isJSONWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}

// removeDanglingCommas removes commas that are immediately followed by a closing
// '}' or ']' (ignoring whitespace) while outside string literals.
func removeDanglingCommas(s string) string {
	if !strings.Contains(s, ",") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			b.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			b.WriteByte(ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(s) && isJSONWhitespace(s[j]) {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				continue
			}
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// repairTruncatedJSONObject tries to repair truncated JSON by closing open
// objects/arrays and unterminated strings.
func repairTruncatedJSONObject(content string) (string, bool) {
	start := strings.Index(content, "{")
	if start < 0 {
		return "", false
	}
	jsonFragment := strings.TrimSpace(content[start:])
	if jsonFragment == "" {
		return "", false
	}
	stack := make([]byte, 0, 32)
	inString := false
	escaped := false
	for i := 0; i < len(jsonFragment); i++ {
		ch := jsonFragment[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '{')
		case '[':
			stack = append(stack, '[')
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		}
	}

	repaired := jsonFragment
	if inString {
		if escaped && strings.HasSuffix(repaired, `\`) {
			repaired = strings.TrimSuffix(repaired, `\`)
		}
		repaired += `"`
	}

	if len(stack) > 0 {
		var suffix strings.Builder
		suffix.Grow(len(stack))
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i] == '{' {
				suffix.WriteByte('}')
			} else {
				suffix.WriteByte(']')
			}
		}
		repaired += suffix.String()
	}

	repaired = removeDanglingCommas(repaired)
	if extracted, ok := extractFirstJSONObject(repaired); ok {
		return extracted, true
	}
	return repaired, strings.HasPrefix(strings.TrimSpace(repaired), "{")
}
