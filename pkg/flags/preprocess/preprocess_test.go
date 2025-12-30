package preprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockPreprocessor is a simple preprocessor for testing that appends a suffix.
type mockPreprocessor struct {
	suffix string
}

func (m *mockPreprocessor) Preprocess(args []string) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = arg + m.suffix
	}
	return result
}

func TestNewPipeline(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline()
	assert.NotNil(t, pipeline)
	assert.Empty(t, pipeline.preprocessors)
}

func TestPipeline_Run_Empty(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline()
	args := []string{"--flag", "value"}

	result := pipeline.Run(args)

	assert.Equal(t, args, result)
}

func TestPipeline_Run_SinglePreprocessor(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(&mockPreprocessor{suffix: "-modified"})
	args := []string{"arg1", "arg2"}

	result := pipeline.Run(args)

	assert.Equal(t, []string{"arg1-modified", "arg2-modified"}, result)
}

func TestPipeline_Run_MultiplePreprocessors(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(
		&mockPreprocessor{suffix: "-first"},
		&mockPreprocessor{suffix: "-second"},
	)
	args := []string{"arg"}

	result := pipeline.Run(args)

	// Each preprocessor runs in sequence.
	assert.Equal(t, []string{"arg-first-second"}, result)
}

func TestPipeline_Run_DoesNotModifyInput(t *testing.T) {
	t.Parallel()

	pipeline := NewPipeline(&mockPreprocessor{suffix: "-modified"})
	args := []string{"arg1", "arg2"}
	originalArgs := make([]string, len(args))
	copy(originalArgs, args)

	_ = pipeline.Run(args)

	// Original args should not be modified.
	assert.Equal(t, originalArgs, args)
}
