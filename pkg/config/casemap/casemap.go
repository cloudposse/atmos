// Package casemap provides utilities for preserving original case of YAML map keys.
// Viper automatically lowercases all YAML map keys, which breaks case-sensitive
// configurations like environment variables (e.g., GITHUB_TOKEN becomes github_token).
// This package extracts original case from raw YAML before Viper processes it,
// allowing restoration of original key casing when needed.
package casemap

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CaseMap stores lowercase -> original case mappings for YAML keys.
// Use this to restore original case after Viper lowercases map keys.
type CaseMap map[string]string

// CaseMaps holds all case mappings for different config paths.
type CaseMaps struct {
	maps map[string]CaseMap // path -> case map (e.g., "env", "auth.identities")
}

// New creates a new CaseMaps instance.
func New() *CaseMaps {
	return &CaseMaps{maps: make(map[string]CaseMap)}
}

// Set stores a case map for a given path.
func (c *CaseMaps) Set(path string, m CaseMap) {
	c.maps[path] = m
}

// Get retrieves the case map for a given path.
func (c *CaseMaps) Get(path string) CaseMap {
	if c == nil {
		return nil
	}
	return c.maps[path]
}

// ApplyCase returns a new map with keys converted to their original case.
// Keys not in the case map are returned unchanged.
func (c *CaseMaps) ApplyCase(path string, lowercased map[string]string) map[string]string {
	if c == nil {
		return lowercased
	}

	caseMap := c.maps[path]
	if caseMap == nil {
		return lowercased
	}

	result := make(map[string]string, len(lowercased))
	for lowerKey, value := range lowercased {
		originalKey := lowerKey
		if original, ok := caseMap[lowerKey]; ok {
			originalKey = original
		}
		result[originalKey] = value
	}
	return result
}

// ExtractFromYAML extracts case mappings from raw YAML for specified paths.
// Paths is a list of dot-separated paths, e.g., ["env", "auth.identities"].
func ExtractFromYAML(rawYAML []byte, paths []string) (*CaseMaps, error) {
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(rawYAML, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	caseMaps := New()
	for _, path := range paths {
		section := navigateToPath(rawConfig, path)
		if section == nil {
			continue
		}

		caseMap := make(CaseMap)
		for originalName := range section {
			lowercaseName := strings.ToLower(originalName)
			caseMap[lowercaseName] = originalName
		}
		caseMaps.Set(path, caseMap)
	}

	return caseMaps, nil
}

// navigateToPath traverses a nested map using dot notation.
func navigateToPath(m map[string]interface{}, path string) map[string]interface{} {
	parts := strings.Split(path, ".")
	current := m
	for _, part := range parts {
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}
