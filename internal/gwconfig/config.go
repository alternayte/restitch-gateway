// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/alternayte/restitch-gateway/v2/internal/admin"
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

// File holds the gateway process settings, which come from flags and env
// only, and the hash of the artifact file. composition.Config owns the
// artifact itself.
type File struct {
	Server ServerConfig
	Admin  AdminConfig

	raw []byte // original bytes for hash
}

// artifactKeys are the top-level keys an artifact may hold.
var artifactKeys = []string{"auth", "rate_limit", "block_private_upstreams", "upstreams", "compositions"}

// v1Moved names where a v1 server:/admin: key went. A key with an empty
// value has no successor and is removed in v2. Every other key maps to the
// flag of the same name, so its env var comes from EnvName.
var v1Moved = map[string]string{
	"server.auth":            "the top-level auth key",
	"server.rate_limit":      "the top-level rate_limit key",
	"admin.request_log_size": "",
	"admin.storage":          "",
}

// ServerConfig holds the gateway process settings. Flags and their
// RESTITCH_ env twins set them; the YAML file never does.
type ServerConfig struct {
	Port            int
	TLSPort         int
	TLSCert         string
	TLSKey          string
	ReadTimeout     Duration
	WriteTimeout    Duration
	ShutdownTimeout Duration
	LogFormat       string
	LogLevel        string
}

// AdminConfig holds admin API server settings, set from RESTITCH_ADMIN_*
// env vars only.
type AdminConfig struct {
	Enabled        *bool
	Port           int
	Bind           string
	APIKey         string
	RequestLogSize int
	Storage        admin.StorageConfig
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
	if err := CheckArtifactKeys(expanded); err != nil {
		return nil, err
	}
	f := File{raw: raw}

	applyDefaults(&f)

	if err := f.Validate(); err != nil {
		return nil, err
	}

	return &f, nil
}

// CheckArtifactKeys refuses the v1 server: and admin: blocks, naming where
// each key went, and any other key an artifact does not hold.
func CheckArtifactKeys(doc []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if len(root.Content) == 0 {
		return nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return fmt.Errorf("parsing config: the top level must be a mapping")
	}
	var errs []error
	for i := 0; i+1 < len(top.Content); i += 2 {
		key, val := top.Content[i].Value, top.Content[i+1]
		switch {
		case key == "server" || key == "admin":
			errs = append(errs, v1BlockErrors(key, val)...)
		case !contains(artifactKeys, key):
			errs = append(errs, fmt.Errorf("unknown top-level key %q (an artifact holds %s)",
				key, strings.Join(artifactKeys, ", ")))
		}
	}
	return errors.Join(errs...)
}

func v1BlockErrors(block string, val *yaml.Node) []error {
	if val.Kind != yaml.MappingNode || len(val.Content) == 0 {
		return []error{fmt.Errorf("%s: removed in v2; process settings are flags and RESTITCH_ env vars", block)}
	}
	var errs []error
	for i := 0; i+1 < len(val.Content); i += 2 {
		name := block + "." + val.Content[i].Value
		dest, known := v1Moved[name]
		switch {
		case known && dest == "":
			errs = append(errs, fmt.Errorf("%s removed in v2", name))
		case known:
			errs = append(errs, fmt.Errorf("%s moved to %s", name, dest))
		default:
			flagName := strings.ReplaceAll(val.Content[i].Value, "_", "-")
			if block == "admin" {
				flagName = "admin-" + flagName
			}
			errs = append(errs, fmt.Errorf("%s moved to %s", name, EnvName(flagName)))
		}
	}
	return errs
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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
		errs = append(errs, fmt.Errorf("RESTITCH_PORT must be 1-65535, got %d", f.Server.Port))
	}
	if f.Server.TLSPort != 0 && (f.Server.TLSPort < 1 || f.Server.TLSPort > 65535) {
		errs = append(errs, fmt.Errorf("RESTITCH_TLS_PORT must be 1-65535, got %d", f.Server.TLSPort))
	}
	if f.Admin.Port < 1 || f.Admin.Port > 65535 {
		errs = append(errs, fmt.Errorf("RESTITCH_ADMIN_PORT must be 1-65535, got %d (admin.port)", f.Admin.Port))
	}

	logFormat := strings.ToLower(f.Server.LogFormat)
	if logFormat != "json" && logFormat != "text" {
		errs = append(errs, fmt.Errorf("RESTITCH_LOG_FORMAT must be json or text, got %q", f.Server.LogFormat))
	}
	logLevel := strings.ToLower(f.Server.LogLevel)
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		errs = append(errs, fmt.Errorf("RESTITCH_LOG_LEVEL must be debug/info/warn/error, got %q", f.Server.LogLevel))
	}

	switch f.Admin.Storage.Type {
	case "", "memory":
		// valid, no additional fields required
	case "sqlite":
		if f.Admin.Storage.URL == "" {
			errs = append(errs, fmt.Errorf("admin.storage.url is required when type is %q", f.Admin.Storage.Type))
		}
	case "turso":
		// The bundled sqlite driver cannot speak libsql (finding L8): a
		// libsql:// DSN used to silently create a local file named after the
		// URL. Reject it until a real libSQL client exists.
		errs = append(errs, fmt.Errorf("admin.storage.type %q is not supported by this build; use sqlite or memory", f.Admin.Storage.Type))
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

// ApplyEnvOverrides applies the RESTITCH_ADMIN_* environment variables.
// Server settings are flags, bound to env by BindEnv.
func ApplyEnvOverrides(f *File) {
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
