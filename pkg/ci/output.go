package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// NoopOutputWriter is an OutputWriter that does nothing.
// Used when not running in CI or when CI outputs are disabled.
type NoopOutputWriter = provider.NoopOutputWriter

// FileOutputWriter writes outputs to a file (like $GITHUB_OUTPUT).
type FileOutputWriter = provider.FileOutputWriter

// NewFileOutputWriter creates a new FileOutputWriter.
func NewFileOutputWriter(outputPath, summaryPath string) *FileOutputWriter {
	return provider.NewFileOutputWriter(outputPath, summaryPath)
}

// OutputHelpers provides helper methods for common CI output patterns.
type OutputHelpers = provider.OutputHelpers

// NewOutputHelpers creates a new OutputHelpers.
func NewOutputHelpers(writer OutputWriter) *OutputHelpers {
	return provider.NewOutputHelpers(writer)
}

// PlanOutputOptions contains options for writing plan outputs.
type PlanOutputOptions = provider.PlanOutputOptions

// ApplyOutputOptions contains options for writing apply outputs.
type ApplyOutputOptions = provider.ApplyOutputOptions
