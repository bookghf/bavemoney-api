// Package config loads runtime configuration from the environment.
package config

import "os"

// Config holds every setting the API needs to boot.
type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
}

// Load reads configuration from the environment, falling back to
// development-friendly defaults.
func Load() Config {
	return Config{
		Port:        env("PORT", "8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://ledger:<db-password>@localhost:5432/ledger?sslmode=disable"),
		JWTSecret:   env("JWT_SECRET", "<dev-jwt-secret>"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
