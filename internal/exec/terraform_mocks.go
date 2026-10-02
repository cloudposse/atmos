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
// In `fallback` mode this reports inactive: the Terraform lookups merge the mocks under the
// real outputs instead (see resolveTerraformOutputWithMocks).
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

// terraformAllOutputsExpression is the YQ identity expression used to fetch the whole real
// output map from a Terraform state/output getter, so the entire map comes back through the
// same cached, authenticated, and masked lookup path as any other expression.
const terraformAllOutputsExpression = "."

// declaredFallbackMocks returns the referenced component's mocks when --use-mocks runs in
// `fallback` mode and the component declares a `mocks` map. It returns nil (and no error) when
// mocks are off, the mode is not fallback, or the component declares none; callers then use the
// plain real lookup unchanged.
func declaredFallbackMocks(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
) (map[string]any, error) {
	if !terraformMocksEnabledInMode(atmosConfig, stackInfo, schema.TerraformMocksModeFallback) {
		return nil, nil
	}
	return describeComponentMocks(atmosConfig, lookup.stack, lookup.component)
}

// resolveTerraformOutputWithMocks evaluates the caller's expression against the component mocks
// deep-merged under the real outputs in `fallback` mode. Precedence is therefore: real value,
// then component mock, then YQ `//` default, then the original lookup error (or nil for an output
// missing from provisioned state).
//
// The merge is recursive for maps, so a mock fills a key missing from a real map output (real
// `config = {a = 1}` plus mock `config: {b: 2}` resolves `.config.b` to 2). Every value present in
// the real outputs wins, and lists and scalars are never merged element by element.
//
// The realOutputs argument is the whole real output map (nil when the state is not provisioned or the
// output map is absent) and realErr is the recoverable lookup error that produced it, if any.
// Neither the mocks nor the real map is mutated: the merge builds new maps.
func resolveTerraformOutputWithMocks(
	atmosConfig *schema.AtmosConfiguration,
	lookup *terraformStateLookup,
	mocks map[string]any,
	realOutputs map[string]any,
	realErr error,
) (any, error) {
	merged := mergeRealOverMocks(mocks, realOutputs)

	value, err := tb.GetTerraformBackendVariable(atmosConfig, merged, lookup.output)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate Terraform output %q for component %q in stack %q against component mocks: %w", lookup.output, lookup.component, lookup.stack, err)
	}
	if value == nil {
		return nil, missingTerraformOutputWithMocks(merged, lookup.output, realErr)
	}

	if resolvedFromMocks(mocks, realOutputs, lookup.output) {
		log.Debug(
			"Terraform YAML function resolved from component mocks (fallback)",
			"component", lookup.component,
			"stack", lookup.stack,
			"output", lookup.output,
		)
	}
	return value, nil
}

// mergeRealOverMocks returns a new map holding mocks deep-merged under real: maps are merged
// recursively and any other real value (including an explicit null, a list, or a scalar) replaces
// the mock at that key. Neither input is mutated.
func mergeRealOverMocks(mocks, real map[string]any) map[string]any {
	merged := make(map[string]any, len(mocks)+len(real))
	for key, value := range mocks {
		merged[key] = value
	}
	for key, realValue := range real {
		realMap, realIsMap := realValue.(map[string]any)
		mockMap, mockIsMap := merged[key].(map[string]any)
		if realIsMap && mockIsMap {
			merged[key] = mergeRealOverMocks(mockMap, realMap)
			continue
		}
		merged[key] = realValue
	}
	return merged
}

// resolvedFromMocks reports whether a simple output path is absent from the real outputs but
// declared in the mocks, i.e. the value came from the mocks. Complex YQ expressions report false.
func resolvedFromMocks(mocks, realOutputs map[string]any, output string) bool {
	inReal, decidable := mockPathExists(realOutputs, output)
	if !decidable || inReal {
		return false
	}
	inMocks, _ := mockPathExists(mocks, output)
	return inMocks
}

// withFallbackModeHint adds a hint to a non-recoverable lookup error raised in `fallback` mode:
// fallback reads real state first, so a machine without credentials, network access, or the
// Terraform binary now fails where mocks-only resolution would not.
func withFallbackModeHint(err error) error {
	return errUtils.Build(err).
		WithHintf("`%s` runs in `%s` mode, which reads real state first and uses mocks only when state or an output is missing. To resolve from mocks only, without Terraform, credentials, or backend access, pass `%s=%s` or set `components.terraform.mocks.mode: %s`.",
			cfg.UseMocksFlag, schema.TerraformMocksModeFallback, cfg.UseMocksFlag, schema.TerraformMocksModeAlways, schema.TerraformMocksModeAlways).
		Err()
}

// missingTerraformOutputWithMocks decides what a nil result means after evaluating against the
// merged outputs. An explicit null declared in the mocks or the real outputs is a hit. Otherwise
// the original recoverable lookup error is preserved (unless a YQ default already ran), and an
// output that is simply missing from provisioned state resolves to nil (backward compatible).
func missingTerraformOutputWithMocks(merged map[string]any, output string, realErr error) error {
	if exists, decidable := mockPathExists(merged, output); decidable && exists {
		return nil
	}
	if realErr != nil && !hasYqDefault(output) {
		return realErr
	}
	return nil
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
