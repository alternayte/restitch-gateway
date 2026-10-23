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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/upstream"
)

// HeartbeatInterval is how often a managed gateway reports to Studio.
const HeartbeatInterval = 15 * time.Second

// Heartbeat is what one gateway instance reports about itself.
type Heartbeat struct {
	InstanceID     string `json:"instance_id"`
	BinaryVersion  string `json:"binary_version"`
	AppliedVersion int    `json:"applied_version"`
	LastPullError  string `json:"last_pull_error,omitempty"`
}

// NewInstanceID names this process: the host name plus a random suffix,
// so two replicas on one host stay apart.
func NewInstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "gateway"
	}
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return host + "-" + hex.EncodeToString(b)
}

// SendHeartbeat posts one heartbeat to Studio.
func (c *Client) SendHeartbeat(ctx context.Context, hb Heartbeat) error {
	body, err := json.Marshal(hb)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/gateway/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	defer upstream.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("heartbeat: studio returned status %d", resp.StatusCode)
	}
	return nil
}

// RunHeartbeat sends a heartbeat every interval, and at once when now
// receives, until ctx ends. A failed heartbeat is logged and retried on
// the next tick; it never touches request handling.
func RunHeartbeat(ctx context.Context, c *Client, interval time.Duration, now <-chan struct{}, report func() Heartbeat) {
	t := time.NewTicker(interval)
	defer t.Stop()
	send := func() {
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := c.SendHeartbeat(sendCtx, report()); err != nil && ctx.Err() == nil {
			slog.Warn("heartbeat failed", "error", err)
		}
	}
	send()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			send()
		case <-now:
			send()
		}
	}
}
