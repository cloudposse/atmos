package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// NewFileOutputWriter creates a new FileOutputWriter.
func NewFileOutputWriter(outputPath, summaryPath string) *provider.FileOutputWriter {
	return provider.NewFileOutputWriter(outputPath, summaryPath)
}

// NewOutputHelpers creates a new OutputHelpers.
func NewOutputHelpers(writer provider.OutputWriter) *provider.OutputHelpers {
	return provider.NewOutputHelpers(writer)
}
