package fragmentservice

import (
	"context"
	"sync"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/http/dto"
)

// MockCreate is a test double for CreateFragmentService. Configure behavior via
// CreateFunc; otherwise Create returns a nil result and nil error.
type MockCreate struct {
	mu         sync.Mutex
	CreateFunc func(ctx context.Context, profileID string, req *dto.CreateFragmentRequest) (*CreateResult, error)
	CallCount  int
}

var _ CreateFragmentService = (*MockCreate)(nil)

// Create invokes CreateFunc when set.
func (m *MockCreate) Create(ctx context.Context, profileID string, req *dto.CreateFragmentRequest) (*CreateResult, error) {
	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, profileID, req)
	}
	return nil, nil
}

// MockGet is a test double for GetFragmentService.
type MockGet struct {
	mu        sync.Mutex
	GetFunc   func(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error)
	CallCount int
}

var _ GetFragmentService = (*MockGet)(nil)

// GetByID invokes GetFunc when set.
func (m *MockGet) GetByID(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error) {
	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()
	if m.GetFunc != nil {
		return m.GetFunc(ctx, profileID, fragmentID)
	}
	return nil, ErrFragmentNotFound
}

// MockList is a test double for ListFragmentsService.
type MockList struct {
	mu        sync.Mutex
	ListFunc  func(ctx context.Context, profileID string, opts ListOptions) ([]domain.Fragment, string, error)
	CallCount int
}

var _ ListFragmentsService = (*MockList)(nil)

// List invokes ListFunc when set.
func (m *MockList) List(ctx context.Context, profileID string, opts ListOptions) ([]domain.Fragment, string, error) {
	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()
	if m.ListFunc != nil {
		return m.ListFunc(ctx, profileID, opts)
	}
	return nil, "", nil
}

// MockDelete is a test double for DeleteFragmentService.
type MockDelete struct {
	mu         sync.Mutex
	DeleteFunc func(ctx context.Context, profileID, fragmentID string) error
	CallCount  int
}

var _ DeleteFragmentService = (*MockDelete)(nil)

// Delete invokes DeleteFunc when set.
func (m *MockDelete) Delete(ctx context.Context, profileID, fragmentID string) error {
	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, profileID, fragmentID)
	}
	return nil
}
