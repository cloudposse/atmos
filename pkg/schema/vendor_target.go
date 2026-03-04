package schema

import (
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Sentinel errors for vendor target validation.
var (
	// ErrVendorTargetInvalidFormat is returned when a vendor target has an invalid format.
	ErrVendorTargetInvalidFormat = errors.New("invalid vendor target format")
	// ErrVendorTargetMissingPath is returned when a vendor target map is missing the required 'path' field.
	ErrVendorTargetMissingPath = errors.New("vendor target missing required 'path' field")
	// ErrVendorTargetUnexpectedNodeKind is returned when a vendor target node has an unexpected kind.
	ErrVendorTargetUnexpectedNodeKind = errors.New("unexpected vendor target node kind")
)

// AtmosVendorTarget represents a single vendor target that can be specified
// as either a simple string (path only) or a map with path and optional version override.
type AtmosVendorTarget struct {
	// Path is the target directory path, supporting Go template syntax.
	Path string `yaml:"path" json:"path" mapstructure:"path"`
	// Version is an optional version override for this specific target.
	// When set, it overrides the source-level Version for both the source URL
	// template and the target path template.
	Version string `yaml:"version,omitempty" json:"version,omitempty" mapstructure:"version"`
}

// AtmosVendorTargets is a slice of AtmosVendorTarget that supports flexible YAML unmarshaling.
// It can parse both simple string syntax and structured syntax:
//
// Simple syntax (backward compatible):
//
//	targets:
//	  - "components/terraform/vpc"
//	  - "components/terraform/{{.Component}}/{{.Version}}"
//
// Structured syntax (per-target version override):
//
//	targets:
//	  - path: "vpc/{{.Version}}"
//	    version: "2.1.0"
//	  - path: "vpc/latest"
//	    version: "2.2.0"
//
// Mixed syntax:
//
//	targets:
//	  - "components/terraform/vpc"
//	  - path: "vpc/{{.Version}}"
//	    version: "2.1.0"
type AtmosVendorTargets []AtmosVendorTarget

// UnmarshalYAML implements custom YAML unmarshaling for AtmosVendorTargets.
// It handles both string elements (simple path) and mapping elements (path + optional version).
func (t *AtmosVendorTargets) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("%w: expected sequence, got %v", ErrVendorTargetInvalidFormat, value.Kind)
	}

	targets := make([]AtmosVendorTarget, 0, len(value.Content))
	for i, node := range value.Content {
		var target AtmosVendorTarget
		switch node.Kind {
		case yaml.ScalarNode:
			// Simple string syntax: "vpc/{{.Version}}" -> path only, no version override.
			target.Path = node.Value
		case yaml.MappingNode:
			// Structured syntax: {path: "vpc/{{.Version}}", version: "2.1.0"}.
			if err := node.Decode(&target); err != nil {
				return fmt.Errorf("failed to decode vendor target at index %d: %w", i, err)
			}
			if target.Path == "" {
				return fmt.Errorf("%w at index %d", ErrVendorTargetMissingPath, i)
			}
		default:
			return fmt.Errorf("%w at index %d: got %v (expected string or mapping)",
				ErrVendorTargetUnexpectedNodeKind, i, node.Kind)
		}
		targets = append(targets, target)
	}
	*t = targets
	return nil
}
