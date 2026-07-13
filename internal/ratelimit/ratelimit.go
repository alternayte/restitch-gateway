// Package ratelimit provides per-key token-bucket rate limiting for HTTP requests.
package ratelimit

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// Config configures a rate limiter.
type Config struct {
	RequestsPerSecond float64 `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int     `yaml:"burst" json:"burst"`
	Key               string  `yaml:"key" json:"key"` // "ip", "header:X-Client-ID", "api-key"
}

// Limiter is a per-key token-bucket rate limiter backed by sync.Map.
type Limiter struct {
	rate    rate.Limit
	burst   int
	keyFunc func(*http.Request) string
	entries sync.Map // map[string]*rate.Limiter
}

// New creates a new Limiter from the given config.
func New(cfg Config) *Limiter {
	l := &Limiter{
		rate:  rate.Limit(cfg.RequestsPerSecond),
		burst: cfg.Burst,
	}

	key := cfg.Key
	if key == "" {
		key = "ip"
	}

	switch {
	case key == "ip":
		l.keyFunc = extractIP
	case key == "api-key":
		l.keyFunc = func(r *http.Request) string {
			return r.Header.Get("X-API-Key")
		}
	case strings.HasPrefix(key, "header:"):
		header := strings.TrimPrefix(key, "header:")
		l.keyFunc = func(r *http.Request) string {
			return r.Header.Get(header)
		}
	default:
		l.keyFunc = extractIP
	}

	return l
}

// Allow reports whether the request is allowed under the rate limit.
func (l *Limiter) Allow(r *http.Request) bool {
	key := l.keyFunc(r)
	if key == "" {
		key = "__empty__"
	}

	v, _ := l.entries.LoadOrStore(key, rate.NewLimiter(l.rate, l.burst))
	return v.(*rate.Limiter).Allow()
}

// Middleware returns an http.Handler that wraps next with rate limiting.
// Rejected requests receive a 429 response with Retry-After header.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(r) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractIP returns the IP address from the request, stripping the port.
func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
