package skills

import (
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Registry manages available skills.
type Registry struct {
	skills map[string]*Skill
	mu     sync.RWMutex
}

// NewRegistry creates a new skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
	}
}

// Register adds a skill to the registry.
func (r *Registry) Register(skill *Skill) error {
	if skill == nil {
		return errUtils.ErrAISkillNil
	}

	if skill.Name == "" {
		return errUtils.ErrAISkillNameEmpty
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[skill.Name]; exists {
		return fmt.Errorf("%w: %s", errUtils.ErrAISkillAlreadyRegistered, skill.Name)
	}

	r.skills[skill.Name] = skill
	return nil
}

// Get retrieves a skill by name.
func (r *Registry) Get(name string) (*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.skills[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAISkillNotFound, name)
	}

	return skill, nil
}

// List returns all registered skills sorted alphabetically by name.
func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// ListByCategory returns skills in a specific category sorted alphabetically by name.
func (r *Registry) ListByCategory(category string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []*Skill
	for _, skill := range r.skills {
		if skill.Category == category {
			skills = append(skills, skill)
		}
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// ListBuiltIn returns only built-in skills sorted alphabetically by name.
func (r *Registry) ListBuiltIn() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []*Skill
	for _, skill := range r.skills {
		if skill.IsBuiltIn {
			skills = append(skills, skill)
		}
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// ListCustom returns only custom (user-defined) skills sorted alphabetically by name.
func (r *Registry) ListCustom() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []*Skill
	for _, skill := range r.skills {
		if !skill.IsBuiltIn {
			skills = append(skills, skill)
		}
	}

	// Sort alphabetically by name for consistent ordering.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// Unregister removes a skill from the registry.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; !exists {
		return fmt.Errorf("%w: %s", errUtils.ErrAISkillNotFound, name)
	}

	delete(r.skills, name)
	return nil
}

// Count returns the number of registered skills.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.skills)
}

// Has checks if a skill exists.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.skills[name]
	return exists
}
