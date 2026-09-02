package gwconfig

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alternayte/restitch-gateway/internal/admin"
)

// Duration wraps time.Duration for YAML unmarshaling from Go duration strings.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// File is the root configuration loaded from restitch.yaml.
type File struct {
	Server       ServerConfig           `yaml:"server"`
	Admin        AdminConfig            `yaml:"admin"`
	Upstreams    map[string]interface{} `yaml:"upstreams"`
	Compositions map[string]interface{} `yaml:"compositions"`

	raw []byte // original bytes for hash
}

// RateLimitConfig configures gateway-wide rate limiting.
type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
	Key               string  `yaml:"key"` // "ip", "header:X-Client-ID", "api-key"
}

// ServerConfig holds gateway server settings.
type ServerConfig struct {
	Port            int                `yaml:"port"`
	TLSPort         int                `yaml:"tls_port"`
	TLSCert         string             `yaml:"tls_cert"`
	TLSKey          string             `yaml:"tls_key"`
	ReadTimeout     Duration           `yaml:"read_timeout"`
	WriteTimeout    Duration           `yaml:"write_timeout"`
	ShutdownTimeout Duration           `yaml:"shutdown_timeout"`
	LogFormat       string             `yaml:"log_format"`
	LogLevel        string             `yaml:"log_level"`
	Auth            *InboundAuthConfig `yaml:"auth"`
	RateLimit       *RateLimitConfig   `yaml:"rate_limit"`
}

// InboundAuthConfig configures gateway-level authentication.
type InboundAuthConfig struct {
	APIKeys []string   `yaml:"api_keys"`
	JWT     *JWTConfig `yaml:"jwt"`
}

// JWTConfig configures JWT validation via JWKS.
type JWTConfig struct {
	JWKSURL  string `yaml:"jwks_url"`
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
}

// AdminConfig holds admin API server settings.
type AdminConfig struct {
	Enabled        *bool               `yaml:"enabled"`
	Port           int                 `yaml:"port"`
	Bind           string              `yaml:"bind"`
	APIKey         string              `yaml:"api_key"`
	RequestLogSize int                 `yaml:"request_log_size"`
	Storage        admin.StorageConfig `yaml:"storage"`
}

// IsEnabled returns whether the admin server is enabled (default true).
func (a AdminConfig) IsEnabled() bool {
	if a.Enabled == nil {
		return true
	}
	return *a.Enabled
}

// ReadAndExpand reads a config file and performs whole-file env expansion.
// Returns the expanded bytes (suitable for passing to ParseConfig and LoadBytes)
// and the raw bytes (for hashing).
func ReadAndExpand(path string) (expanded []byte, raw []byte, err error) {
	raw, err = os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading config: %w", err)
	}

	exp, err := ExpandEnvStrict(string(raw))
	if err != nil {
		return nil, raw, fmt.Errorf("expanding env vars: %w", err)
	}

	return []byte(exp), raw, nil
}

// Load reads a config file, expands env vars, unmarshals, applies defaults, and validates.
func Load(path string) (*File, error) {
	expanded, raw, err := ReadAndExpand(path)
	if err != nil {
		return nil, err
	}
	return LoadBytes(expanded, raw)
}

// LoadBytes parses already-expanded YAML bytes into a File.
// raw is the original (unexpanded) bytes used for hashing.
func LoadBytes(expanded []byte, raw []byte) (*File, error) {
	var f File
	if err := yaml.Unmarshal(expanded, &f); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	f.raw = raw

	applyDefaults(&f)

	if err := f.Validate(); err != nil {
		return nil, err
	}

	return &f, nil
}

func applyDefaults(f *File) {
	if f.Server.Port == 0 {
		f.Server.Port = 8080
	}
	if f.Server.LogFormat == "" {
		f.Server.LogFormat = "json"
	}
	if f.Server.LogLevel == "" {
		f.Server.LogLevel = "info"
	}
	if f.Server.ReadTimeout.Duration == 0 {
		f.Server.ReadTimeout.Duration = 10 * time.Second
	}
	if f.Server.WriteTimeout.Duration == 0 {
		f.Server.WriteTimeout.Duration = 30 * time.Second
	}
	if f.Server.ShutdownTimeout.Duration == 0 {
		f.Server.ShutdownTimeout.Duration = 30 * time.Second
	}
	if f.Admin.Port == 0 {
		f.Admin.Port = 9090
	}
	if f.Admin.Bind == "" {
		f.Admin.Bind = "127.0.0.1"
	}
	if f.Admin.RequestLogSize == 0 {
		f.Admin.RequestLogSize = 500
	}
}

