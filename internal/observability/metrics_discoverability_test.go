package observability

import "testing"

func TestNoopDiscoverabilityMetrics_NeverPanics(t *testing.T) {
	m := NoopDiscoverabilityMetrics()
	m.ObserveEmbeddingLatency(10, "ok")
	m.IncEmbeddingError("timeout")
	m.ObserveRecallLatency(5)
	m.IncFragmentCreate("created")
}

func TestInMemoryDiscoverabilityMetrics_RecordsEmbeddingLatency(t *testing.T) {
	m := NewInMemoryDiscoverabilityMetrics()
	m.ObserveEmbeddingLatency(123.4, "ok")
	m.ObserveEmbeddingLatency(987.6, "timeout")

	samples := m.EmbeddingSamples()
	if len(samples) != 2 {
		t.Fatalf("samples len = %d; want 2", len(samples))
	}
	if samples[0].DurationMs != 123.4 || samples[0].Outcome != "ok" {
		t.Errorf("sample[0] = %+v", samples[0])
	}
	if samples[1].DurationMs != 987.6 || samples[1].Outcome != "timeout" {
		t.Errorf("sample[1] = %+v", samples[1])
	}
}

func TestInMemoryDiscoverabilityMetrics_RecordsEmbeddingErrors(t *testing.T) {
	m := NewInMemoryDiscoverabilityMetrics()
	m.IncEmbeddingError("timeout")
	m.IncEmbeddingError("timeout")
	m.IncEmbeddingError("rate_limited")
	if got := m.EmbeddingErrorCount("timeout"); got != 2 {
		t.Errorf("timeout count = %d; want 2", got)
	}
	if got := m.EmbeddingErrorCount("rate_limited"); got != 1 {
		t.Errorf("rate_limited count = %d; want 1", got)
	}
	if got := m.EmbeddingErrorCount("never"); got != 0 {
		t.Errorf("unused code count = %d; want 0", got)
	}
}

func TestInMemoryDiscoverabilityMetrics_RecordsRecallLatency(t *testing.T) {
	m := NewInMemoryDiscoverabilityMetrics()
	m.ObserveRecallLatency(42.0)
	m.ObserveRecallLatency(7.5)
	out := m.RecallLatencies()
	if len(out) != 2 || out[0] != 42.0 || out[1] != 7.5 {
		t.Errorf("latencies = %+v", out)
	}
}

func TestInMemoryDiscoverabilityMetrics_RecordsFragmentCreateOutcomes(t *testing.T) {
	m := NewInMemoryDiscoverabilityMetrics()
	m.IncFragmentCreate("created")
	m.IncFragmentCreate("created")
	m.IncFragmentCreate("duplicate")
	if got := m.FragmentCreateCount("created"); got != 2 {
		t.Errorf("created = %d; want 2", got)
	}
	if got := m.FragmentCreateCount("duplicate"); got != 1 {
		t.Errorf("duplicate = %d; want 1", got)
	}
}

func TestInMemoryDiscoverabilityMetrics_ConcurrentSafe(t *testing.T) {
	m := NewInMemoryDiscoverabilityMetrics()
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			m.ObserveEmbeddingLatency(1, "ok")
			m.IncEmbeddingError("timeout")
			m.ObserveRecallLatency(1)
			m.IncFragmentCreate("created")
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	if got := len(m.EmbeddingSamples()); got != 50 {
		t.Errorf("embedding samples = %d; want 50", got)
	}
	if got := m.FragmentCreateCount("created"); got != 50 {
		t.Errorf("fragment_create created = %d; want 50", got)
	}
}
