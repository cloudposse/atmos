package internal

import (
	"encoding/json"
	"slices"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

const metadataAnnotation = "atmos-command-metadata"

// CommandExample describes an installable example. Registration order determines
// display order, with the first example recommended by the command's author.
type CommandExample struct {
	Source string `json:"source"`
	Label  string `json:"label"`
}

// CommandMetadata provides documentation and examples independently of execution.
type CommandMetadata struct {
	DocumentationURL string           `json:"documentation_url,omitempty"`
	Examples         []CommandExample `json:"examples,omitempty"`
}

// CommandOption configures discovery metadata without changing CommandProvider.
type CommandOption func(*CommandMetadata)

// WithDocumentationURL sets the command's canonical documentation URL.
func WithDocumentationURL(url string) CommandOption {
	defer perf.Track(nil, "internal.WithDocumentationURL")()
	return func(metadata *CommandMetadata) { metadata.DocumentationURL = url }
}

// WithExample appends an installable example, in recommendation order.
func WithExample(source, label string) CommandOption {
	defer perf.Track(nil, "internal.WithExample")()
	return func(metadata *CommandMetadata) {
		metadata.Examples = append(metadata.Examples, CommandExample{Source: source, Label: label})
	}
}

// RegisterCommandMetadata attaches the same options accepted by Register to a
// nested command or a command registered directly with Cobra.
func RegisterCommandMetadata(cmd *cobra.Command, options ...CommandOption) {
	defer perf.Track(nil, "internal.RegisterCommandMetadata")()
	if cmd == nil || len(options) == 0 {
		return
	}
	metadata := GetCommandMetadata(cmd)
	for _, option := range options {
		option(&metadata)
	}
	setCommandMetadata(cmd, metadata)
}

func setCommandMetadata(cmd *cobra.Command, metadata CommandMetadata) {
	// This struct contains only strings and slices, so marshaling cannot fail.
	encoded, _ := json.Marshal(metadata)
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[metadataAnnotation] = string(encoded)
}

func uniqueExamples(examples []CommandExample) []CommandExample {
	seen := make(map[string]bool)
	result := make([]CommandExample, 0, len(examples))
	for _, example := range examples {
		if !seen[example.Source] {
			result = append(result, example)
			seen[example.Source] = true
		}
	}
	return result
}

// GetCommandMetadata returns only metadata explicitly registered on cmd.
func GetCommandMetadata(cmd *cobra.Command) CommandMetadata {
	defer perf.Track(nil, "internal.GetCommandMetadata")()
	var metadata CommandMetadata
	if cmd != nil {
		_ = json.Unmarshal([]byte(cmd.Annotations[metadataAnnotation]), &metadata)
	}
	return metadata
}

// ResolveCommandMetadata inherits each missing field from its nearest ancestor.
// A local example list replaces the parent's entire list.
func ResolveCommandMetadata(cmd *cobra.Command) CommandMetadata {
	defer perf.Track(nil, "internal.ResolveCommandMetadata")()
	var resolved CommandMetadata
	for current := cmd; current != nil; current = current.Parent() {
		metadata := GetCommandMetadata(current)
		if resolved.DocumentationURL == "" {
			resolved.DocumentationURL = metadata.DocumentationURL
		}
		if len(resolved.Examples) == 0 {
			resolved.Examples = slices.Clone(metadata.Examples)
		}
	}
	if resolved.DocumentationURL == "" {
		resolved.DocumentationURL = "https://atmos.tools"
	}
	return resolved
}
