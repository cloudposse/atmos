// Package rules provides all built-in lint rule implementations for atmos lint stacks.
// Each rule is in a separate file named lNN_<description>.go where NN is the rule number.
// This file contains shared utility functions used by multiple rule implementations
// to avoid code duplication.
package rules

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// extractInherits returns the list of parent component names from a metadata section.
func extractInherits(metadata map[string]any) []string {
	raw, ok := metadata[cfg.InheritsSectionName]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// appendIfMissing appends s to slice only if s is not already present.
func appendIfMissing(slice []string, s string) []string {
	if u.SliceContainsString(slice, s) {
		return slice
	}
	return append(slice, s)
}

// getNestedMap traverses a nested map[string]any using the provided key path and
// returns the inner map at that path. Returns (nil, false) if any key is missing
// or the value is not a map.
func getNestedMap(m map[string]any, keys ...string) (map[string]any, bool) {
	current := m
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
