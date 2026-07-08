package upstream

import (
	"context"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

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
	next http.RoundTripper
	cfg  RetryConfig
	name string
}

func newRetryTripper(next http.RoundTripper, cfg RetryConfig, name string) http.RoundTripper {
	cfg = applyRetryDefaults(cfg)
	return &retryTripper{next: next, cfg: cfg, name: name}
}

func (rt *retryTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !rt.isRetryable(req.Method) {
		return rt.next.RoundTrip(req)
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt < rt.cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
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
			if !rt.shouldRetry(req.Context(), attempt, 0, nil) {
				return nil, err
			}
			continue
		}

		if rt.isDropOn(resp.StatusCode) {
			return resp, nil
		}

		if rt.isBackoffOn(resp.StatusCode) {
			if lastResp != nil {
				drainBody(lastResp.Body)
			}
			lastResp = resp
			lastErr = nil
			if !rt.shouldRetry(req.Context(), attempt, resp.StatusCode, resp) {
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

func (rt *retryTripper) isRetryable(method string) bool {
	if rt.cfg.RetryNonIdempotent {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

func (rt *retryTripper) isBackoffOn(status int) bool {
	for _, s := range rt.cfg.BackoffOn {
		if s == status {
			return true
		}
	}
	return false
}

func (rt *retryTripper) isDropOn(status int) bool {
	for _, s := range rt.cfg.DropOn {
		if s == status {
			return true
		}
	}
	return false
}

func (rt *retryTripper) shouldRetry(ctx context.Context, attempt, status int, resp *http.Response) bool {
	if attempt+1 >= rt.cfg.MaxAttempts {
		return false
	}

	sleepDur := rt.backoff(attempt)
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				sleepDur = time.Duration(secs) * time.Second
				if sleepDur > rt.cfg.MaxBackoff {
					sleepDur = rt.cfg.MaxBackoff
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

func (rt *retryTripper) backoff(attempt int) time.Duration {
	base := float64(rt.cfg.Interval) * math.Pow(2, float64(attempt))
	jitter := 0.8 + 0.4*rand.Float64()
	d := time.Duration(base * jitter)
	if d > rt.cfg.MaxBackoff {
		d = rt.cfg.MaxBackoff
	}
	return d
}

func drainBody(body io.ReadCloser) {
	if body != nil {
		io.Copy(io.Discard, body)
		body.Close()
	}
}
