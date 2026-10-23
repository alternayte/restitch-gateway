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
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
		case block == "admin":
			errs = append(errs, fmt.Errorf("%s removed in v2: the gateway has no admin API; Studio shows its state", name))
		case known && dest == "":
			errs = append(errs, fmt.Errorf("%s removed in v2", name))
		case known:
			errs = append(errs, fmt.Errorf("%s moved to %s", name, dest))
		default:
			flagName := strings.ReplaceAll(val.Content[i].Value, "_", "-")
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

	logFormat := strings.ToLower(f.Server.LogFormat)
	if logFormat != "json" && logFormat != "text" {
		errs = append(errs, fmt.Errorf("RESTITCH_LOG_FORMAT must be json or text, got %q", f.Server.LogFormat))
	}
	logLevel := strings.ToLower(f.Server.LogLevel)
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		errs = append(errs, fmt.Errorf("RESTITCH_LOG_LEVEL must be debug/info/warn/error, got %q", f.Server.LogLevel))
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
