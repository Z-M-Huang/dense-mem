package observability

import "sync"

// DiscoverabilityMetrics is the minimal contract Unit 25 services emit to.
// Implementations must be safe for concurrent use. A no-op implementation is
// returned by NoopDiscoverabilityMetrics and is suitable as a default when a
// production metrics backend (e.g. Prometheus) is not wired.
type DiscoverabilityMetrics interface {
	// ObserveEmbeddingLatency records one embedding-provider call.
	// outcome must be one of: "ok" | "timeout" | "error" | "rate_limited".
	ObserveEmbeddingLatency(durationMs float64, outcome string)
	// IncEmbeddingError bumps the error counter for the given failure code
	// so dashboards can surface which failure mode dominates.
	IncEmbeddingError(code string)
	// ObserveRecallLatency records one recall-service call end-to-end.
	ObserveRecallLatency(durationMs float64)
	// IncFragmentCreate bumps the fragment-create outcome counter.
	// outcome must be one of: "created" | "duplicate" | "error".
	IncFragmentCreate(outcome string)
}

// NoopDiscoverabilityMetrics returns a DiscoverabilityMetrics that discards
// every call. Use this as the default when no metrics backend is configured —
// call sites never need nil checks.
func NoopDiscoverabilityMetrics() DiscoverabilityMetrics { return noopMetrics{} }

type noopMetrics struct{}

var _ DiscoverabilityMetrics = noopMetrics{}

func (noopMetrics) ObserveEmbeddingLatency(float64, string) {}
func (noopMetrics) IncEmbeddingError(string)                {}
func (noopMetrics) ObserveRecallLatency(float64)            {}
func (noopMetrics) IncFragmentCreate(string)                {}

// InMemoryDiscoverabilityMetrics is a test-friendly recorder. Tests can
// inspect the captured samples to assert that a code path actually emitted
// the expected metric without standing up Prometheus.
type InMemoryDiscoverabilityMetrics struct {
	mu                sync.Mutex
	embeddingSamples  []EmbeddingSample
	embeddingErrors   map[string]int
	recallLatencies   []float64
	fragmentOutcomes  map[string]int
}

// EmbeddingSample is one recorded embedding latency observation.
type EmbeddingSample struct {
	DurationMs float64
	Outcome    string
}

var _ DiscoverabilityMetrics = (*InMemoryDiscoverabilityMetrics)(nil)

// NewInMemoryDiscoverabilityMetrics constructs a fresh recorder.
func NewInMemoryDiscoverabilityMetrics() *InMemoryDiscoverabilityMetrics {
	return &InMemoryDiscoverabilityMetrics{
		embeddingErrors:  make(map[string]int),
		fragmentOutcomes: make(map[string]int),
	}
}

// ObserveEmbeddingLatency records one sample.
func (m *InMemoryDiscoverabilityMetrics) ObserveEmbeddingLatency(durationMs float64, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.embeddingSamples = append(m.embeddingSamples, EmbeddingSample{DurationMs: durationMs, Outcome: outcome})
}

// IncEmbeddingError bumps the counter for `code`.
func (m *InMemoryDiscoverabilityMetrics) IncEmbeddingError(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.embeddingErrors[code]++
}

// ObserveRecallLatency records one end-to-end recall duration.
func (m *InMemoryDiscoverabilityMetrics) ObserveRecallLatency(durationMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recallLatencies = append(m.recallLatencies, durationMs)
}

// IncFragmentCreate bumps the fragment-create outcome counter.
func (m *InMemoryDiscoverabilityMetrics) IncFragmentCreate(outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fragmentOutcomes[outcome]++
}

// EmbeddingSamples returns a copy of the recorded embedding samples.
func (m *InMemoryDiscoverabilityMetrics) EmbeddingSamples() []EmbeddingSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]EmbeddingSample, len(m.embeddingSamples))
	copy(out, m.embeddingSamples)
	return out
}

// EmbeddingErrorCount returns the recorded count for `code`.
func (m *InMemoryDiscoverabilityMetrics) EmbeddingErrorCount(code string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.embeddingErrors[code]
}

// RecallLatencies returns a copy of the recorded recall-latency samples.
func (m *InMemoryDiscoverabilityMetrics) RecallLatencies() []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]float64, len(m.recallLatencies))
	copy(out, m.recallLatencies)
	return out
}

// FragmentCreateCount returns the recorded count for `outcome`.
func (m *InMemoryDiscoverabilityMetrics) FragmentCreateCount(outcome string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fragmentOutcomes[outcome]
}
