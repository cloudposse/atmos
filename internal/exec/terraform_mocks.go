package exec

import (
	"errors"
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

// terraformMocksMode returns the effective components.terraform.mocks.mode, defaulting to
// fallback for a nil config.
func terraformMocksMode(atmosConfig *schema.AtmosConfiguration) schema.TerraformMocksMode {
	if atmosConfig == nil {
		return schema.TerraformMocksModeFallback
	}
	return atmosConfig.Components.Terraform.EffectiveMocksMode()
}

// terraformMocksEnabledInMode reports whether --use-mocks is on and the effective mocks mode is mode.
func terraformMocksEnabledInMode(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	mode schema.TerraformMocksMode,
) bool {
	return stackInfo != nil && stackInfo.UseMocks && terraformMocksMode(atmosConfig) == mode
}

// describeComponentMocks returns the literal `mocks` map of the referenced component, or nil when
// the component declares none. It deliberately describes the target with templates and YAML
// functions disabled (and auth disabled) so mocks cannot trigger the real dependency they replace.
func describeComponentMocks(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
) (map[string]any, error) {
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
		return nil, fmt.Errorf("failed to load mocks for Terraform component %q in stack %q: %w", component, stack, err)
	}

	mocks, ok := componentSection[cfg.MocksSectionName].(map[string]any)
	if !ok || mocks == nil {
		return nil, nil
	}
	return mocks, nil
}

// resolveTerraformMockOutput resolves a Terraform lookup from the referenced
// component's literal `mocks` map in `always` mode. It deliberately describes the
// target with templates and YAML functions disabled so mocks cannot trigger the
// real dependency they replace.
//
// The second return value reports whether always-mode mock resolution was active.
// In that mode a missing component mock or output is an error; callers must never
// fall back to remote state while the user explicitly requested hermetic mocks.
// In `fallback` mode this reports inactive: the real lookup runs first and
// resolveTerraformMockFallback consults the mocks only on a recoverable miss.
func resolveTerraformMockOutput(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	stack string,
	component string,
	output string,
) (any, bool, error) {
	if err := validateTerraformMocksMode(atmosConfig, stackInfo); err != nil {
		return nil, true, err
	}
	if !terraformMocksEnabledInMode(atmosConfig, stackInfo, schema.TerraformMocksModeAlways) {
		return nil, false, nil
	}

	mocks, err := describeComponentMocks(atmosConfig, stack, component)
	if err != nil {
		return nil, true, err
	}
	mocks, err = defaultUndeclaredMocks(mocks, stack, component, output)
	if err != nil {
		return nil, true, err
	}

	value, err := tb.GetTerraformBackendVariable(atmosConfig, mocks, output)
	if err != nil {
		return nil, true, fmt.Errorf("failed to resolve mocked Terraform output %q for component %q in stack %q: %w", output, component, stack, err)
	}
	if value == nil && !hasYqDefault(output) && !mockOutputExists(mocks, output) {
		return nil, true, fmt.Errorf("%w: %q for component %q in stack %q", errUtils.ErrTerraformMockOutputNotDeclared, output, component, stack)
	}

	log.Debug(
		"Resolved Terraform YAML function from component mocks",
		"component", component,
		"stack", stack,
		"output", output,
	)
	return value, true, nil
}

// validateTerraformMocksMode rejects an unknown mocks mode when --use-mocks is on.
// The env var and flag paths validate the mode, but an atmos.yaml value is only checked
// here. An unknown mode would match neither always nor fallback and silently disable
// --use-mocks, so fail loudly instead.
func validateTerraformMocksMode(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	if stackInfo == nil || !stackInfo.UseMocks {
		return nil
	}
	if mode := terraformMocksMode(atmosConfig); !mode.IsValid() {
		return fmt.Errorf("%w: got %q", errUtils.ErrInvalidMocksMode, mode)
	}
	return nil
}