// Validate checks the config for errors. Returns joined errors for all problems.
func (f *File) Validate() error {
	var errs []error

	if f.Server.Port < 1 || f.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port must be 1-65535, got %d", f.Server.Port))
	}
	if f.Server.TLSPort != 0 && (f.Server.TLSPort < 1 || f.Server.TLSPort > 65535) {
		errs = append(errs, fmt.Errorf("server.tls_port must be 1-65535, got %d", f.Server.TLSPort))
	}
	if f.Admin.Port < 1 || f.Admin.Port > 65535 {
		errs = append(errs, fmt.Errorf("admin.port must be 1-65535, got %d", f.Admin.Port))
	}

	logFormat := strings.ToLower(f.Server.LogFormat)
	if logFormat != "json" && logFormat != "text" {
		errs = append(errs, fmt.Errorf("server.log_format must be json or text, got %q", f.Server.LogFormat))
	}
	logLevel := strings.ToLower(f.Server.LogLevel)
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		errs = append(errs, fmt.Errorf("server.log_level must be debug/info/warn/error, got %q", f.Server.LogLevel))
	}

	if f.Server.Auth != nil && f.Server.Auth.JWT != nil {
		if f.Server.Auth.JWT.JWKSURL == "" {
			errs = append(errs, fmt.Errorf("server.auth.jwt.jwks_url is required when jwt is configured"))
		}
	}

	switch f.Admin.Storage.Type {
	case "", "memory":
		// valid, no additional fields required
	case "sqlite":
		if f.Admin.Storage.URL == "" {
			errs = append(errs, fmt.Errorf("admin.storage.url is required when type is %q", f.Admin.Storage.Type))
		}
	case "turso":
		if f.Admin.Storage.URL == "" {
			errs = append(errs, fmt.Errorf("admin.storage.url is required when type is %q", f.Admin.Storage.Type))
		}
	default:
		errs = append(errs, fmt.Errorf("admin.storage.type must be memory, sqlite, or turso, got %q", f.Admin.Storage.Type))
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// Hash returns the SHA-256 hex digest of the original config file bytes.
func (f *File) Hash() string {
	h := sha256.Sum256(f.raw)
	return fmt.Sprintf("%x", h)
}

// ApplyEnvOverrides applies RESTITCH_* environment variables per §2.5.
func ApplyEnvOverrides(f *File) {
	if v := os.Getenv("RESTITCH_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Server.Port = n
		}
	}
	if v := os.Getenv("RESTITCH_TLS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Server.TLSPort = n
		}
	}
	if v := os.Getenv("RESTITCH_TLS_CERT"); v != "" {
		f.Server.TLSCert = v
	}
	if v := os.Getenv("RESTITCH_TLS_KEY"); v != "" {
		f.Server.TLSKey = v
	}
	if v := os.Getenv("RESTITCH_LOG_FORMAT"); v != "" {
		f.Server.LogFormat = v
	}
	if v := os.Getenv("RESTITCH_LOG_LEVEL"); v != "" {
		f.Server.LogLevel = v
	}
	if v := os.Getenv("RESTITCH_ADMIN_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Admin.Port = n
		}
	}
	if v := os.Getenv("RESTITCH_ADMIN_BIND"); v != "" {
		f.Admin.Bind = v
	}
	if v := os.Getenv("RESTITCH_ADMIN_ENABLED"); v != "" {
		b := strings.ToLower(v) == "true" || v == "1"
		f.Admin.Enabled = &b
	}
	if v := os.Getenv("RESTITCH_ADMIN_API_KEY"); v != "" {
		f.Admin.APIKey = v
	}
}
