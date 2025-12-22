package function

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TagRandom is defined in tags.go.
// Use YAMLTag(TagRandom) to get the YAML tag format.

const (
	// Default range for !random when no arguments provided.
	defaultRandomMin = 0
	defaultRandomMax = 65535
)

// RandomFunction implements the !random YAML function.
// It generates cryptographically secure random integers.
type RandomFunction struct {
	BaseFunction
}

// NewRandomFunction creates a new RandomFunction.
func NewRandomFunction() *RandomFunction {
	defer perf.Track(nil, "function.NewRandomFunction")()

	return &RandomFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "random",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the !random function.
// Syntax:
//
//	!random           - Generate random number between 0 and 65535
//	!random max       - Generate random number between 0 and max
//	!random min max   - Generate random number between min and max
func (f *RandomFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.RandomFunction.Execute")()

	args = strings.TrimSpace(args)

	// No arguments: use defaults.
	if args == "" {
		return generateRandomNumber(defaultRandomMin, defaultRandomMax)
	}

	min, max, err := parseRandomArgs(args)
	if err != nil {
		return nil, err
	}

	return generateRandomNumber(min, max)
}

// parseRandomArgs parses the arguments for the random function.
func parseRandomArgs(args string) (min, max int, err error) {
	// Split by whitespace - simple splitting is sufficient for numeric arguments.
	parts := strings.Fields(args)

	switch len(parts) {
	case 1:
		// One argument: treat as max, min defaults to 0.
		maxStr := strings.TrimSpace(parts[0])
		max, err = strconv.Atoi(maxStr)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: invalid max value '%s', must be an integer", ErrInvalidArguments, maxStr)
		}
		return 0, max, nil

	case 2:
		// Two arguments: min and max.
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])

		min, err = strconv.Atoi(minStr)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: invalid min value '%s', must be an integer", ErrInvalidArguments, minStr)
		}

		max, err = strconv.Atoi(maxStr)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: invalid max value '%s', must be an integer", ErrInvalidArguments, maxStr)
		}
		return min, max, nil

	default:
		return 0, 0, fmt.Errorf("%w: !random accepts 0, 1, or 2 arguments, got %d", ErrInvalidArguments, len(parts))
	}
}

// generateRandomNumber generates a cryptographically secure random number in the range [min, max].
func generateRandomNumber(min, max int) (int, error) {
	defer perf.Track(nil, "function.generateRandomNumber")()

	if min >= max {
		return 0, fmt.Errorf("%w: min value (%d) must be less than max value (%d)", ErrInvalidArguments, min, max)
	}

	// Generate cryptographically secure random number in range [min, max].
	// Cast to int64 before arithmetic to prevent overflow with large ranges.
	rangeSize := int64(max) - int64(min) + 1
	if rangeSize <= 0 {
		return 0, fmt.Errorf("%w: invalid range size computed for min=%d max=%d", ErrInvalidArguments, min, max)
	}

	n, err := rand.Int(rand.Reader, big.NewInt(rangeSize))
	if err != nil {
		return 0, fmt.Errorf("failed to generate random number: %w", err)
	}

	// Compute result in int64 space to avoid overflow on addition.
	result := n.Int64() + int64(min)
	return int(result), nil
}
