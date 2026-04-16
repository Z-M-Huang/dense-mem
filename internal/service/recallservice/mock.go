package recallservice

import (
	"context"
	"sync"
)

// MockRecall is a test double for RecallService.
type MockRecall struct {
	mu         sync.Mutex
	RecallFunc func(ctx context.Context, profileID string, req RecallRequest) ([]RecallHit, error)
	CallCount  int
}

var _ RecallService = (*MockRecall)(nil)

// Recall invokes RecallFunc when set.
func (m *MockRecall) Recall(ctx context.Context, profileID string, req RecallRequest) ([]RecallHit, error) {
	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()
	if m.RecallFunc != nil {
		return m.RecallFunc(ctx, profileID, req)
	}
	return nil, nil
}
