package agents

import (
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Registry manages available agents.
type Registry struct {
	agents map[string]*Agent
	mu     sync.RWMutex
}

// NewRegistry creates a new agent registry.
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*Agent),
	}
}

// Register adds an agent to the registry.
func (r *Registry) Register(agent *Agent) error {
	if agent == nil {
		return errUtils.ErrAIAgentNil
	}

	if agent.Name == "" {
		return errUtils.ErrAIAgentNameEmpty
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agent.Name]; exists {
		return fmt.Errorf("%w: %s", errUtils.ErrAIAgentAlreadyRegistered, agent.Name)
	}

	r.agents[agent.Name] = agent
	return nil
}

// Get retrieves an agent by name.
func (r *Registry) Get(name string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIAgentNotFound, name)
	}

	return agent, nil
}

// List returns all registered agents sorted alphabetically by name.
func (r *Registry) List() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// ListByCategory returns agents in a specific category sorted alphabetically by name.
func (r *Registry) ListByCategory(category string) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []*Agent
	for _, agent := range r.agents {
		if agent.Category == category {
			agents = append(agents, agent)
		}
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// ListBuiltIn returns only built-in agents sorted alphabetically by name.
func (r *Registry) ListBuiltIn() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []*Agent
	for _, agent := range r.agents {
		if agent.IsBuiltIn {
			agents = append(agents, agent)
		}
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// ListCustom returns only custom (user-defined) agents sorted alphabetically by name.
func (r *Registry) ListCustom() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []*Agent
	for _, agent := range r.agents {
		if !agent.IsBuiltIn {
			agents = append(agents, agent)
		}
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// Unregister removes an agent from the registry.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[name]; !exists {
		return fmt.Errorf("%w: %s", errUtils.ErrAIAgentNotFound, name)
	}

	delete(r.agents, name)
	return nil
}

// Count returns the number of registered agents.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.agents)
}

// Has checks if an agent exists.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.agents[name]
	return exists
}
