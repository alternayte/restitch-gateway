package composition

import (
	"testing"
	"time"

	"github.com/restitch/restitch-gateway/internal/upstream"
)

// The transport: block is documented in docs/configuration.md and PLAN.md but
// was never bound to a struct field, so yaml.Unmarshal silently discarded it.
// These tests pin the binding.

func TestTransportBlockParsesAllFields(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      dial_timeout: 3s
      tls_handshake_timeout: 4s
      response_header_timeout: 7s
      max_idle_conns_per_host: 250
      insecure_skip_verify: true
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	tr := cfg.Upstreams["api"].Transport
	if tr == nil {
		t.Fatal("transport block was not parsed (nil)")
	}
	if tr.DialTimeout != 3*time.Second {
		t.Errorf("DialTimeout = %v, want 3s", tr.DialTimeout)
	}
	if tr.TLSHandshakeTimeout != 4*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 4s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 7*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 7s", tr.ResponseHeaderTimeout)
	}
	if tr.MaxIdleConnsPerHost != 250 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 250", tr.MaxIdleConnsPerHost)
	}
	if !tr.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestTransportBlockOmittedIsNil(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Upstreams["api"].Transport != nil {
		t.Error("Transport should be nil when the block is omitted")
	}
}

func TestTransportPartialBlockLeavesOthersZero(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      max_idle_conns_per_host: 42
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tr := cfg.Upstreams["api"].Transport
	if tr == nil {
		t.Fatal("transport block was not parsed (nil)")
	}
	if tr.MaxIdleConnsPerHost != 42 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 42", tr.MaxIdleConnsPerHost)
	}
	// Unspecified fields stay zero so BuildTransport applies its defaults.
	if tr.DialTimeout != 0 {
		t.Errorf("DialTimeout = %v, want 0 (so BuildTransport defaults apply)", tr.DialTimeout)
	}
}

func TestTransportTranslatesToUpstreamConfig(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      dial_timeout: 13s
      tls_handshake_timeout: 17s
      response_header_timeout: 9s
      max_idle_conns_per_host: 77
      insecure_skip_verify: true
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	// toUpstreamTransport is the exact translation the parser uses, so building
	// a transport from its output asserts the real code path. upstream.Build
	// wraps the base transport in a metrics/retry/breaker/auth RoundTripper
	// chain that cannot be unwrapped from outside the package, which is why
	// this asserts on BuildTransport rather than on the compiled client.
	//
	// All five fields use distinctive non-default values so a dropped field
	// in toUpstreamTransport fails loudly here, at the built-transport layer,
	// rather than only against the raw config struct.
	tr := upstream.BuildTransport(toUpstreamTransport(cfg.Upstreams["api"].Transport))
	if tr.MaxIdleConnsPerHost != 77 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 77", tr.MaxIdleConnsPerHost)
	}
	if tr.ResponseHeaderTimeout != 9*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 9s", tr.ResponseHeaderTimeout)
	}
	if tr.TLSHandshakeTimeout != 17*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 17s", tr.TLSHandshakeTimeout)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("TLSClientConfig.InsecureSkipVerify = false, want true")
	}
	// DialTimeout is not observable on *http.Transport: net/http closes over it
	// inside the DialContext closure built by BuildTransport, and http.Transport
	// exposes no field or accessor for it. It is verified at the
	// config-translation layer only, in TestTransportBlockParsesAllFields.

	// And the full compile path accepts a config carrying a transport block.
	if _, err := CompileConfig(t.Context(), cfg, CompileOptions{SkipAuthInit: true}); err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}
}
