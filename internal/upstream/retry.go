package upstream

import (
	"context"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/alternayte/restitch-gateway/internal/observability"
)

type retryOverrideKey struct{}

// WithRetryOverride stores a per-step retry config override on the context.
func WithRetryOverride(ctx context.Context, cfg *RetryConfig) context.Context {
	return context.WithValue(ctx, retryOverrideKey{}, cfg)
}

func retryOverride(ctx context.Context) *RetryConfig {
	v, _ := ctx.Value(retryOverrideKey{}).(*RetryConfig)
	return v
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxAttempts        int
	Interval           time.Duration
	MaxBackoff         time.Duration
	BackoffOn          []int
	DropOn             []int
	RetryNonIdempotent bool
}

func applyRetryDefaults(cfg RetryConfig) RetryConfig {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 250 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Second
	}
	if len(cfg.BackoffOn) == 0 {
		cfg.BackoffOn = []int{429, 502, 503, 504}
	}
	return cfg
}

type retryTripper struct {
	next    http.RoundTripper
	cfg     RetryConfig
	name    string
	metrics *observability.Metrics
}

func newRetryTripper(next http.RoundTripper, cfg RetryConfig, name string, metrics *observability.Metrics) http.RoundTripper {
	cfg = applyRetryDefaults(cfg)
	return &retryTripper{next: next, cfg: cfg, name: name, metrics: metrics}
}

func (rt *retryTripper) effectiveConfig(ctx context.Context) RetryConfig {
	if override := retryOverride(ctx); override != nil {
		return applyRetryDefaults(*override)
	}
	return rt.cfg
}

func (rt *retryTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cfg := rt.effectiveConfig(req.Context())

	if !isRetryableMethod(req.Method, cfg.RetryNonIdempotent) {
		return rt.next.RoundTrip(req)
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			if rt.metrics != nil {
				rt.metrics.RetriesTotal.WithLabelValues(rt.name).Inc()
			}
			// A body that cannot be recreated must not be re-sent consumed:
			// there is nothing to retry with (finding M1). Return the last
			// outcome instead.
			if req.Body != nil && req.GetBody == nil {
				if lastResp != nil {
					return lastResp, nil
				}
				return nil, lastErr
			}
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return lastResp, lastErr
				}
				req.Body = body
			}
		}

		resp, err := rt.next.RoundTrip(req)

		if err != nil {
			lastErr = err
			lastResp = nil
			if !shouldRetry(req.Context(), attempt, cfg, nil) {
				return nil, err
			}
			continue
		}

		if isInStatusList(resp.StatusCode, cfg.DropOn) {
			return resp, nil
		}

		if isInStatusList(resp.StatusCode, cfg.BackoffOn) {
			if lastResp != nil {
				drainBody(lastResp.Body)
			}
			lastResp = resp
			lastErr = nil
			if !shouldRetry(req.Context(), attempt, cfg, resp) {
				return resp, nil
			}
			drainBody(resp.Body)
			continue
		}

		return resp, nil
	}

	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

func isRetryableMethod(method string, retryNonIdempotent bool) bool {
	if retryNonIdempotent {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

func isInStatusList(status int, list []int) bool {
	for _, s := range list {
		if s == status {
			return true
		}
	}
	return false
}

func shouldRetry(ctx context.Context, attempt int, cfg RetryConfig, resp *http.Response) bool {
	if attempt+1 >= cfg.MaxAttempts {
		return false
	}

	sleepDur := retryBackoff(attempt, cfg)
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				// A negative Retry-After must not produce an immediate
				// (negative) sleep; clamp at zero (finding M2).
				if secs < 0 {
					secs = 0
				}
				sleepDur = time.Duration(secs) * time.Second
				if sleepDur > cfg.MaxBackoff {
					sleepDur = cfg.MaxBackoff
				}
			}
		}
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(sleepDur):
		return true
	}
}

func retryBackoff(attempt int, cfg RetryConfig) time.Duration {
	base := float64(cfg.Interval) * math.Pow(2, float64(attempt))
	jitter := 0.8 + 0.4*rand.Float64()
	d := time.Duration(base * jitter)
	if d > cfg.MaxBackoff {
		d = cfg.MaxBackoff
	}
	return d
}

func drainBody(body io.ReadCloser) {
	if body != nil {
		_, _ = io.Copy(io.Discard, body)
		body.Close()
	}
}
