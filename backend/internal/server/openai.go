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
	"time"
)

type ReceiptParseResult struct {
	Merchant      string           `json:"merchant,omitempty"`
	Items         []ReceiptItem    `json:"items"`
	SubtotalCents *int             `json:"subtotal_cents"`
	TaxCents      *int             `json:"tax_cents"`
	TotalCents    *int             `json:"total_cents"`
	Fees          []string         `json:"fees,omitempty"`
	Warnings      []string         `json:"warnings"`
	Confidence    float64          `json:"confidence"`
	UnparsedLines []string         `json:"unparsed_lines,omitempty"`
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
      "discount_percent": "number or null",
      "raw_text": "string or null"
    }
  ],
  "subtotal_cents": "int or null",
  "tax_cents": "int or null",
  "total_cents": "int or null",
  "fees": "array of strings",
  "warnings": "array of strings",
  "confidence": "number between 0 and 1",
  "unparsed_lines": "array of strings"
}`
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]any{
			{
				"role": "system",
				"content": "You are a receipt parser. Return ONLY valid JSON that matches the schema. Do not include markdown. IMPORTANT: all prices must be integers in cents (e.g., $5.99 -> 599). Detect quantities from markers like 'x', 'qty', leading numbers, and do NOT merge identical items—list each line separately OR set quantity accordingly. If items repeat as separate lines, set quantity to the count. Keep line_price_cents as the gross line amount before discounts; discount_cents is per-unit. If you are uncertain, set the field to null and add a warning.",
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Parse this receipt and return JSON with the schema: " + schema + " Use best-effort extraction for prices and discounts; do not leave prices null if a number is present.",
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
		"max_tokens": 1500,
	}
	return json.Marshal(body)
}
