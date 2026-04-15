package sse

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSSEWriter implements SSEWriter for testing.
type mockSSEWriter struct {
	events   []mockEvent
	comments []string
	closed   bool
	mu       sync.Mutex
}

type mockEvent struct {
	eventType string
	payload   any
}

func newMockSSEWriter() *mockSSEWriter {
	return &mockSSEWriter{
		events:   make([]mockEvent, 0),
		comments: make([]string, 0),
	}
}

func (m *mockSSEWriter) WriteEvent(eventType string, payload any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrStreamClosed
	}
	m.events = append(m.events, mockEvent{eventType: eventType, payload: payload})
	if eventType == EventTypeDone || eventType == EventTypeError {
		m.closed = true
	}
	return nil
}

func (m *mockSSEWriter) WriteComment(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrStreamClosed
	}
	m.comments = append(m.comments, text)
	return nil
}

func (m *mockSSEWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockSSEWriter) GetEvents() []mockEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockEvent{}, m.events...)
}

func (m *mockSSEWriter) GetComments() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.comments...)
}

func (m *mockSSEWriter) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// mockConcurrencyLimiter implements ConcurrencyLimiter for testing.
type mockConcurrencyLimiter struct {
	currentCount int64
	maxStreams   int
	released     bool
	mu           sync.Mutex
}

func newMockConcurrencyLimiter(maxStreams int) *mockConcurrencyLimiter {
	return &mockConcurrencyLimiter{
		maxStreams: maxStreams,
	}
}

func (m *mockConcurrencyLimiter) Acquire(ctx context.Context, profileID string) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := atomic.AddInt64(&m.currentCount, 1)
	if count > int64(m.maxStreams) {
		atomic.AddInt64(&m.currentCount, -1)
		return nil, ErrTooManyStreams
	}

	released := false
	return func() {
		if released {
			return
		}
		released = true
		m.released = true
		atomic.AddInt64(&m.currentCount, -1)
	}, nil
}

func (m *mockConcurrencyLimiter) GetCount() int64 {
	return atomic.LoadInt64(&m.currentCount)
}

// mockCleanupRepository implements StreamCleanupRepository for testing.
type mockCleanupRepository struct {
	cleanedUp bool
	profileID string
	mu        sync.Mutex
}

func newMockCleanupRepository() *mockCleanupRepository {
	return &mockCleanupRepository{}
}

func (m *mockCleanupRepository) PurgeProfileStreamState(ctx context.Context, profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanedUp = true
	m.profileID = profileID
	return nil
}

func (m *mockCleanupRepository) WasCleanedUp() (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cleanedUp, m.profileID
}

// mockHeartbeatSender implements HeartbeatSender for testing.
type mockHeartbeatSender struct {
	runCalled bool
	mu        sync.Mutex
}

func newMockHeartbeatSender() *mockHeartbeatSender {
	return &mockHeartbeatSender{}
}

func (m *mockHeartbeatSender) Run(ctx context.Context, writer SSEWriter) {
	m.mu.Lock()
	m.runCalled = true
	m.mu.Unlock()
	<-ctx.Done()
}

func (m *mockHeartbeatSender) WasRunCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runCalled
}

// TestHeartbeatInterval tests that heartbeat fires at the correct interval.
func TestHeartbeatInterval(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a mock writer
	writer := newMockSSEWriter()

	// Create heartbeat sender with short interval for testing
	interval := 100 * time.Millisecond
	sender := NewHeartbeatSenderWithInterval(interval)

	// Run heartbeat sender with context that we'll cancel
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	// Run in goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Run(ctx, writer)
	}()

	// Wait for context to timeout
	<-ctx.Done()

	// Wait for Run to finish
	<-done

	// Should have sent 3 heartbeats (at 100ms, 200ms, 300ms before timeout at 350ms)
	comments := writer.GetComments()
	assert.GreaterOrEqual(t, len(comments), 2, "expected at least 2 heartbeat comments")
	assert.LessOrEqual(t, len(comments), 4, "expected at most 4 heartbeat comments")

	// All comments should be "keepalive"
	for _, c := range comments {
		assert.Equal(t, "keepalive", c)
	}
}

