// Package config loads runtime configuration from the environment.
//
// Credentials (DATABASE_URL, JWT_SECRET) have no defaults in code: they come
// from the environment, which docker compose and LoadDotEnv fill from the
// git-ignored .env file (see .env.example).
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/url"
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

// Minimum lengths for secrets in production.
const (
	minSecretLength     = 32
	minDBPasswordLength = 16
)

// Load reads configuration from the environment.
func Load() Config {
	cfg := Config{
		Env:                env("APP_ENV", "development"),
		Port:               env("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
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

// Validate rejects missing settings, and settings that are unsafe in production.
func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is not set: copy .env.example to .env and fill it in")
	}
	if !c.Production() {
		return nil
	}
	var problems []string
	if len(c.JWTSecret) < minSecretLength {
		problems = append(problems, "JWT_SECRET must be a random value of at least 32 characters")
	}
	if c.CORSAllowedOrigins == "" || strings.Contains(c.CORSAllowedOrigins, "*") {
		problems = append(problems, "CORS_ALLOWED_ORIGINS must list explicit origins")
	}
	if parsed, err := url.Parse(c.DatabaseURL); err != nil {
		problems = append(problems, "DATABASE_URL is not a valid URL")
	} else if password, _ := parsed.User.Password(); len(password) < minDBPasswordLength {
		problems = append(problems, "the DATABASE_URL password must be at least 16 characters")
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
