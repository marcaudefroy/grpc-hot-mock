package mocks

import (
	"sync"
)

type Registry interface {
	RegisterMock(MockConfig)
	GetMock(fullMethod string) (MockConfig, bool)
}

type DefaultRegistry struct {
	mockRegistry   map[string]MockConfig
	mockRegistryMu sync.RWMutex
}

func (r *DefaultRegistry) RegisterMock(mc MockConfig) {
	full := "/" + mc.Service + "/" + mc.Method
	r.mockRegistryMu.Lock()
	if r.mockRegistry == nil {
		r.mockRegistry = map[string]MockConfig{}
	}
	r.mockRegistry[full] = mc
	r.mockRegistryMu.Unlock()
}

func (r *DefaultRegistry) GetMock(fullMethod string) (MockConfig, bool) {
	r.mockRegistryMu.RLock()
	defer r.mockRegistryMu.RUnlock()
	mc, ok := r.mockRegistry[fullMethod]
	return mc, ok
}
