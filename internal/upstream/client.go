package upstream

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/restitch/restitch-gateway/internal/auth"
	"github.com/restitch/restitch-gateway/internal/observability"
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

// BuildConfig holds all configuration needed to build an upstream client.
type BuildConfig struct {
	Name             string
	BaseURL          string
	Timeout          time.Duration
	MaxResponseBytes int64
	HealthPath       string
	Transport        TransportConfig
	Auth             auth.Strategy
	Retry            *RetryConfig
	Breaker          *BreakerConfig
	Metrics          *observability.Metrics
}

// Build assembles a per-upstream *http.Client with the RoundTripper chain:
// retry → breaker → auth → transport. Timeout: 0 — deadlines come from step context.
func Build(cfg BuildConfig) *Upstream {
	transport := BuildTransport(cfg.Transport)

	var rt http.RoundTripper = transport
	if cfg.Auth != nil {
		rt = cfg.Auth.RoundTripper(rt)
	}
	if cfg.Breaker != nil {
		rt = newBreakerTripper(rt, *cfg.Breaker, cfg.Name)
	}
	if cfg.Retry != nil {
		rt = newRetryTripper(rt, *cfg.Retry, cfg.Name, cfg.Metrics)
	}
	rt = newMetricsTripper(rt, cfg.Name, cfg.Metrics)

	return &Upstream{
		Name:    cfg.Name,
		BaseURL: cfg.BaseURL,
		Client: &http.Client{
			Transport: rt,
			Timeout:   0,
		},
		MaxResponseBytes: cfg.MaxResponseBytes,
		Timeout:          cfg.Timeout,
		HealthPath:       cfg.HealthPath,
	}
}
