package verifier

import (
	"context"
	"errors"
	"time"

	"github.com/dense-mem/dense-mem/internal/config"
	"github.com/dense-mem/dense-mem/internal/observability"
)

// RetryVerifier wraps a Verifier with up to three retry attempts and a
// process-global concurrency semaphore. It is safe for concurrent use.
//
// Retry policy:
//   - max 3 attempts total
//   - retries on: ErrVerifierTimeout, ErrVerifierProvider (5xx), ErrVerifierRateLimit (429)
//   - on 429 with Retry-After set, sleeps exactly that duration before the next attempt
//   - preserves the original sentinel error class on final failure
type RetryVerifier struct {
	inner      Verifier
	maxRetries int
	// sem is a buffered channel used as a counting semaphore to cap the number
	// of concurrent verify calls for the lifetime of this instance.
	// Sized from AI_VERIFIER_MAX_CONCURRENCY at construction time.
	sem     chan struct{}
	logger  observability.LogProvider
	metrics observability.DiscoverabilityMetrics
}

// Compile-time assertion that RetryVerifier implements Verifier.
var _ Verifier = (*RetryVerifier)(nil)

// NewRetryVerifier wraps inner with retry logic and a concurrency semaphore.
// The semaphore capacity is cfg.GetAIVerifierMaxConcurrency(); falls back to 5
// if the value is not positive. The logger is optional — if omitted or nil a
// no-op logger is used.
func NewRetryVerifier(inner Verifier, cfg config.ConfigProvider, logger ...observability.LogProvider) *RetryVerifier {
	var log observability.LogProvider
	if len(logger) > 0 && logger[0] != nil {
		log = logger[0]
	} else {
		log = noopLogProvider{}
	}

	concurrency := cfg.GetAIVerifierMaxConcurrency()
	if concurrency <= 0 {
		concurrency = 5
	}

	return &RetryVerifier{
		inner:      inner,
		maxRetries: 3,
		sem:        make(chan struct{}, concurrency),
		logger:     log,
		metrics:    observability.NoopDiscoverabilityMetrics(),
	}
}

// SetMetrics attaches a DiscoverabilityMetrics recorder. A nil value is
// normalised to the noop recorder so call sites need no nil checks.
// Intended for bootstrap-time wiring; not safe to call mid-request.
func (r *RetryVerifier) SetMetrics(m observability.DiscoverabilityMetrics) {
	if m == nil {
		m = observability.NoopDiscoverabilityMetrics()
	}
	r.metrics = m
}

// Verify submits req to the inner Verifier, retrying on transient errors.
// It acquires a semaphore slot before calling the inner provider; if the
// context is cancelled while waiting, it returns ErrVerifierTimeout.
func (r *RetryVerifier) Verify(ctx context.Context, req Request) (Response, error) {
	// Acquire a concurrency slot; block until one is free or ctx is done.
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return Response{}, &TimeoutError{
			Provider: "retry_verifier",
			Message:  ctx.Err().Error(),
		}
	}
	defer func() { <-r.sem }()

	var lastErr error

	for attempt := 0; attempt < r.maxRetries; attempt++ {
		resp, err := r.inner.Verify(ctx, req)
		if err == nil {
			r.metrics.IncVerifyVerdict(resp.Verdict)
			return resp, nil
		}

		lastErr = err

		// Stop if the caller cancelled the context.
		if ctx.Err() != nil {
			break
		}

		// Non-retryable error — fail immediately, preserving the original type.
		if !r.shouldRetry(err) {
			break
		}

		// Last attempt — no need to sleep before giving up.
		if attempt == r.maxRetries-1 {
			break
		}

		// On 429, honour Retry-After when the provider supplied it.
		var rateErr *RateLimitError
		if errors.As(err, &rateErr) && rateErr.RetryAfter > 0 {
			wait := time.Duration(rateErr.RetryAfter) * time.Second
			r.logger.Warn("verifier rate limited; honouring Retry-After",
				observability.Int("retry_after_seconds", rateErr.RetryAfter),
				observability.Int("attempt", attempt+1),
			)
			select {
			case <-ctx.Done():
				return Response{}, lastErr
			case <-time.After(wait):
			}
			continue
		}

		r.logger.Warn("verifier transient error; retrying",
			observability.String("error", err.Error()),
			observability.Int("attempt", attempt+1),
		)
	}

	r.metrics.IncVerifyVerdict("error")
	// Return lastErr unchanged so the original sentinel class is preserved.
	return Response{}, lastErr
}

// shouldRetry reports whether err warrants another attempt.
// ErrVerifierMalformedResponse is not retryable — the response shape is wrong,
// not the network or the server; retrying would return the same malformed data.
func (r *RetryVerifier) shouldRetry(err error) bool {
	return err != nil && (errors.Is(err, ErrVerifierTimeout) ||
		errors.Is(err, ErrVerifierProvider) ||
		errors.Is(err, ErrVerifierRateLimit))
}

// noopLogProvider discards all log output. Used when no logger is supplied
// to NewRetryVerifier so that call sites never need nil checks.
type noopLogProvider struct{}

func (noopLogProvider) Info(string, ...observability.LogAttr)         {}
func (noopLogProvider) Error(string, error, ...observability.LogAttr) {}
func (noopLogProvider) Warn(string, ...observability.LogAttr)         {}
func (noopLogProvider) Debug(string, ...observability.LogAttr)        {}
func (noopLogProvider) With(...observability.LogAttr) observability.LogProvider {
	return noopLogProvider{}
}
