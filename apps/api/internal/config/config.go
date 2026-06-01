// Package config loads and validates process configuration from the environment (12-factor).
// It is imported only by internal/app (the composition root); adapters receive plain values, not
// this struct. See docs/architecture/go-backend.md §11.2.
package config

import (
	"fmt"
	"log/slog"

	"github.com/caarlos0/env/v11"
)

// Secret is a string whose value is redacted in logs and stringification. Use string(s) to read the
// real value where genuinely needed (DSNs, keys). slog never prints it (LogValue) and neither does
// fmt (String) — enforcing "no secrets in logs" at the type boundary.
type Secret string

func (Secret) String() string       { return "[REDACTED]" }
func (Secret) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }
func (s Secret) Reveal() string     { return string(s) }

// Config is the validated configuration for a Pixela process. Required fields fail fast at startup.
type Config struct {
	Env      string `env:"PIXELA_ENV" envDefault:"development"`
	HTTPPort int    `env:"PORT" envDefault:"3000"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	// Allowed CORS origin for the dashboard.
	CORSOrigin string `env:"CORS_ORIGIN" envDefault:"http://localhost:4200"`

	// Postgres (data + River queue).
	DatabaseURL Secret `env:"DATABASE_URL,required,notEmpty"`

	// Redis — dashboard sessions only (NOT the queue).
	RedisURL Secret `env:"REDIS_URL,required,notEmpty"`

	// Object storage (S3-compatible / MinIO) — content-addressable blobs.
	S3Endpoint       string `env:"S3_ENDPOINT,required,notEmpty"`
	S3Region         string `env:"S3_REGION" envDefault:"us-east-1"`
	S3Bucket         string `env:"S3_BUCKET" envDefault:"pixela"`
	S3AccessKey      Secret `env:"S3_ACCESS_KEY,required,notEmpty"`
	S3SecretKey      Secret `env:"S3_SECRET_KEY,required,notEmpty"`
	S3ForcePathStyle bool   `env:"S3_FORCE_PATH_STYLE" envDefault:"true"`
	S3UseSSL         bool   `env:"S3_USE_SSL" envDefault:"false"`

	// Dashboard session signing secret (server-side sessions in Redis).
	SessionSecret Secret `env:"SESSION_SECRET,required,notEmpty"`

	// GitLab integration (optional in Phase 0).
	GitLabBaseURL string `env:"GITLAB_BASE_URL" envDefault:"https://gitlab.com"`
	GitLabToken   Secret `env:"GITLAB_TOKEN"`
}

// Load parses and validates configuration from the environment.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// IsProduction reports whether the process runs in a production environment.
func (c Config) IsProduction() bool { return c.Env == "production" }
