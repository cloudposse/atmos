package theme

import (
	"errors"
	"fmt"
	"strings"
)

// ErrThemeNotFound is returned when a requested theme is not found.
var ErrThemeNotFound = errors.New("theme not found")

// ErrInvalidTheme is returned when an invalid theme is specified.
var ErrInvalidTheme = errors.New("invalid theme")

// Registry manages the collection of available themes.
type Registry struct {
	themes map[string]*Theme
	sorted []*Theme
}

// NewRegistry creates a new theme registry and loads all themes.
func NewRegistry() (*Registry, error) {
	themes, err := LoadThemes()
	if err != nil {
		return nil, fmt.Errorf("failed to load themes: %w", err)
	}

	r := &Registry{
		themes: make(map[string]*Theme),
		sorted: themes,
	}

	// Build the map for quick lookups (case-insensitive)
	for _, t := range themes {
		r.themes[strings.ToLower(t.Name)] = t
	}

	// Sort the themes for consistent listing
	SortThemes(r.sorted)

	// Ensure default theme exists
	if _, exists := r.Get("default"); !exists {
		return nil, fmt.Errorf("%w: theme 'default' not found", ErrThemeNotFound)
	}

	return r, nil
}

// Get returns a theme by name (case-insensitive).
func (r *Registry) Get(name string) (*Theme, bool) {
	theme, exists := r.themes[strings.ToLower(name)]
	return theme, exists
}

// GetOrDefault returns a theme by name, falling back to default if not found.
func (r *Registry) GetOrDefault(name string) *Theme {
	if theme, exists := r.Get(name); exists {
		return theme
	}
	// Return default theme - we know it exists from NewRegistry validation
	defaultTheme, _ := r.Get("default")
	return defaultTheme
}

// List returns all themes sorted alphabetically.
func (r *Registry) List() []*Theme {
	return r.sorted
}

// ListRecommended returns only the recommended themes.
func (r *Registry) ListRecommended() []*Theme {
	return FilterRecommended(r.sorted)
}

// Search returns themes matching the given query (case-insensitive partial match).
func (r *Registry) Search(query string) []*Theme {
	if query == "" {
		return r.sorted
	}

	query = strings.ToLower(query)
	var matches []*Theme
	for _, t := range r.sorted {
		if strings.Contains(strings.ToLower(t.Name), query) {
			matches = append(matches, t)
		}
	}
	return matches
}

// Count returns the total number of themes in the registry.
func (r *Registry) Count() int {
	return len(r.themes)
}

// CountRecommended returns the number of recommended themes.
func (r *Registry) CountRecommended() int {
	count := 0
	for _, themeName := range RecommendedThemes {
		if _, exists := r.Get(themeName); exists {
			count++
		}
	}
	return count
}

// ValidateTheme checks if a theme name is valid.
// Returns nil if the theme is valid or empty (will use default).
// Returns an error with available themes if the theme is invalid.
func ValidateTheme(themeName string) error {
	if themeName == "" {
		return nil // Empty is valid, will use default
	}

	registry, err := NewRegistry()
	if err != nil {
		return fmt.Errorf("failed to load theme registry: %w", err)
	}

	_, exists := registry.Get(themeName)
	if !exists {
		availableThemes := make([]string, 0, len(registry.sorted))
		for _, t := range registry.sorted {
			availableThemes = append(availableThemes, t.Name)
		}
		return fmt.Errorf("%w: '%s'. Available themes: %s",
			ErrInvalidTheme, themeName, strings.Join(availableThemes, ", "))
	}

	return nil
}