// defaultUndeclaredMocks returns mocks unchanged when the component declares them. For a
// component with no `mocks` map, a `//` default in the caller's expression is honored the same
// way whether or not the component declares mocks, mirroring how a `//` default rescues a
// component with no real state at all. Without a default, an undeclared `mocks` map is a hard error.
func defaultUndeclaredMocks(mocks map[string]any, stack, component, output string) (map[string]any, error) {
	if mocks != nil {
		return mocks, nil
	}
	if !hasYqDefault(output) {
		return nil, fmt.Errorf("%w: component %q in stack %q", errUtils.ErrTerraformComponentMocksNotDeclared, component, stack)
	}
	return map[string]any{}, nil
}

// resolveTerraformMockFallback is called by the Terraform YAML functions on a recoverable miss
// (state not provisioned, output not found, or a null direct output). When --use-mocks runs in
// `fallback` mode and the component declares the output, it returns the mock value.
//
// The second return value reports whether the lookup was resolved here; when false the caller
// continues down its existing path (YQ default, then the original error). An absent mock is not
// an error in fallback mode. The cause argument is the original lookup error (nil for a silent miss); it is
// joined with a mock-loading failure so the original sentinel stays matchable.
func resolveTerraformMockFallback(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
	cause error,
) (any, bool, error) {
	if !terraformMocksEnabledInMode(atmosConfig, stackInfo, schema.TerraformMocksModeFallback) {
		return nil, false, nil
	}

	value, found, err := lookupTerraformMock(atmosConfig, lookup.stack, lookup.component, lookup.output)
	if err != nil {
		return nil, true, errors.Join(cause, err)
	}
	if !found {
		return nil, false, nil
	}

	log.Debug(
		"Terraform YAML function resolved from component mocks (fallback)",
		"component", lookup.component,
		"stack", lookup.stack,
		"output", lookup.output,
	)
	return value, true, nil
}

// lookupTerraformMock evaluates output against the referenced component's `mocks` map. Found is
// false when the component declares no mocks or the mocks do not declare the requested output.
func lookupTerraformMock(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
) (value any, found bool, err error) {
	mocks, err := describeComponentMocks(atmosConfig, stack, component)
	if err != nil || mocks == nil {
		return nil, false, err
	}

	value, err = tb.GetTerraformBackendVariable(atmosConfig, mocks, output)
	if err != nil {
		return nil, false, fmt.Errorf("failed to resolve mocked Terraform output %q for component %q in stack %q: %w", output, component, stack, err)
	}
	if value != nil {
		return value, true, nil
	}

	// A nil value is a hit only for a simple output path the mocks explicitly declare (an
	// explicit null). Complex YQ expressions that yield nil count as a miss.
	exists, decidable := mockPathExists(mocks, output)
	return nil, decidable && exists, nil
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
	exists, decidable := mockPathExists(mocks, output)
	return exists || !decidable
}

// mockPathExists reports whether a simple output path is declared in mocks. Decidable is false
// for complex YQ expressions, whose existence cannot be determined without evaluating them.
func mockPathExists(mocks map[string]any, output string) (exists, decidable bool) {
	if isDirectMockOutputReference(output) {
		key := strings.TrimPrefix(strings.TrimSpace(output), yqPathSeparator)
		_, declared := mocks[key]
		return declared, true
	}

	output = strings.TrimSpace(output)
	if !strings.HasPrefix(output, yqPathSeparator) || strings.ContainsAny(output, "[]{}|/ \t\n\r\"'") {
		return false, false
	}

	current := any(mocks)
	for _, key := range strings.Split(strings.TrimPrefix(output, yqPathSeparator), yqPathSeparator) {
		if key == "" {
			return false, false
		}
		section, ok := current.(map[string]any)
		if !ok {
			return false, true
		}
		value, declared := section[key]
		if !declared {
			return false, true
		}
		current = value
	}
	return true, true
}
