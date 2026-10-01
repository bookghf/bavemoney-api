// Package config loads runtime configuration from the environment.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"strings"
)

// Config holds every setting the API needs to boot.
type Config struct {
	// Env is "development" (default) or "production". Production refuses to
	// start with development-only settings.
	Env         string
	Port        string
	DatabaseURL string
	JWTSecret   string
	// CORSAllowedOrigins is a comma-separated list of browser origins allowed
	// to call the API; "*" allows any origin (development only).
	CORSAllowedOrigins string
}

// minSecretLength is the shortest JWT secret accepted in production.
const minSecretLength = 32

// knownWeakSecrets are placeholder values that must never sign real tokens.
var knownWeakSecrets = map[string]bool{
	"<jwt-secret>": true,
	"<dev-jwt-secret>":      true,
	"secret":                  true,
}

// Load reads configuration from the environment, falling back to
// development-friendly defaults.
func Load() Config {
	cfg := Config{
		Env:                env("APP_ENV", "development"),
		Port:               env("PORT", "8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://ledger:<db-password>@localhost:5432/ledger?sslmode=disable"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		CORSAllowedOrigins: os.Getenv("CORS_ALLOWED_ORIGINS"),
	}
	if !cfg.Production() {
		if cfg.CORSAllowedOrigins == "" {
			cfg.CORSAllowedOrigins = "*"
		}
		if cfg.JWTSecret == "" {
			// Sessions then last until the next restart; set JWT_SECRET to keep them.
			cfg.JWTSecret = randomSecret()
			log.Print("config: JWT_SECRET is not set; using a random secret for this run")
		}
	}
	return cfg
}

// Production reports whether the API runs with production safeguards.
func (c Config) Production() bool { return c.Env == "production" }

// Validate rejects settings that are unsafe in production.
func (c Config) Validate() error {
	if !c.Production() {
		return nil
	}
	var problems []string
	if len(c.JWTSecret) < minSecretLength || knownWeakSecrets[c.JWTSecret] {
		problems = append(problems, "JWT_SECRET must be a random value of at least 32 characters")
	}
	if c.CORSAllowedOrigins == "" || strings.Contains(c.CORSAllowedOrigins, "*") {
		problems = append(problems, "CORS_ALLOWED_ORIGINS must list explicit origins")
	}
	if strings.Contains(c.DatabaseURL, "<db-password>") {
		problems = append(problems, "DATABASE_URL uses the development password")
	}
	if len(problems) > 0 {
		return errors.New("unsafe production config: " + strings.Join(problems, "; "))
	}
	return nil
}

func randomSecret() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		log.Fatalf("config: generate secret: %v", err)
	}
	return hex.EncodeToString(raw)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
