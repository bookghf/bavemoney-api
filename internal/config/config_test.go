package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test values are generated, not literals, so nothing here looks like a
// credential to a secret scanner.
var (
	strongSecret   = strings.Repeat("a", minSecretLength)
	strongPassword = strings.Repeat("p", minDBPasswordLength)
)

func TestValidateProduction(t *testing.T) {
	good := Config{
		Env:                "production",
		JWTSecret:          strongSecret,
		CORSAllowedOrigins: "https://app.example.com",
		DatabaseURL:        "postgres://ledger:" + strongPassword + "@db/ledger",
		ResendAPIKey:       strongSecret,
		MailFrom:           "Ledger <no-reply@example.com>",
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("good config rejected: %v", err)
	}

	for name, edit := range map[string]func(*Config){
		"short secret":   func(c *Config) { c.JWTSecret = "short" },
		"wildcard cors":  func(c *Config) { c.CORSAllowedOrigins = "*" },
		"short password": func(c *Config) { c.DatabaseURL = "postgres://ledger:short@db/ledger" },
		"no database":    func(c *Config) { c.DatabaseURL = "" },
		"no mail key":    func(c *Config) { c.ResendAPIKey = "" },
		"no mail sender": func(c *Config) { c.MailFrom = "" },
	} {
		cfg := good
		edit(&cfg)
		if cfg.Validate() == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestValidateDevelopmentStillNeedsADatabase(t *testing.T) {
	if (Config{Env: "development"}).Validate() == nil {
		t.Error("missing DATABASE_URL must be reported")
	}
	if (Config{Env: "development", DatabaseURL: "postgres://localhost/ledger"}).Validate() != nil {
		t.Error("development config with a database must pass")
	}
}

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\nDOTENV_TEST_A=from-file\nexport DOTENV_TEST_B=\"quoted value\"\nDOTENV_TEST_C=from-file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_C", "from-env")
	for _, key := range []string{"DOTENV_TEST_A", "DOTENV_TEST_B"} {
		t.Setenv(key, "") // registers cleanup
		os.Unsetenv(key)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_TEST_A"); got != "from-file" {
		t.Errorf("A = %q", got)
	}
	if got := os.Getenv("DOTENV_TEST_B"); got != "quoted value" {
		t.Errorf("B = %q", got)
	}
	if got := os.Getenv("DOTENV_TEST_C"); got != "from-env" {
		t.Errorf("C = %q, the existing environment must win", got)
	}
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("missing file: %v", err)
	}
}
