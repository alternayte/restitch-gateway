package upstream

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// TransportConfig mirrors the YAML upstream.transport block.
type TransportConfig struct {
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	MaxIdleConnsPerHost   int
	InsecureSkipVerify    bool
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
