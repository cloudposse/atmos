package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// yqPathSeparator is the YQ-style path separator used to address a nested mock
// output (e.g. ".foo.bar"); it also delimits path segments when checking whether
// a mocked output path exists.
const yqPathSeparator = "."

// resolveTerraformMockOutput resolves a Terraform lookup from the referenced
// component's literal `mocks` map. It deliberately describes the target with
// templates and YAML functions disabled so mocks cannot trigger the real
// dependency they replace.
//
// The second return value reports whether mock mode was active. In that mode a
// missing component mock or output is an error; callers must never fall back to
// remote state while the user explicitly requested --use-mocks.
func resolveTerraformMockOutput(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	stack string,
	component string,
	output string,
) (any, bool, error) {
	if stackInfo == nil || !stackInfo.UseMocks {
		return nil, false, nil
	}

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ComponentType:        cfg.TerraformComponentType,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		AuthDisabled:         true,
	})
	if err != nil {
		return nil, true, fmt.Errorf("failed to load mocks for Terraform component %q in stack %q: %w", component, stack, err)
	}

	mocks, ok := componentSection[cfg.MocksSectionName].(map[string]any)
	if !ok || mocks == nil {
		return nil, true, fmt.Errorf("%w: component %q in stack %q", errUtils.ErrTerraformComponentMocksNotDeclared, component, stack)
	}

	value, err := tb.GetTerraformBackendVariable(atmosConfig, mocks, output)
	if err != nil {
		return nil, true, fmt.Errorf("failed to resolve mocked Terraform output %q for component %q in stack %q: %w", output, component, stack, err)
	}
	if value == nil && !hasYqDefault(output) && !mockOutputExists(mocks, output) {
		return nil, true, fmt.Errorf("%w %q for component %q in stack %q", errUtils.ErrTerraformMockOutputNotDeclared, output, component, stack)
	}

	log.Debug(
		"Resolved Terraform YAML function from component mocks",
		"component", component,
		"stack", stack,
		"output", output,
	)
	return value, true, nil
}

// isDirectMockOutputReference reports whether an expression names one top-level
// output exactly. It lets an explicitly null mock remain valid while producing
// a useful error for a missing direct output name.
func isDirectMockOutputReference(output string) bool {
	output = strings.TrimPrefix(strings.TrimSpace(output), yqPathSeparator)
	return output != "" && !strings.Contains(output, yqPathSeparator) && !strings.ContainsAny(output, "[]{}|/ \t\n\r\"'")
}

// mockOutputExists detects absent simple output paths while preserving an
// explicitly configured null value. Complex YQ expressions are left to YQ: an
// empty result can be intentional (for example, a filter expression).
func mockOutputExists(mocks map[string]any, output string) bool {
	if isDirectMockOutputReference(output) {
		key := strings.TrimPrefix(strings.TrimSpace(output), yqPathSeparator)
		_, exists := mocks[key]
		return exists
	}

	output = strings.TrimSpace(output)
	if !strings.HasPrefix(output, yqPathSeparator) || strings.ContainsAny(output, "[]{}|/ \t\n\r\"'") {
		return true
	}

	current := any(mocks)
	for _, key := range strings.Split(strings.TrimPrefix(output, yqPathSeparator), yqPathSeparator) {
		if key == "" {
			return true
		}
		section, ok := current.(map[string]any)
		if !ok {
			return false
		}
		value, exists := section[key]
		if !exists {
			return false
		}
		current = value
	}
	return true
}
