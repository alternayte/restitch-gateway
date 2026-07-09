package admin

import (
	"math"
	"sort"
	"sync"
	"time"
)

type Accumulator struct {
	mu      sync.Mutex
	global  *accBucket
	perComp map[string]*accBucket
}

type accBucket struct {
	requests  int64
	errors    int64
	partials  int64
	latencies []float64
	steps     map[string]*accStep
}

type accStep struct {
	latencies []float64
	errors    int64
	upstream  string
}

func NewAccumulator() *Accumulator {
	return &Accumulator{
		global:  newAccBucket(),
		perComp: make(map[string]*accBucket),
	}
}

func newAccBucket() *accBucket {
	return &accBucket{steps: make(map[string]*accStep)}
}

func (a *Accumulator) Record(composition string, durationMS float64, isError, isPartial bool, steps []StepSample) {
	a.mu.Lock()
	defer a.mu.Unlock()

	recordBucket(a.global, durationMS, isError, isPartial, steps)

	cb, ok := a.perComp[composition]
	if !ok {
		cb = newAccBucket()
		a.perComp[composition] = cb
	}
	recordBucket(cb, durationMS, isError, isPartial, steps)
}

func recordBucket(b *accBucket, durationMS float64, isError, isPartial bool, steps []StepSample) {
	b.requests++
	if isError {
		b.errors++
	}
	if isPartial {
		b.partials++
	}
	b.latencies = append(b.latencies, durationMS)
	for _, s := range steps {
		as, ok := b.steps[s.Name]
		if !ok {
			as = &accStep{upstream: s.Upstream}
			b.steps[s.Name] = as
		}
		as.latencies = append(as.latencies, s.DurationMS)
		if s.IsError {
			as.errors++
		}
	}
}

func (a *Accumulator) Flush() []Bucket {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().Truncate(time.Minute)
	var buckets []Bucket

	buckets = append(buckets, toBucket(now, "", a.global))
	for comp, ab := range a.perComp {
		buckets = append(buckets, toBucket(now, comp, ab))
	}

	a.global = newAccBucket()
	a.perComp = make(map[string]*accBucket)

	return buckets
}

func toBucket(ts time.Time, composition string, ab *accBucket) Bucket {
	b := Bucket{
		Timestamp:      ts,
		Composition:    composition,
		Requests:       ab.requests,
		Errors:         ab.errors,
		Partials:       ab.partials,
		LatencyBuckets: computeLatencyBuckets(ab.latencies),
	}

	if len(ab.latencies) > 0 {
		b.LatencyP50 = percentile(ab.latencies, 0.50)
		b.LatencyP95 = percentile(ab.latencies, 0.95)
		b.LatencyP99 = percentile(ab.latencies, 0.99)
	}

	if len(ab.steps) > 0 {
		b.StepMetrics = make(map[string]*StepBucket, len(ab.steps))
		for name, as := range ab.steps {
			sb := &StepBucket{
				Requests: int64(len(as.latencies)),
				Errors:   as.errors,
				Upstream: as.upstream,
			}
			if len(as.latencies) > 0 {
				var sum float64
				for _, v := range as.latencies {
					sum += v
				}
				sb.AvgMS = math.Round(sum/float64(len(as.latencies))*100) / 100
				sb.P95MS = percentile(as.latencies, 0.95)
			}
			b.StepMetrics[name] = sb
		}
	}

	return b
}

func computeLatencyBuckets(latencies []float64) []int64 {
	buckets := make([]int64, len(LatencyBucketBounds)+1)
	for _, v := range latencies {
		placed := false
		for i, bound := range LatencyBucketBounds {
			if v <= bound {
				buckets[i]++
				placed = true
				break
			}
		}
		if !placed {
			buckets[len(buckets)-1]++
		}
	}
	return buckets
}

func percentile(data []float64, p float64) float64 {
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return math.Round(sorted[idx]*100) / 100
}
