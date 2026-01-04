package function

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	// Default range for random when no arguments provided.
	defaultRandomMin = 0
	defaultRandomMax = 65535
)

// RandomFunction implements the random function for generating random numbers.
type RandomFunction struct {
	BaseFunction
}

// NewRandomFunction creates a new random function handler.
func NewRandomFunction() *RandomFunction {
	defer perf.Track(nil, "function.NewRandomFunction")()

	return &RandomFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagRandom,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the random function.
// Usage:
//
//	!random           - Generate random number between 0 and 65535
//	!random max       - Generate random number between 0 and max
//	!random min max   - Generate random number between min and max
func (f *RandomFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.RandomFunction.Execute")()

	log.Debug("Executing random function", "args", args)

	args = strings.TrimSpace(args)

	// No arguments: use defaults.
	if args == "" {
		return generateRandom(defaultRandomMin, defaultRandomMax)
	}

	parts, err := utils.SplitStringByDelimiter(args, ' ')
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArguments, args)
	}

	var min, max int

	switch len(parts) {
	case 0:
		min = defaultRandomMin
		max = defaultRandomMax

	case 1:
		// One argument: treat as max, min defaults to 0.
		min = 0
		maxStr := strings.TrimSpace(parts[0])
		max, err = strconv.Atoi(maxStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid max value '%s', must be an integer", ErrInvalidArguments, maxStr)
		}

	case 2:
		// Two arguments: min and max.
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])

		min, err = strconv.Atoi(minStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid min value '%s', must be an integer", ErrInvalidArguments, minStr)
		}

		max, err = strconv.Atoi(maxStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid max value '%s', must be an integer", ErrInvalidArguments, maxStr)
		}

	default:
		return nil, fmt.Errorf("%w: random function accepts 0, 1, or 2 arguments, got %d", ErrInvalidArguments, len(parts))
	}

	return generateRandom(min, max)
}

// generateRandom generates a cryptographically secure random number in the range [min, max].
func generateRandom(min, max int) (int, error) {
	if min >= max {
		return 0, fmt.Errorf("%w: min value (%d) must be less than max value (%d)", ErrInvalidArguments, min, max)
	}

	// Generate cryptographically secure random number in range [min, max].
	rangeSize := int64(max - min + 1)
	n, err := rand.Int(rand.Reader, big.NewInt(rangeSize))
	if err != nil {
		return 0, fmt.Errorf("failed to generate random number: %w", err)
	}

	result := int(n.Int64()) + min

	log.Debug("Generated random number", "min", min, "max", max, "result", result)

	return result, nil
}
