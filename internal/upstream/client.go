package upstream

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/restitch/restitch-gateway/internal/auth"
)

// TransportConfig mirrors the YAML upstream.transport block.
type TransportConfig struct {
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	MaxIdleConnsPerHost   int
	InsecureSkipVerify    bool
}

// Upstream holds a compiled upstream with its HTTP client ready for use.
type Upstream struct {
	Name             string
	BaseURL          string
	Client           *http.Client
	MaxResponseBytes int64
	Timeout          time.Duration
	HealthPath       string
}

// BuildTransport returns a hardened *http.Transport with sensible defaults.
func BuildTransport(tc TransportConfig) *http.Transport {
	dialTimeout := tc.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	tlsHandshakeTimeout := tc.TLSHandshakeTimeout
	if tlsHandshakeTimeout == 0 {
		tlsHandshakeTimeout = 5 * time.Second
	}

	maxIdleConnsPerHost := tc.MaxIdleConnsPerHost
	if maxIdleConnsPerHost == 0 {
		maxIdleConnsPerHost = 100
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: tc.ResponseHeaderTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: tc.InsecureSkipVerify,
		},
	}
}

// Build assembles a per-upstream *http.Client with the RoundTripper chain:
// auth → transport. Timeout: 0 — deadlines come from step context.
func Build(name, baseURL string, timeout time.Duration, maxResponseBytes int64, healthPath string, tc TransportConfig, authStrategy auth.Strategy) *Upstream {
	transport := BuildTransport(tc)

	var rt http.RoundTripper = transport
	if authStrategy != nil {
		rt = authStrategy.RoundTripper(rt)
	}

	return &Upstream{
		Name:    name,
		BaseURL: baseURL,
		Client: &http.Client{
			Transport: rt,
			Timeout:   0, // deadlines via per-step context
		},
		MaxResponseBytes: maxResponseBytes,
		Timeout:          timeout,
		HealthPath:       healthPath,
	}
}
