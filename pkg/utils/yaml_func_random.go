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

// ProcessTagRandom processes the !random YAML function and returns a random integer within the specified range.
// Usage: !random min max
// Example: !random 1024 65535
func ProcessTagRandom(input string) (int, error) {
	defer perf.Track(nil, "utils.ProcessTagRandom")()

	log.Debug("Executing Atmos YAML function", "input", input)

	str, err := getStringAfterTag(input, AtmosYamlFuncRandom)
	if err != nil {
		return 0, err
	}

	parts, err := SplitStringByDelimiter(str, ' ')
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidAtmosYAMLFunction, input)
	}

	if len(parts) != 2 {
		return 0, fmt.Errorf("%w: invalid number of arguments. The function requires exactly 2 arguments (min max): %s", ErrInvalidAtmosYAMLFunction, input)
	}

	minStr := strings.TrimSpace(parts[0])
	maxStr := strings.TrimSpace(parts[1])

	min, err := strconv.Atoi(minStr)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid min value '%s', must be an integer: %s", ErrInvalidAtmosYAMLFunction, minStr, input)
	}

	max, err := strconv.Atoi(maxStr)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid max value '%s', must be an integer: %s", ErrInvalidAtmosYAMLFunction, maxStr, input)
	}

	if min >= max {
		return 0, fmt.Errorf("%w: min value (%d) must be less than max value (%d): %s", ErrInvalidAtmosYAMLFunction, min, max, input)
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
