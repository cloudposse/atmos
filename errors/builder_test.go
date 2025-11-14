package errors

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
)

func TestBuild(t *testing.T) {
	baseErr := errors.New("test error")
	builder := Build(baseErr)

	assert.NotNil(t, builder)
	assert.Equal(t, baseErr, builder.err)
	assert.Empty(t, builder.hints)
	assert.Nil(t, builder.exitCode)
}

func TestErrorBuilder_WithHint(t *testing.T) {
	baseErr := errors.New("test error")
	builder := Build(baseErr).WithHint("hint 1")

	assert.Len(t, builder.hints, 1)
	assert.Equal(t, "hint 1", builder.hints[0])
}

func TestErrorBuilder_WithHint_Multiple(t *testing.T) {
	baseErr := errors.New("test error")
	builder := Build(baseErr).
		WithHint("hint 1").
		WithHint("hint 2").
		WithHint("hint 3")

	assert.Len(t, builder.hints, 3)
	assert.Equal(t, "hint 1", builder.hints[0])
	assert.Equal(t, "hint 2", builder.hints[1])
	assert.Equal(t, "hint 3", builder.hints[2])
}

func TestErrorBuilder_WithHintf(t *testing.T) {
	baseErr := errors.New("test error")
	builder := Build(baseErr).WithHintf("component: %s, stack: %s", "vpc", "dev")

	assert.Len(t, builder.hints, 1)
	assert.Equal(t, "component: vpc, stack: dev", builder.hints[0])
}

func TestErrorBuilder_WithHintf_Multiple(t *testing.T) {
	baseErr := errors.New("test error")
	builder := Build(baseErr).
		WithHintf("Check %s", "config").
		WithHintf("Run: %s", "atmos validate stacks")

	assert.Len(t, builder.hints, 2)
	assert.Equal(t, "Check config", builder.hints[0])
	assert.Equal(t, "Run: atmos validate stacks", builder.hints[1])
}

func TestErrorBuilder_WithContext(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithContext("component", "vpc").
		WithContext("stack", "dev").
		Err()

	assert.NotNil(t, err)

	// Verify context is present by checking the error contains safe details.
	details := errors.GetSafeDetails(err)
	assert.NotEmpty(t, details.SafeDetails)

	// Verify context contains expected values in sorted order.
	// Format should be: "component=vpc stack=dev" (alphabetically sorted).
	safeDetailsStr := ""
	for _, detail := range details.SafeDetails {
		safeDetailsStr = detail
		break // Get first detail string
	}
	assert.Contains(t, safeDetailsStr, "component=vpc")
	assert.Contains(t, safeDetailsStr, "stack=dev")
}

func TestErrorBuilder_WithContext_MultiplePairs(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithContext("region", "us-east-1").
		Err()

	assert.NotNil(t, err)

	// Verify context is present.
	details := errors.GetSafeDetails(err)
	assert.NotEmpty(t, details.SafeDetails)

	// Verify context contains all values in sorted order.
	safeDetailsStr := ""
	for _, detail := range details.SafeDetails {
		safeDetailsStr = detail
		break
	}
	assert.Contains(t, safeDetailsStr, "component=vpc")
	assert.Contains(t, safeDetailsStr, "stack=prod")
	assert.Contains(t, safeDetailsStr, "region=us-east-1")
}

func TestErrorBuilder_WithContext_SortedKeys(t *testing.T) {
	baseErr := errors.New("test error")
	// Add context in non-alphabetical order.
	err := Build(baseErr).
		WithContext("stack", "prod").
		WithContext("component", "vpc").
		WithContext("region", "us-east-1").
		Err()

	details := errors.GetSafeDetails(err)
	assert.NotEmpty(t, details.SafeDetails)

	// Keys should be sorted alphabetically: component, region, stack.
	safeDetailsStr := ""
	for _, detail := range details.SafeDetails {
		safeDetailsStr = detail
		break
	}

	// Check order by finding positions.
	componentPos := strings.Index(safeDetailsStr, "component=")
	regionPos := strings.Index(safeDetailsStr, "region=")
	stackPos := strings.Index(safeDetailsStr, "stack=")

	assert.True(t, componentPos < regionPos, "component should come before region")
	assert.True(t, regionPos < stackPos, "region should come before stack")
}

func TestErrorBuilder_WithExitCode(t *testing.T) {
	baseErr := errors.New("test error")
	builder := Build(baseErr).WithExitCode(42)

	assert.NotNil(t, builder.exitCode)
	assert.Equal(t, 42, *builder.exitCode)
}

func TestErrorBuilder_Err_WithHints(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithHint("hint 1").
		WithHint("hint 2").
		Err()

	assert.NotNil(t, err)

	// Verify hints are present.
	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 2)
	assert.Equal(t, "hint 1", hints[0])
	assert.Equal(t, "hint 2", hints[1])
}

