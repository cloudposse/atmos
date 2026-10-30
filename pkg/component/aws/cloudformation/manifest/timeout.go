package manifest

import (
	"fmt"
	"maps"
	"time"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TimeoutSection normalizes the legacy minutes field at one inheritance scope.
func TimeoutSection(section map[string]any) map[string]any {
	defer perf.Track(nil, "cloudformation.manifest.TimeoutSection")()

	if value, ok := section[config.CloudFormationTimeoutSectionName]; ok {
		return map[string]any{config.CloudFormationTimeoutSectionName: value}
	}
	if value, ok := section[config.TimeoutInMinutesSectionName]; ok {
		return map[string]any{config.CloudFormationTimeoutSectionName: fmt.Sprintf("%vm", value)}
	}
	return nil
}

// DefaultsSection collects the CloudFormation options supported at every defaults scope.
func DefaultsSection(section map[string]any) map[string]any {
	defer perf.Track(nil, "cloudformation.manifest.DefaultsSection")()

	defaults := make(map[string]any)
	maps.Copy(defaults, RegionSection(section))
	maps.Copy(defaults, TimeoutSection(section))
	return defaults
}

// Timeout parses an optional positive operation deadline using Atmos duration syntax.
// Zero means no configured deadline; each operation retains its existing default wait budget.
func Timeout(section map[string]any) (time.Duration, error) {
	defer perf.Track(nil, "cloudformation.manifest.Timeout")()

	value, ok := TimeoutSection(section)[config.CloudFormationTimeoutSectionName]
	if !ok {
		return 0, nil
	}
	parsed, err := duration.ParseAny(value)
	if err != nil {
		return 0, fmt.Errorf("timeout: %w", err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("timeout: %w", duration.ErrNotPositive)
	}
	return parsed, nil
}
