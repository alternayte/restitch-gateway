package hotreload

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/alternayte/restitch-gateway/internal/observability"
)

type PollStatus struct {
	LastPollTime      time.Time `json:"last_poll"`
	LastSuccessTime   time.Time `json:"last_success"`
	LastETag          string    `json:"etag"`
	CompositionCount  int       `json:"composition_count"`
	LastError         string    `json:"error"`
	ErrorType         string    `json:"error_type"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
}

type Poller struct {
	client    *RegistryClient
	interval  time.Duration
	reloadFn  func(yaml []byte) (string, error)
	triggerCh chan struct{}
	status    atomic.Pointer[PollStatus]
	metrics   *observability.Metrics
	bo        *backoff.ExponentialBackOff
}

func NewPoller(
	client *RegistryClient,
	interval time.Duration,
	reloadFn func(yaml []byte) (string, error),
	metrics *observability.Metrics,
) *Poller {
	p := &Poller{
		client:    client,
		interval:  interval,
		reloadFn:  reloadFn,
		triggerCh: make(chan struct{}, 1),
		metrics:   metrics,
	}
	p.status.Store(&PollStatus{})

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = interval
	bo.MaxInterval = 5 * time.Minute
	p.bo = bo

	return p
}

func (p *Poller) Run(ctx context.Context) error {
	consecutiveErrors := 0
	lastETag := ""

	for {
		start := time.Now()
		result, pollErr := p.poll(ctx, lastETag)
		elapsed := time.Since(start)

		p.recordMetricsDuration(elapsed)

		now := time.Now()
		s := p.Status()
		s.LastPollTime = now

		if pollErr != nil {
			consecutiveErrors++
			errType := classifyError(pollErr)
			s.LastError = pollErr.Error()
			s.ErrorType = errType
			s.ConsecutiveErrors = consecutiveErrors
			p.status.Store(&s)
			p.recordMetricsResult("error")

			slog.Error("registry poll failed",
				"error", pollErr, "type", errType,
				"consecutive", consecutiveErrors)

			wait := p.bo.NextBackOff()
			if !p.sleepOrTrigger(ctx, wait) {
				return ctx.Err()
			}
			continue
		}

		consecutiveErrors = 0
		s.ConsecutiveErrors = 0
		s.LastError = ""
		s.ErrorType = ""
		p.bo.Reset()

		if result.NotModified {
			s.LastSuccessTime = now
			p.status.Store(&s)
			p.recordMetricsResult("not_modified")
		} else {
			lastETag = result.ETag
			s.LastETag = result.ETag
			s.CompositionCount = result.CompositionCount
			s.LastSuccessTime = now
			p.status.Store(&s)
			p.recordMetricsResult("success")
		}

		if !p.sleepOrTrigger(ctx, p.interval) {
			return ctx.Err()
		}
	}
}

func (p *Poller) poll(ctx context.Context, lastETag string) (*FetchResult, error) {
	result, err := p.client.Fetch(ctx, lastETag)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if result.NotModified {
		return result, nil
	}
	if _, err := p.reloadFn(result.YAML); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}
	return result, nil
}

func (p *Poller) Trigger() {
	select {
	case p.triggerCh <- struct{}{}:
		slog.Info("registry poll triggered")
	default:
	}
}

func (p *Poller) Status() PollStatus {
	if s := p.status.Load(); s != nil {
		return *s
	}
	return PollStatus{}
}

func (p *Poller) sleepOrTrigger(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-p.triggerCh:
		return true
	case <-t.C:
		return true
	}
}

func classifyError(err error) string {
	msg := err.Error()
	if len(msg) >= 6 && msg[:6] == "fetch:" {
		return "fetch"
	}
	if len(msg) >= 7 && msg[:7] == "reload:" {
		return "reload"
	}
	return "fetch"
}

func (p *Poller) recordMetricsResult(result string) {
	if p.metrics == nil || p.metrics.RegistryPollsTotal == nil {
		return
	}
	p.metrics.RegistryPollsTotal.WithLabelValues(result).Inc()
	if result != "error" {
		p.metrics.RegistryLastSuccess.SetToCurrentTime()
	}
}

func (p *Poller) recordMetricsDuration(d time.Duration) {
	if p.metrics == nil || p.metrics.RegistryPollDuration == nil {
		return
	}
	p.metrics.RegistryPollDuration.Observe(d.Seconds())
}
