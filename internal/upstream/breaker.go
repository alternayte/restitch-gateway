package upstream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sony/gobreaker/v2"
)

// BreakerConfig configures a circuit breaker for an upstream.
type BreakerConfig struct {
	MaxFailures int
	Interval    time.Duration
	Timeout     time.Duration
}

type breakerTripper struct {
	cb   *gobreaker.CircuitBreaker[*http.Response]
	next http.RoundTripper
}

func newBreakerTripper(next http.RoundTripper, cfg BreakerConfig, name string) http.RoundTripper {
	settings := gobreaker.Settings{
		Name:     name,
		Interval: cfg.Interval,
		Timeout:  cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(cfg.MaxFailures)
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Info("circuit breaker state change",
				"upstream", name,
				"from", from.String(),
				"to", to.String())
		},
	}

	cb := gobreaker.NewCircuitBreaker[*http.Response](settings)
	return &breakerTripper{cb: cb, next: next}
}

var errBreakerCountedStatus = errors.New("breaker: upstream 5xx")

func (bt *breakerTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := bt.cb.Execute(func() (*http.Response, error) {
		resp, err := bt.next.RoundTrip(req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &breakerNonFailure{err: err}
			}
			return nil, err
		}
		if resp.StatusCode >= 500 {
			return resp, errBreakerCountedStatus
		}
		return resp, nil
	})

	if err != nil {
		var nf *breakerNonFailure
		if errors.As(err, &nf) {
			return nil, nf.err
		}
		if errors.Is(err, errBreakerCountedStatus) {
			return resp, nil
		}
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("upstream %s: circuit open: %w", bt.cb.Name(), err)
		}
		return nil, err
	}
	return resp, nil
}

// breakerNonFailure wraps errors that should not count as breaker failures.
type breakerNonFailure struct {
	err error
}

func (e *breakerNonFailure) Error() string { return e.err.Error() }
func (e *breakerNonFailure) Unwrap() error { return e.err }
