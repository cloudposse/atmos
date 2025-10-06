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
