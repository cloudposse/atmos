package exec

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateStaticStackMaps reuses stack inheritance and duplicate-instance validation
// for already decoded, complete static inputs. It never loads YAML, runs templates,
// evaluates functions, or provisions components. Callers must reject unresolved inputs.
func ValidateStaticStackMaps(ctx context.Context, config *schema.AtmosConfiguration, inputs map[string]map[string]any) error {
	defer perf.Track(config, "exec.ValidateStaticStackMaps")()
	stacks := map[string]any{}
	for path, input := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		stack, _, err := ProcessStackConfig(config, config.StacksBaseAbsolutePath, config.TerraformDirAbsolutePath, config.HelmfileDirAbsolutePath, config.PackerDirAbsolutePath, config.AnsibleDirAbsolutePath, filepath.Join(config.StacksBaseAbsolutePath, path), input, false, true, "", map[string]map[string][]string{}, map[string]map[string]any{}, true)
		if err != nil {
			return err
		}
		stacks[strings.TrimSuffix(path, filepath.Ext(path))] = stack
	}
	var messages []string
	for _, kind := range []string{"terraform", "helmfile"} {
		instances, err := createComponentStackMap(config, stacks, kind)
		if err != nil {
			return err
		}
		failures, err := checkComponentStackMap(stacks, kind, instances)
		if err != nil {
			return err
		}
		messages = append(messages, failures...)
	}
	if len(messages) > 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrValidationFailed, strings.Join(dedupeStrings(messages), "\n\n"))
	}
	return nil
}
