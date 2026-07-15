package hotreload

import (
	"testing"
	"time"
)

func TestBackoffDuration(t *testing.T) {
	base := 10 * time.Second
	max := 5 * time.Minute

	tests := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{0, 8 * time.Second, 12 * time.Second},       // 10s ± 20%
		{1, 16 * time.Second, 24 * time.Second},      // 20s ± 20%
		{2, 32 * time.Second, 48 * time.Second},      // 40s ± 20%
		{3, 64 * time.Second, 96 * time.Second},      // 80s ± 20%
		{4, 128 * time.Second, 192 * time.Second},    // 160s ± 20%
		{5, 240 * time.Second, 360 * time.Second},    // 300s (capped) ± 20%
		{10, 240 * time.Second, 360 * time.Second},   // still capped
	}

	for _, tt := range tests {
		for i := 0; i < 50; i++ {
			got := backoffDuration(tt.attempt, base, max)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("attempt=%d: got %v, want [%v, %v]", tt.attempt, got, tt.wantMin, tt.wantMax)
				break
			}
		}
	}
}
