package config

import "testing"

func TestValidateProduction(t *testing.T) {
	good := Config{
		Env:                "production",
		JWTSecret:          "<32-char-secret>",
		CORSAllowedOrigins: "https://app.example.com",
		DatabaseURL:        "postgres://ledger:<db-password>@db/ledger",
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("good config rejected: %v", err)
	}

	for name, edit := range map[string]func(*Config){
		"short secret":  func(c *Config) { c.JWTSecret = "short" },
		"weak secret":   func(c *Config) { c.JWTSecret = "<jwt-secret>" },
		"wildcard cors": func(c *Config) { c.CORSAllowedOrigins = "*" },
		"dev password":  func(c *Config) { c.DatabaseURL = "postgres://ledger:<db-password>@db/ledger" },
	} {
		cfg := good
		edit(&cfg)
		if cfg.Validate() == nil {
			t.Errorf("%s: want error", name)
		}
	}

	if (Config{Env: "development"}).Validate() != nil {
		t.Error("development config must not be rejected")
	}
}
