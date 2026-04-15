package inmem

import (
	"context"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/sse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryConcurrencyLimiter_RejectsEleventhAcquire(t *testing.T) {
	t.Parallel()

	limiter := NewInMemoryConcurrencyLimiter(10, time.Hour)
	releases := make([]func(), 0, 10)

	for i := 0; i < 10; i++ {
		release, err := limiter.Acquire(context.Background(), "profile-1")
		require.NoError(t, err)
		releases = append(releases, release)
	}

	release, err := limiter.Acquire(context.Background(), "profile-1")
	assert.ErrorIs(t, err, sse.ErrTooManyStreams)
	assert.Nil(t, release)

	for _, r := range releases {
		r()
	}
}

func TestInMemoryConcurrencyLimiter_DoubleRelease_NoNegative(t *testing.T) {
	t.Parallel()

	limiter := NewInMemoryConcurrencyLimiter(1, time.Hour)
	release, err := limiter.Acquire(context.Background(), "profile-1")
	require.NoError(t, err)

	release()
	release() // must be no-op

	// After double release the entry should be deleted, so we can't
	// dereference it. Instead verify the map no longer contains it.
	limiter.mu.Lock()
	_, exists := limiter.entries["profile-1"]
	limiter.mu.Unlock()
	assert.False(t, exists, "entry should be removed after count reaches zero")
}

func TestInMemoryConcurrencyLimiter_ExpiryResetsCapacity(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	limiter := NewInMemoryConcurrencyLimiterWithClock(2, time.Hour, clock)

	// Fill both slots.
	release1, err := limiter.Acquire(context.Background(), "profile-1")
	require.NoError(t, err)
	release2, err := limiter.Acquire(context.Background(), "profile-1")
	require.NoError(t, err)

	// Third should be rejected.
	release3, err := limiter.Acquire(context.Background(), "profile-1")
	assert.ErrorIs(t, err, sse.ErrTooManyStreams)
	assert.Nil(t, release3)

	// Advance clock past the TTL.
	now = now.Add(2 * time.Hour)

	// Next acquire should succeed because the expired entry is treated as absent.
	release3, err = limiter.Acquire(context.Background(), "profile-1")
	require.NoError(t, err)

	// Clean up.
	release1()
	release2()
	release3()
}

func TestInMemoryConcurrencyLimiter_DifferentProfilesIndependent(t *testing.T) {
	t.Parallel()

	limiter := NewInMemoryConcurrencyLimiter(1, time.Hour)

	// Acquire slot for profile-1.
	release1, err := limiter.Acquire(context.Background(), "profile-1")
	require.NoError(t, err)

	// profile-2 should still have its own slot available.
	release2, err := limiter.Acquire(context.Background(), "profile-2")
	require.NoError(t, err)

	// Second acquire for profile-1 should fail.
	release1b, err := limiter.Acquire(context.Background(), "profile-1")
	assert.ErrorIs(t, err, sse.ErrTooManyStreams)
	assert.Nil(t, release1b)

	release1()
	release2()
}