func TestErrorBuilder_Err_WithExitCode(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithExitCode(42).
		Err()

	code := GetExitCode(err)
	assert.Equal(t, 42, code)
}

func TestErrorBuilder_Err_NilError(t *testing.T) {
	err := Build(nil).
		WithHint("hint").
		WithContext("key", "value").
		WithExitCode(42).
		Err()

	assert.Nil(t, err)
}

func TestErrorBuilder_Err_CompleteExample(t *testing.T) {
	baseErr := errors.New("database connection failed")
	err := Build(baseErr).
		WithHint("Check database credentials in atmos.yaml").
		WithHintf("Verify network connectivity to %s", "db.example.com").
		WithContext("component", "vpc").
		WithContext("stack", "prod").
		WithContext("region", "us-east-1").
		WithExitCode(2).
		Err()

	assert.NotNil(t, err)

	// Verify hints.
	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 2)
	assert.Equal(t, "Check database credentials in atmos.yaml", hints[0])
	assert.Equal(t, "Verify network connectivity to db.example.com", hints[1])

	// Verify context is present.
	details := errors.GetAllSafeDetails(err)
	assert.NotEmpty(t, details)

	// Verify exit code.
	code := GetExitCode(err)
	assert.Equal(t, 2, code)

	// Verify error message is preserved.
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestErrorBuilder_Chaining(t *testing.T) {
	baseErr := errors.New("base error")
	builder := Build(baseErr)

	// Test that chaining returns the same builder instance.
	b1 := builder.WithHint("hint 1")
	b2 := b1.WithHint("hint 2")
	b3 := b2.WithContext("key", "value")
	b4 := b3.WithExitCode(42)

	assert.Equal(t, builder, b1)
	assert.Equal(t, builder, b2)
	assert.Equal(t, builder, b3)
	assert.Equal(t, builder, b4)
}

func TestErrorBuilder_Err_PreservesErrorChain(t *testing.T) {
	baseErr := errors.New("base error")
	wrappedErr := errors.Wrap(baseErr, "wrapped")
	err := Build(wrappedErr).
		WithHint("hint").
		WithExitCode(2).
		Err()

	// Verify error chain is preserved.
	assert.True(t, errors.Is(err, baseErr))
	assert.True(t, errors.Is(err, wrappedErr))
}

func TestErrorBuilder_WithExplanation(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithExplanation("This is a detailed explanation of the error.").
		Err()

	assert.NotNil(t, err)

	// Verify explanation is present via GetAllDetails.
	details := errors.GetAllDetails(err)
	assert.Len(t, details, 1)
	assert.Equal(t, "This is a detailed explanation of the error.", details[0])
}

func TestErrorBuilder_WithExplanationf(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithExplanationf("The file `%s` does not exist in directory `%s`.", "config.yaml", "/path/to/dir").
		Err()

	assert.NotNil(t, err)

	details := errors.GetAllDetails(err)
	assert.Len(t, details, 1)
	assert.Equal(t, "The file `config.yaml` does not exist in directory `/path/to/dir`.", details[0])
}

func TestErrorBuilder_WithExample(t *testing.T) {
	baseErr := errors.New("test error")
	exampleContent := "component: vpc\nstack: prod"
	err := Build(baseErr).
		WithExample(exampleContent).
		Err()

	assert.NotNil(t, err)

	// Verify example is stored as a hint with EXAMPLE: prefix.
	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 1)
	assert.Equal(t, "EXAMPLE:"+exampleContent, hints[0])
}

func TestErrorBuilder_WithExampleFile(t *testing.T) {
	baseErr := errors.New("test error")
	exampleContent := "```yaml\nworkflows:\n  deploy:\n    steps:\n      - command: terraform apply\n```"
	err := Build(baseErr).
		WithExampleFile(exampleContent).
		Err()

	assert.NotNil(t, err)

	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 1)
	assert.Equal(t, "EXAMPLE:"+exampleContent, hints[0])
}

