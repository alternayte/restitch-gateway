// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hotreload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/observability"
	"github.com/cenkalti/backoff/v5"
)

type PollStatus struct {
	LastPollTime      time.Time `json:"last_poll"`
	LastSuccessTime   time.Time `json:"last_success"`
	LastETag          string    `json:"etag"`
	Version           int       `json:"version"`
	LastError         string    `json:"error"`
	ErrorType         string    `json:"error_type"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
}

type Poller struct {
	client    *Client
	interval  time.Duration
	reloadFn  func(yaml []byte) (string, error)
	triggerCh chan struct{}
	status    atomic.Pointer[PollStatus]
	metrics   *observability.Metrics
	bo        *backoff.ExponentialBackOff
	// onApplied runs after every version the reload function accepted.
	onApplied func(version int)
}

// OnApplied sets a callback that runs after each applied version. The
// gateway uses it to turn /ready on after the first one.
func (p *Poller) OnApplied(fn func(version int)) { p.onApplied = fn }

func NewPoller(
	client *Client,
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

			if errors.Is(pollErr, ErrNoVersion) {
				slog.Info("waiting for the first published version", "consecutive", consecutiveErrors)
			} else {
				slog.Error("artifact pull failed",
					"error", pollErr, "type", errType,
					"consecutive", consecutiveErrors)
			}

			// No published version yet is a normal state, not a fault:
			// keep the poll interval, so the first publish arrives quickly.
			wait := p.interval
			if !errors.Is(pollErr, ErrNoVersion) {
				wait = p.bo.NextBackOff()
			}
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
			s.Version = result.Version
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
	if p.onApplied != nil {
		p.onApplied(result.Version)
	}
	return result, nil
}

func (p *Poller) Trigger() {
	select {
	case p.triggerCh <- struct{}{}:
		slog.Info("artifact pull triggered")
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
	if errors.Is(err, ErrNoVersion) {
		return "no_version"
	}
	if errors.Is(err, ErrTokenRejected) {
		return "token"
	}
	if len(msg) >= 6 && msg[:6] == "fetch:" {
		return "fetch"
	}
	if len(msg) >= 7 && msg[:7] == "reload:" {
		return "reload"
	}
	return "fetch"
}

func (p *Poller) recordMetricsResult(result string) {
	if p.metrics == nil || p.metrics.StudioPullsTotal == nil {
		return
	}
	p.metrics.StudioPullsTotal.WithLabelValues(result).Inc()
	if result != "error" {
		p.metrics.StudioLastPullSuccess.SetToCurrentTime()
	}
}

func (p *Poller) recordMetricsDuration(d time.Duration) {
	if p.metrics == nil || p.metrics.StudioPullDuration == nil {
		return
	}
	p.metrics.StudioPullDuration.Observe(d.Seconds())
}