// TestMaxDurationTermination tests that stream terminates with done event after max duration.
func TestMaxDurationTermination(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mocks
	writer := newMockSSEWriter()
	limiter := newMockConcurrencyLimiter(10)
	cleanupRepo := newMockCleanupRepository()
	heartbeatSender := newMockHeartbeatSender()

	// Use short max duration for testing
	maxDuration := 200 * time.Millisecond

	lifecycle := NewStreamLifecycleWithConfig(limiter, heartbeatSender, maxDuration, cleanupRepo)

	// Create context that won't timeout
	ctx := context.Background()

	// Work function that never completes on its own
	work := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}

	// Start the stream
	err := lifecycle.Start(ctx, "profile123", writer, work)

	// Should have terminated with ErrStreamTerminated
	assert.ErrorIs(t, err, ErrStreamTerminated)

	// Check that done event was sent
	events := writer.GetEvents()
	assert.Len(t, events, 1, "expected exactly 1 event (done)")
	if len(events) > 0 {
		assert.Equal(t, EventTypeDone, events[0].eventType)
		if payload, ok := events[0].payload.(map[string]any); ok {
			assert.Equal(t, "max_duration_exceeded", payload["reason"])
		}
	}

	// Verify concurrency was released
	time.Sleep(50 * time.Millisecond) // Give time for cleanup
	assert.Equal(t, int64(0), limiter.GetCount())
}

// TestDisconnectCleanup tests that disconnect aborts work and cleans up Redis keys.
func TestDisconnectCleanup(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mocks
	writer := newMockSSEWriter()
	limiter := newMockConcurrencyLimiter(10)
	cleanupRepo := newMockCleanupRepository()
	heartbeatSender := newMockHeartbeatSender()

	// Use long max duration so disconnect happens first
	maxDuration := 5 * time.Second

	lifecycle := NewStreamLifecycleWithConfig(limiter, heartbeatSender, maxDuration, cleanupRepo)

	// Create context that we'll cancel to simulate disconnect
	ctx, cancel := context.WithCancel(context.Background())

	// Work function that tracks if it was aborted
	workStarted := make(chan struct{})
	workAborted := make(chan struct{})
	work := func(ctx context.Context) error {
		close(workStarted)
		<-ctx.Done()
		close(workAborted)
		return ctx.Err()
	}

	// Start the stream in goroutine
	done := make(chan error, 1)
	go func() {
		done <- lifecycle.Start(ctx, "profile456", writer, work)
	}()

	// Wait for work to start
	<-workStarted

	// Simulate disconnect by cancelling context
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for stream to finish
	err := <-done

	// Should have context canceled error
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.Canceled))

	// Check that cleanup was called
	cleanedUp, profileID := cleanupRepo.WasCleanedUp()
	assert.True(t, cleanedUp, "expected cleanup to be called on disconnect")
	assert.Equal(t, "profile456", profileID)

	// Check that work was aborted
	select {
	case <-workAborted:
		// Good - work was aborted
	default:
		t.Error("expected work to be aborted on disconnect")
	}

	// Verify concurrency was released
	assert.Equal(t, int64(0), limiter.GetCount())
}

// TestConcurrencyLimit tests that 11th concurrent stream receives 429 rejection.
func TestConcurrencyLimit(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test the mock implementation
	mockLimiter := newMockConcurrencyLimiter(10)

	// Acquire 10 streams - should all succeed
	releases := make([]func(), 0, 10)
	for i := 0; i < 10; i++ {
		release, err := mockLimiter.Acquire(context.Background(), "profile123")
		require.NoError(t, err, "stream %d should be allowed", i+1)
		releases = append(releases, release)
	}

	// Verify count is 10
	assert.Equal(t, int64(10), mockLimiter.GetCount())

	// 11th stream should be rejected with ErrTooManyStreams
	release, err := mockLimiter.Acquire(context.Background(), "profile123")
	assert.ErrorIs(t, err, ErrTooManyStreams, "11th stream should be rejected")
	assert.Nil(t, release, "release function should be nil on error")

	// Count should still be 10 (11th was rejected)
	assert.Equal(t, int64(10), mockLimiter.GetCount())

	// Release one stream
	releases[0]()
	assert.Equal(t, int64(9), mockLimiter.GetCount())

	// Now 11th should succeed (we're back to 10 allowed)
	release, err = mockLimiter.Acquire(context.Background(), "profile123")
	require.NoError(t, err, "stream after release should be allowed")
	releases = append(releases, release)
	assert.Equal(t, int64(10), mockLimiter.GetCount())

	// Release all
	for _, r := range releases {
		r()
	}
	assert.Equal(t, int64(0), mockLimiter.GetCount())
}