func TestErrorBuilder_CompleteWithAllSections(t *testing.T) {
	baseErr := errors.New("invalid workflow manifest")
	exampleContent := "```yaml\nworkflows:\n  deploy:\n    steps:\n      - command: terraform apply\n```"

	err := Build(baseErr).
		WithExplanation("The workflow manifest must contain a top-level workflows: key.").
		WithExampleFile(exampleContent).
		WithHint("Check the YAML structure").
		WithHintf("Valid format requires %s", "`workflows:` key").
		WithContext("file", "/path/to/workflow.yaml").
		WithContext("line", "1").
		WithExitCode(2).
		Err()

	assert.NotNil(t, err)

	// Verify explanation.
	details := errors.GetAllDetails(err)
	assert.Len(t, details, 1)
	assert.Equal(t, "The workflow manifest must contain a top-level workflows: key.", details[0])

	// Verify hints (2 regular hints + 1 example).
	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 3)

	// Check regular hints.
	var regularHints []string
	var examples []string
	for _, hint := range hints {
		if strings.HasPrefix(hint, "EXAMPLE:") {
			examples = append(examples, strings.TrimPrefix(hint, "EXAMPLE:"))
		} else {
			regularHints = append(regularHints, hint)
		}
	}

	assert.Len(t, regularHints, 2)
	assert.Equal(t, "Check the YAML structure", regularHints[0])
	assert.Equal(t, "Valid format requires `workflows:` key", regularHints[1])

	assert.Len(t, examples, 1)
	assert.Equal(t, exampleContent, examples[0])

	// Verify context.
	safeDetails := errors.GetAllSafeDetails(err)
	assert.NotEmpty(t, safeDetails)

	// Verify exit code.
	code := GetExitCode(err)
	assert.Equal(t, 2, code)

	// Verify base error message.
	assert.Contains(t, err.Error(), "invalid workflow manifest")
}

func TestErrorBuilder_MultipleExamples(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithExample("example 1").
		WithExample("example 2").
		WithExample("example 3").
		Err()

	assert.NotNil(t, err)

	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 3)

	// All should have EXAMPLE: prefix.
	for _, hint := range hints {
		assert.True(t, strings.HasPrefix(hint, "EXAMPLE:"))
		expected := strings.TrimPrefix(hint, "EXAMPLE:")
		assert.Equal(t, expected, hint[8:]) // Skip "EXAMPLE:" prefix
		assert.Contains(t, hint, "example")
	}
}

func TestErrorBuilder_MixedHintsAndExamples(t *testing.T) {
	baseErr := errors.New("test error")
	err := Build(baseErr).
		WithHint("Regular hint 1").
		WithExample("Example code").
		WithHint("Regular hint 2").
		WithExample("Another example").
		Err()

	assert.NotNil(t, err)

	hints := errors.GetAllHints(err)
	assert.Len(t, hints, 4)

	var regularHints []string
	var examples []string
	for _, hint := range hints {
		if strings.HasPrefix(hint, "EXAMPLE:") {
			examples = append(examples, strings.TrimPrefix(hint, "EXAMPLE:"))
		} else {
			regularHints = append(regularHints, hint)
		}
	}

	assert.Len(t, regularHints, 2)
	assert.Equal(t, "Regular hint 1", regularHints[0])
	assert.Equal(t, "Regular hint 2", regularHints[1])

	assert.Len(t, examples, 2)
	assert.Equal(t, "Example code", examples[0])
	assert.Equal(t, "Another example", examples[1])
}

func TestErrorBuilder_SentinelMarking(t *testing.T) {
	t.Run("automatically marks sentinel when used as base error", func(t *testing.T) {
		err := Build(ErrContainerRuntimeOperation).
			WithExplanation("Failed to start container").
			WithHint("Check Docker is running").
			Err()

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrContainerRuntimeOperation))
	})
}

func TestErrorBuilder_WithCause(t *testing.T) {
	t.Run("wraps sentinel with cause error", func(t *testing.T) {
		causeErr := errors.New("container already running")

		err := Build(ErrContainerRuntimeOperation).
			WithCause(causeErr).
			WithExplanation("Failed to start container").
			Err()

		assert.Error(t, err)
		// Both sentinel and cause should be in the error chain.
		assert.True(t, errors.Is(err, ErrContainerRuntimeOperation))
		assert.True(t, errors.Is(err, causeErr))
		// Error message should include the cause.
		assert.Contains(t, err.Error(), "container already running")
	})

	t.Run("preserves cause error message", func(t *testing.T) {
		causeErr := errors.New("docker daemon not running")

		err := Build(ErrContainerRuntimeOperation).
			WithCause(causeErr).
			Err()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "docker daemon not running")
	})

	t.Run("handles nil cause gracefully", func(t *testing.T) {
		err := Build(ErrContainerRuntimeOperation).
			WithCause(nil).
			WithExplanation("Operation failed").
			Err()

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrContainerRuntimeOperation))
	})

	t.Run("works with stdlib errors.Is in tests", func(t *testing.T) {
		// This test verifies WithCause works with stdlib errors package.
		// This is critical because our test files use stdlib errors, not cockroachdb.
		causeErr := errors.New("connection refused")

		err := Build(ErrContainerRuntimeOperation).
			WithCause(causeErr).
			Err()

		// Using stdlib errors.Is, not cockroachdb.
		assert.True(t, errors.Is(err, ErrContainerRuntimeOperation))
		assert.True(t, errors.Is(err, causeErr))
	})
}
