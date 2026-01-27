package skills

import (
	"fmt"
	"sort"
	"strings"
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

// ToPromptXML generates an XML block listing available skills for injection into the system prompt.
// This follows the Agent Skills integration guide recommendation for Claude models.
// The XML format helps the model understand what skills are available and when to use them.
func (r *Registry) ToPromptXML(currentSkillName string) string {
	skills := r.List()
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	sb.WriteString("  <current_skill>")
	sb.WriteString(currentSkillName)
	sb.WriteString("</current_skill>\n")
	sb.WriteString("  <skills>\n")

	for _, skill := range skills {
		sb.WriteString("    <skill>\n")
		sb.WriteString("      <name>")
		sb.WriteString(skill.Name)
		sb.WriteString("</name>\n")
		sb.WriteString("      <description>")
		sb.WriteString(skill.Description)
		sb.WriteString("</description>\n")
		if skill.Category != "" {
			sb.WriteString("      <category>")
			sb.WriteString(skill.Category)
			sb.WriteString("</category>\n")
		}
		sb.WriteString("    </skill>\n")
	}

	sb.WriteString("  </skills>\n")
	sb.WriteString("  <note>The user can switch skills using Ctrl+A in the TUI. Each skill provides specialized expertise.</note>\n")
	sb.WriteString("</available_skills>")

	return sb.String()
}
