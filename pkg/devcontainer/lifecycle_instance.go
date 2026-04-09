package devcontainer

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GenerateNewInstance generates a new unique instance name by finding
// existing containers for the given devcontainer name and incrementing the highest number.
// Pattern: {baseInstance}-1, {baseInstance}-2, etc.
// Returns the new instance name (e.g., "default-1", "default-2").
func (m *Manager) GenerateNewInstance(atmosConfig *schema.AtmosConfiguration, name, baseInstance string) (string, error) {
	defer perf.Track(atmosConfig, "devcontainer.GenerateNewInstance")()

	_, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return "", err
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return "", fmt.Errorf("%w: failed to initialize container runtime: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	ctx := context.Background()
	if baseInstance == "" {
		baseInstance = DefaultInstance
	}

	containers, err := runtime.List(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	maxNumber := findMaxInstanceNumber(containers, name, baseInstance)
	return fmt.Sprintf("%s-%d", baseInstance, maxNumber+1), nil
}

// findMaxInstanceNumber finds the highest instance number for a given devcontainer name and base instance.
func findMaxInstanceNumber(containers []container.Info, name, baseInstance string) int {
	maxNumber := 0
	basePattern := fmt.Sprintf("%s-", baseInstance)

	for _, c := range containers {
		parsedName, parsedInstance := ParseContainerName(c.Name)
		if parsedName != name {
			continue
		}

		if strings.HasPrefix(parsedInstance, basePattern) {
			numberStr := strings.TrimPrefix(parsedInstance, basePattern)
			var number int
			if _, err := fmt.Sscanf(numberStr, "%d", &number); err == nil {
				if number > maxNumber {
					maxNumber = number
				}
			}
		}
	}

	return maxNumber
}
