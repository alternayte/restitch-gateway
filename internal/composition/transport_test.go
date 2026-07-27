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

	// toUpstreamTransport is the exact translation the parser uses. Its
	// returned upstream.TransportConfig is asserted directly here (all five
	// fields) because that is the layer where a dropped/mistranslated field
	// would first go undetected — some of these fields (DialTimeout in
	// particular) are not independently observable once BuildTransport turns
	// them into an *http.Transport, since net/http closes DialTimeout over
	// inside the DialContext closure with no field or accessor exposing it
	// back out. Asserting here, on the direct return value of the function
	// under test, is therefore the only place DialTimeout can be pinned at
	// all, and is more precise than the *http.Transport layer for every
	// other field too.
	got := toUpstreamTransport(cfg.Upstreams["api"].Transport)
	want := upstream.TransportConfig{
		DialTimeout:           13 * time.Second,
		TLSHandshakeTimeout:   17 * time.Second,
		ResponseHeaderTimeout: 9 * time.Second,
		MaxIdleConnsPerHost:   77,
		InsecureSkipVerify:    true,
	}
	if got != want {
		t.Errorf("toUpstreamTransport() = %+v, want %+v", got, want)
	}

	// Also build a transport from that output so the fields observable on
	// *http.Transport (everything but DialTimeout) get a second, independent
	// check at the layer BuildTransport actually produces.
	tr := upstream.BuildTransport(got)
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

	// And the full compile path accepts a config carrying a transport block.
	if _, err := CompileConfig(t.Context(), cfg, CompileOptions{SkipAuthInit: true}); err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}
}

// TestTransportEndToEndThroughCompileConfig closes the gap left by the tests
// above: they all call toUpstreamTransport and upstream.BuildTransport
// directly, so nothing proves that ParseConfig + CompileConfig actually wire
// a parsed transport: block into the compiled upstream's HTTP client. A
// mutation that makes the parser silently discard up.Transport before
// building the upstream (the exact bug this milestone fixed) would leave
// every other test in this file green.
//
// upstream.Build wraps the base *http.Transport in a metrics -> retry ->
// breaker -> auth -> transport RoundTripper chain that cannot be unwrapped
// from outside the upstream package, so this test relies on
// upstream.Upstream.BaseTransport, which upstream.Build now sets to the same
// *http.Transport value it wraps, purely for introspection/testing.
//
// MaxIdleConnsPerHost uses 7: neither the Go zero-value default (2) nor
// restitch's own BuildTransport fallback (100), so silently falling back to
// either one fails loudly here.
func TestTransportEndToEndThroughCompileConfig(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      max_idle_conns_per_host: 7
      insecure_skip_verify: true
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiled, err := CompileConfig(t.Context(), cfg, CompileOptions{SkipAuthInit: true})
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	up, ok := compiled.Upstreams["api"]
	if !ok {
		t.Fatal(`compiled.Upstreams["api"] missing`)
	}
	if up.BaseTransport == nil {
		t.Fatal("BaseTransport is nil")
	}
	if up.BaseTransport.MaxIdleConnsPerHost != 7 {
		t.Errorf("BaseTransport.MaxIdleConnsPerHost = %d, want 7", up.BaseTransport.MaxIdleConnsPerHost)
	}
	if up.BaseTransport.TLSClientConfig == nil {
		t.Fatal("BaseTransport.TLSClientConfig is nil")
	}
	if !up.BaseTransport.TLSClientConfig.InsecureSkipVerify {
		t.Error("BaseTransport.TLSClientConfig.InsecureSkipVerify = false, want true")
	}
}
