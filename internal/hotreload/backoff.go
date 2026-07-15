package hotreload

import (
	"math"
	"math/rand/v2"
	"time"
)

func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if d > max {
		d = max
	}
	jitter := 0.8 + rand.Float64()*0.4 // ±20%
	return time.Duration(float64(d) * jitter)
}
