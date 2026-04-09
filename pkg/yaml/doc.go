// Package yaml provides YAML parsing, caching, and utility functions for Atmos.
//
// This package contains YAML-specific functionality including:
//   - Parsing and unmarshaling YAML with custom tag processing
//   - Content-aware caching for parsed YAML documents
//   - Position tracking for provenance
//   - Output formatting and highlighting
//
// The custom tag processing uses the function registry from pkg/function to
// handle tags like !env, !exec, !terraform.output, etc.
//
// Example usage:
//
//	data, err := yaml.UnmarshalYAML[map[string]any](content)
//	if err != nil {
//	    return err
//	}
package yaml
