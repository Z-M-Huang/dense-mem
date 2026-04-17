package registry

import (
	"sync"
)

// MockRegistry is a test double for Registry.
type MockRegistry struct {
	mu           sync.Mutex
	RegisterFunc func(tool Tool) error
	GetFunc      func(name string) (Tool, bool)
	ListFunc     func() []Tool
}

var _ Registry = (*MockRegistry)(nil)

// Register invokes RegisterFunc when set.
func (m *MockRegistry) Register(tool Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RegisterFunc != nil {
		return m.RegisterFunc(tool)
	}
	return nil
}

// Get invokes GetFunc when set.
func (m *MockRegistry) Get(name string) (Tool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetFunc != nil {
		return m.GetFunc(name)
	}
	return Tool{}, false
}

// List invokes ListFunc when set.
func (m *MockRegistry) List() []Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ListFunc != nil {
		return m.ListFunc()
	}
	return nil
}
