package server

import (
	"sort"
	"strings"
)

type CurrencyInfo struct {
	Code     string
	Symbol   string
	Exponent int
	Flag     string
}

var currencyMeta = map[string]CurrencyInfo{
	"USD": {Code: "USD", Symbol: "$", Exponent: 2, Flag: "🇺🇸"},
	"EUR": {Code: "EUR", Symbol: "€", Exponent: 2, Flag: "🇪🇺"},
	"GBP": {Code: "GBP", Symbol: "£", Exponent: 2, Flag: "🇬🇧"},
	"JPY": {Code: "JPY", Symbol: "¥", Exponent: 0, Flag: "🇯🇵"},
	"CAD": {Code: "CAD", Symbol: "$", Exponent: 2, Flag: "🇨🇦"},
	"AUD": {Code: "AUD", Symbol: "$", Exponent: 2, Flag: "🇦🇺"},
	"CHF": {Code: "CHF", Symbol: "Fr", Exponent: 2, Flag: "🇨🇭"},
	"CNY": {Code: "CNY", Symbol: "¥", Exponent: 2, Flag: "🇨🇳"},
	"KRW": {Code: "KRW", Symbol: "₩", Exponent: 0, Flag: "🇰🇷"},
	"MXN": {Code: "MXN", Symbol: "$", Exponent: 2, Flag: "🇲🇽"},
	"SGD": {Code: "SGD", Symbol: "$", Exponent: 2, Flag: "🇸🇬"},
	"HKD": {Code: "HKD", Symbol: "$", Exponent: 2, Flag: "🇭🇰"},
	"INR": {Code: "INR", Symbol: "₹", Exponent: 2, Flag: "🇮🇳"},
	"SEK": {Code: "SEK", Symbol: "kr", Exponent: 2, Flag: "🇸🇪"},
	"NOK": {Code: "NOK", Symbol: "kr", Exponent: 2, Flag: "🇳🇴"},
}

func normalizeCurrencyCode(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	if c == "" {
		return ""
	}
	if _, ok := currencyMeta[c]; ok {
		return c
	}
	return ""
}

func supportedCurrencyCodes() []string {
	out := make([]string, 0, len(currencyMeta))
	for k := range currencyMeta {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func currencyExponent(code string) int {
	if info, ok := currencyMeta[code]; ok {
		return info.Exponent
	}
	return 2
}
