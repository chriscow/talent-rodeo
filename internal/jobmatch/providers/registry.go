package providers

import (
	"fmt"
	"sync"

	"talent-rodeo/internal/jobmatch/types"
)

// Registry manages the available job providers
type Registry struct {
	providers map[string]types.JobProvider
	mu        sync.RWMutex
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]types.JobProvider),
	}
}

// Register adds a new provider to the registry
func (r *Registry) Register(provider types.JobProvider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %s already registered", name)
	}

	r.providers[name] = provider
	return nil
}

// Get returns a provider by name
func (r *Registry) Get(name string) (types.JobProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}

	return provider, nil
}

// List returns all registered providers
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Unregister removes a provider from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; !exists {
		return fmt.Errorf("provider %s not found", name)
	}

	delete(r.providers, name)
	return nil
}
