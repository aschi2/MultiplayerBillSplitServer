package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type fxRates struct {
	Base  string             `json:"base"`
	Rates map[string]float64 `json:"rates"`
	Date  string             `json:"date"`
}

func (s *Server) handleFX(w http.ResponseWriter, r *http.Request) {
	base := strings.ToUpper(r.URL.Query().Get("base"))
	target := strings.ToUpper(r.URL.Query().Get("target"))
	if base == "" || target == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "base and target required"})
		return
	}
	rate, asOf, err := s.getRate(r.Context(), base, target)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"base":   base,
		"target": target,
		"rate":   rate,
		"as_of":  asOf,
	})
}

func (s *Server) getRate(ctx context.Context, base, target string) (float64, time.Time, error) {
	if strings.EqualFold(base, target) {
		return 1, time.Now(), nil
	}
	rates, asOf, err := s.getECBRates(ctx)
	if err != nil {
		return 0, time.Time{}, err
	}
	baseRate, ok := rates[strings.ToUpper(base)]
	if !ok || baseRate == 0 {
		return 0, time.Time{}, fmt.Errorf("unsupported base currency")
	}
	targetRate, ok := rates[strings.ToUpper(target)]
	if !ok || targetRate == 0 {
		return 0, time.Time{}, fmt.Errorf("unsupported target currency")
	}
	return targetRate / baseRate, asOf, nil
}

func (s *Server) getECBRates(ctx context.Context) (map[string]float64, time.Time, error) {
	cacheKey := "fx:ecb:latest"
	if cached, err := s.store.Client.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var payload struct {
			Rates map[string]float64 `json:"rates"`
			AsOf  int64              `json:"as_of"`
			Base  string             `json:"base"`
		}
		if json.Unmarshal([]byte(cached), &payload) == nil && payload.Rates != nil {
			return payload.Rates, time.Unix(payload.AsOf, 0), nil
		}
	}

	rates, base, err := fetchRatesWithFallback(ctx, []string{
		forceEURBase(s.config.ECBRatesURL),
		"https://api.frankfurter.app/latest?from=EUR",
	})
	if err != nil {
		return nil, time.Time{}, err
	}

	asOf := time.Now()
	payload := map[string]any{"rates": rates, "as_of": asOf.Unix(), "base": base}
	if encoded, err := json.Marshal(payload); err == nil {
		_ = s.store.Client.Set(ctx, cacheKey, encoded, 24*time.Hour).Err()
	}
	return rates, asOf, nil
}

func fetchRatesWithFallback(ctx context.Context, urls []string) (map[string]float64, string, error) {
	var lastErr error
	for _, candidate := range urls {
		rates, base, err := fetchRates(ctx, candidate)
		if err == nil {
			return rates, base, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("rates fetch failed")
	}
	return nil, "", lastErr
}

func fetchRates(ctx context.Context, rawURL string) (map[string]float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("rates fetch failed: status %d", resp.StatusCode)
	}

	// Handle common formats (exchangerate.host/frankfurter both include base+rates).
	var data fxRates
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, "", err
	}
	if len(data.Rates) == 0 {
		return nil, "", fmt.Errorf("rates fetch failed: empty rates")
	}

	base := strings.ToUpper(strings.TrimSpace(data.Base))
	if base == "" {
		base = "EUR"
	}
	data.Rates[base] = 1.0
	return data.Rates, base, nil
}

func forceEURBase(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := parsed.Query()
	if q.Get("base") == "" {
		q.Set("base", "EUR")
		parsed.RawQuery = q.Encode()
	}
	return parsed.String()
}