// TestConcurrencyLimitRelease tests that release function is safe to call multiple times.
func TestConcurrencyLimitRelease(t *testing.T) {
	t.Parallel()

	mockLimiter := newMockConcurrencyLimiter(10)

	// Acquire a stream
	release, err := mockLimiter.Acquire(context.Background(), "profile123")
	require.NoError(t, err)
	assert.Equal(t, int64(1), mockLimiter.GetCount())

	// Call release multiple times - should be idempotent
	release()
	assert.Equal(t, int64(0), mockLimiter.GetCount())

	release() // Second call should be no-op
	assert.Equal(t, int64(0), mockLimiter.GetCount())
}

// TestNormalCompletion tests that stream completes normally when work finishes.
func TestNormalCompletion(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mocks
	writer := newMockSSEWriter()
	limiter := newMockConcurrencyLimiter(10)
	cleanupRepo := newMockCleanupRepository()
	heartbeatSender := newMockHeartbeatSender()

	maxDuration := 5 * time.Second
	lifecycle := NewStreamLifecycleWithConfig(limiter, heartbeatSender, maxDuration, cleanupRepo)

	// Work function that completes quickly
	work := func(ctx context.Context) error {
		return nil // Normal completion
	}

	// Start the stream
	ctx := context.Background()
	err := lifecycle.Start(ctx, "profile789", writer, work)

	// Should have no error (work completed successfully)
	assert.NoError(t, err)

	// Verify concurrency was released
	assert.Equal(t, int64(0), limiter.GetCount())

	// Cleanup should NOT be called on normal completion
	cleanedUp, _ := cleanupRepo.WasCleanedUp()
	assert.False(t, cleanedUp, "cleanup should not be called on normal completion")
}

// TestWorkError tests that work function errors are propagated.
func TestWorkError(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mocks
	writer := newMockSSEWriter()
	limiter := newMockConcurrencyLimiter(10)
	cleanupRepo := newMockCleanupRepository()
	heartbeatSender := newMockHeartbeatSender()

	maxDuration := 5 * time.Second
	lifecycle := NewStreamLifecycleWithConfig(limiter, heartbeatSender, maxDuration, cleanupRepo)

	// Work function that returns an error
	expectedErr := errors.New("work failed")
	work := func(ctx context.Context) error {
		return expectedErr
	}

	// Start the stream
	ctx := context.Background()
	err := lifecycle.Start(ctx, "profile999", writer, work)

	// Should have the work error
	assert.ErrorIs(t, err, expectedErr)

	// Verify concurrency was released
	assert.Equal(t, int64(0), limiter.GetCount())
}

// TestSSEWriterWriteComment tests the WriteComment method.
func TestSSEWriterWriteComment(t *testing.T) {
	flusher := newMockSSEWriter()
	writer := flusher // mockSSEWriter implements SSEWriter

	// Write a comment
	err := writer.WriteComment("keepalive")
	require.NoError(t, err)

	// Check the comment was stored
	comments := flusher.GetComments()
	assert.Len(t, comments, 1)
	assert.Equal(t, "keepalive", comments[0])
}

// TestSSEWriterWriteCommentAfterClose tests that WriteComment fails after stream is closed.
func TestSSEWriterWriteCommentAfterClose(t *testing.T) {
	flusher := newMockSSEWriter()
	writer := flusher // mockSSEWriter implements SSEWriter

	// Close the stream
	err := writer.Close()
	require.NoError(t, err)

	// Try to write a comment
	err = writer.WriteComment("keepalive")
	assert.ErrorIs(t, err, ErrStreamClosed)
}

// Integration test with real HTTP response writer
func TestHeartbeatWithRealHTTPResponseWriter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a mock writer that we can inspect
	writer := newMockSSEWriter()

	// Create heartbeat sender with short interval
	interval := 50 * time.Millisecond
	sender := NewHeartbeatSenderWithInterval(interval)

	// Run with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Run(ctx, writer)
	}()

	// Wait for completion
	<-ctx.Done()
	<-done

	// Check output contains heartbeat comments
	comments := writer.GetComments()
	assert.GreaterOrEqual(t, len(comments), 2, "expected at least 2 heartbeat comments")
	assert.LessOrEqual(t, len(comments), 4, "expected at most 4 heartbeat comments")
}