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

type ReceiptParseResult struct {
	Merchant      string        `json:"merchant,omitempty"`
	Items         []ReceiptItem `json:"items"`
	SubtotalCents *int          `json:"subtotal_cents"`
	TaxCents      *int          `json:"tax_cents"`
	TotalCents    *int          `json:"total_cents"`
	Currency      string        `json:"currency,omitempty"`
	Fees          []string      `json:"fees,omitempty"`
	Warnings      []string      `json:"warnings"`
	Confidence    float64       `json:"confidence"`
	UnparsedLines []string      `json:"unparsed_lines,omitempty"`
}

type ReceiptItem struct {
	Name            string   `json:"name"`
	Quantity        *float64 `json:"quantity"`
	UnitPriceCents  *int     `json:"unit_price_cents"`
	LinePriceCents  *int     `json:"line_price_cents"`
	DiscountCents   *int     `json:"discount_cents"`
	DiscountPercent *float64 `json:"discount_percent"`
	RawText         *string  `json:"raw_text"`
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

func callGeminiReceiptParse(ctx context.Context, apiKey string, image []byte, contentType string) (*ReceiptParseResult, error) {
	payload, err := buildGeminiRequest(image, contentType)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

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
			return nil, fmt.Errorf("gemini error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini error: status %d", resp.StatusCode)
	}

	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	raw := firstGeminiText(response.Candidates)
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("no response from Gemini")
	}
	content := cleanModelJSON(raw)
	var result ReceiptParseResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, err
	}
	result.Currency = normalizeCurrencyCode(result.Currency)
	return &result, nil
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
      "discount_percent": "number or null"
    }
  ],
  "subtotal_cents": "int or null",
  "tax_cents": "int or null",
  "total_cents": "int or null",
  "currency": "string or null",
  "fees": "array of strings",
  "warnings": "array of strings",
  "confidence": "number between 0 and 1",
  "unparsed_lines": "array of strings"
}`
	supported := strings.Join(supportedCurrencyCodes(), ", ")
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "You are a receipt parser. Return ONLY valid JSON that matches the schema. Do not include markdown. IMPORTANT: all prices must be integers in cents (e.g., $5.99 -> 599). Detect quantities from markers like 'x', 'qty', leading numbers, and do NOT merge identical items—list each line separately OR set quantity accordingly. If items repeat as separate lines, set quantity to the count. Keep line_price_cents as the gross line amount before discounts; discount_cents is per-unit. If you are uncertain, set the field to null and add a warning.",
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Parse this receipt and return JSON with the schema: " + schema + " Use best-effort extraction for prices and discounts; do not leave prices null if a number is present. Currency: set `currency` to an ISO 4217 code. If the receipt does not explicitly show a currency symbol/code, infer the most likely country/locale from context clues (address, language, phone numbers, tax labels, merchant name, etc.) and choose the corresponding currency. Only use one of these supported codes: " + supported + ". If you cannot infer confidently OR the inferred currency is not in the supported list, set currency to null and add a warning. Do not wrap the JSON in markdown or code fences.",
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
		"temperature": 0.2,
		"max_tokens":  1500,
	}
	return json.Marshal(body)
}

func buildGeminiRequest(image []byte, contentType string) ([]byte, error) {
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
      "raw_text": "string or null"
    }
  ],
  "subtotal_cents": "int or null",
  "tax_cents": "int or null",
  "total_cents": "int or null",
  "currency": "string or null",
  "fees": "array of strings",
  "warnings": "array of strings",
  "confidence": "number between 0 and 1",
  "unparsed_lines": "array of strings"
}`
	supported := strings.Join(supportedCurrencyCodes(), ", ")
	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]any{
				{
					"text": "You are a receipt parser. Return ONLY valid JSON that matches the schema. Do not include markdown, code fences, or any extra text. IMPORTANT: all prices must be integers in cents (e.g., $5.99 -> 599). Detect quantities from markers like 'x', 'qty', leading numbers, and do NOT merge identical items—list each line separately OR set quantity accordingly. If items repeat as separate lines, set quantity to the count. Keep line_price_cents as the gross line amount before discounts; discount_cents is per-unit. Currency: set `currency` to an ISO 4217 code. If the receipt does not explicitly show a currency symbol/code, infer the most likely country/locale from context clues (address, language, phone numbers, tax labels, merchant name, etc.) and choose the corresponding currency. Only use one of these supported codes: " + supported + ". If you cannot infer confidently OR the inferred currency is not in the supported list, set currency to null and add a warning. To minimize output size, omit raw text for lines unless it is truly necessary.",
				},
			},
		},
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"text": "Parse this receipt and return ONLY raw JSON with the schema: " + schema + " Use best-effort extraction for prices and discounts; do not leave prices null if a number is present. Currency: set `currency` to an ISO 4217 code. If it is not explicitly shown, infer from context; only use supported codes (" + supported + "). Otherwise use null and add a warning. Do not wrap the JSON in markdown or code fences. Keep output concise.",
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
		"generation_config": map[string]any{
			"temperature":        0.2,
			"max_output_tokens":  3000,
			"response_mime_type": "application/json",
		},
	}
	return json.Marshal(body)
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
		return extracted
	}
	if repaired, ok := repairTruncatedJSONObject(content); ok {
		return repaired
	}
	return content
}

func firstGeminiText(candidates []struct {
	Content struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"content"`
}) string {
	for _, candidate := range candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		}
	}
	return ""
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

// repairTruncatedJSONObject tries to close a truncated JSON object by balancing braces.
func repairTruncatedJSONObject(content string) (string, bool) {
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
	if depth > 0 && !inString {
		return content[start:] + strings.Repeat("}", depth), true
	}
	return "", false
}
