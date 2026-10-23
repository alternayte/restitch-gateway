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

package main

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

// TestGatewayShutdown_Order pins the shutdown order: servers drain first,
// then background work stops, then the final step runs with a live
// deadline. Defer order once closed storage before its final flush.
func TestGatewayShutdown_Order(t *testing.T) {
	var mu sync.Mutex
	var steps []string
	step := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		steps = append(steps, s)
	}

	sd := &gatewayShutdown{
		servers: []func(context.Context) error{
			func(context.Context) error { step("gateway-server"); return nil },
			func(context.Context) error { step("ops-server"); return nil },
		},
		stopBackground: func() { step("background") },
		final: func(ctx context.Context) {
			if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
				t.Error("the final step needs its own live deadline")
			}
			step("final")
		},
		timeout: 5 * time.Second,
	}
	if err := sd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := []string{"gateway-server", "ops-server", "background", "final"}; !slices.Equal(steps, want) {
		t.Errorf("steps = %v, want %v", steps, want)
	}
}
