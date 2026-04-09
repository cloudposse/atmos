package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Default range for !random when no arguments provided.
	defaultRandomMin = 0
	defaultRandomMax = 65535
)

// ProcessTagRandom processes the !random YAML function and returns a random integer within the specified range.
// Usage:
//
//	!random           - Generate random number between 0 and 65535
//	!random max       - Generate random number between 0 and max
//	!random min max   - Generate random number between min and max
//
// Examples:
//
//	!random           -> random number 0-65535
//	!random 100       -> random number 0-100
//	!random 1024 65535 -> random number 1024-65535
func ProcessTagRandom(input string) (int, error) {
	defer perf.Track(nil, "utils.ProcessTagRandom")()

	log.Debug("Executing Atmos YAML function", "input", input)

	// Handle case where !random has no arguments.
	str, err := getStringAfterTag(input, AtmosYamlFuncRandom)
	if err != nil {
		// If no arguments, use defaults.
		return generateRandom(defaultRandomMin, defaultRandomMax)
	}

	parts, err := SplitStringByDelimiter(str, ' ')
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidAtmosYAMLFunction, input)
	}

	var min, max int

	switch len(parts) {
	case 0:
		// No arguments: use defaults.
		min = defaultRandomMin
		max = defaultRandomMax

	case 1:
		// One argument: treat as max, min defaults to 0.
		min = 0
		maxStr := strings.TrimSpace(parts[0])
		max, err = strconv.Atoi(maxStr)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid max value '%s', must be an integer: %s", ErrInvalidAtmosYAMLFunction, maxStr, input)
		}

	case 2:
		// Two arguments: min and max.
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])

		min, err = strconv.Atoi(minStr)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid min value '%s', must be an integer: %s", ErrInvalidAtmosYAMLFunction, minStr, input)
		}

		max, err = strconv.Atoi(maxStr)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid max value '%s', must be an integer: %s", ErrInvalidAtmosYAMLFunction, maxStr, input)
		}

	default:
		return 0, fmt.Errorf("%w: invalid number of arguments. The function accepts 0, 1, or 2 arguments: %s", ErrInvalidAtmosYAMLFunction, input)
	}

	return generateRandom(min, max)
}

// generateRandom generates a cryptographically secure random number in the range [min, max].
func generateRandom(min, max int) (int, error) {
	if min >= max {
		return 0, fmt.Errorf("%w: min value (%d) must be less than max value (%d)", ErrInvalidAtmosYAMLFunction, min, max)
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
