package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// Config holds all runtime configuration for the portal.
type Config struct {
	Port          string // HTTP listen port, default "3000"
	DBPath        string // SQLite file path, default "data/portal.db"
	EncryptionKey string // 32-byte hex key for AES-256-GCM field encryption
	SessionSecret string // secret used to sign/verify session tokens
	AppEnv        string // "development" | "production"
	BannedWords   string // comma-separated list of prohibited comment words
	Timezone      string // IANA timezone for DND hour evaluation, default "UTC"
}

// Load reads configuration from environment variables, applying defaults where
// a variable is absent or empty.
func Load() *Config {
	return &Config{
		Port:          envOrDefault("PORT", "3000"),
		DBPath:        envOrDefault("DB_PATH", "data/portal.db"),
		EncryptionKey: envOrDefault("ENCRYPTION_KEY", ""),
		SessionSecret: envOrDefault("SESSION_SECRET", ""),
		AppEnv:        envOrDefault("APP_ENV", "development"),
		BannedWords:   envOrDefault("BANNED_WORDS", ""),
		Timezone:      envOrDefault("TIMEZONE", "UTC"),
	}
}

// Validate returns an error if any required secret configuration is absent or
// malformed.  Call this at startup so the application fails fast rather than
// running with an invalid encryption key or session secret.
//
// ENCRYPTION_KEY must be a 64-character lower-case hex string (32 raw bytes).
// Any other value — including empty — is rejected; there is no plaintext
// fallback once the system has started.
func (c *Config) Validate() error {
	var errs []error
	if c.EncryptionKey == "" {
		errs = append(errs, errors.New("ENCRYPTION_KEY must be set (64 hex chars / 32 bytes)"))
	} else {
		raw, err := hex.DecodeString(c.EncryptionKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("ENCRYPTION_KEY is not valid hex: %w", err))
		} else if len(raw) != 32 {
			errs = append(errs, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes (got %d); supply a 64-char hex string", len(raw)))
		}
	}
	if c.SessionSecret == "" {
		errs = append(errs, errors.New("SESSION_SECRET must be set"))
	}
	return errors.Join(errs...)
}

// envOrDefault returns the value of the named environment variable, or
// fallback when the variable is unset or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
