package server

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	RedisURL           string
	SessionSecret      string
	JoinTokenKey       string
	CorsAllowedOrigins []string
	RoomTTL            time.Duration
	CookieSecure       bool
	CookieDomain       string
	OpenAIKey          string
	PublicBaseURL      string
}

func LoadConfig() Config {
	return Config{
		Port:               getenv("BACKEND_PORT", "8080"),
		RedisURL:           getenv("REDIS_URL", "redis://redis:6379/0"),
		SessionSecret:      os.Getenv("SESSION_SECRET"),
		JoinTokenKey:       os.Getenv("JOIN_TOKEN_SIGNING_KEY"),
		CorsAllowedOrigins: splitCSV(os.Getenv("CORS_ALLOWED_ORIGINS")),
		RoomTTL:            time.Duration(getenvInt("ROOM_TTL_SECONDS", 14400)) * time.Second,
		CookieSecure:       getenvBool("COOKIE_SECURE", true),
		CookieDomain:       os.Getenv("COOKIE_DOMAIN"),
		OpenAIKey:          os.Getenv("OPENAI_API_KEY"),
		PublicBaseURL:      getenv("PUBLIC_BASE_URL", "https://localhost"),
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
