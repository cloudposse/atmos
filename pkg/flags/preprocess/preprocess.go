package preprocess

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// Preprocessor transforms command-line arguments before Cobra parses them.
// This is distinct from compatibility flags (pkg/flags/compat) which translate
// external tool syntax like Terraform's -var.
//
// Preprocessing handles native Atmos flag behavior that requires argument
// transformation before Cobra's parsing phase.
type Preprocessor interface {
	// Preprocess transforms args and returns the modified args.
	// Implementations should not modify the input slice.
	Preprocess(args []string) []string
}

// Pipeline runs multiple preprocessors in sequence.
// This allows composing multiple preprocessing steps in a clear, testable manner.
type Pipeline struct {
	preprocessors []Preprocessor
}

// NewPipeline creates a new preprocessing pipeline with the given preprocessors.
// Preprocessors are run in the order they are provided.
func NewPipeline(preprocessors ...Preprocessor) *Pipeline {
	defer perf.Track(nil, "preprocess.NewPipeline")()

	return &Pipeline{
		preprocessors: preprocessors,
	}
}

// Run executes all preprocessors in sequence, passing the output of each
// as the input to the next.
func (p *Pipeline) Run(args []string) []string {
	defer perf.Track(nil, "preprocess.Pipeline.Run")()

	result := args
	for _, preprocessor := range p.preprocessors {
		result = preprocessor.Preprocess(result)
	}
	return result
}
