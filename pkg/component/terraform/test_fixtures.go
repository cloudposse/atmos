package terraform

import (
	"maps"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prepareTerraformTestFixtures preserves the pre-authentication contract of test
// hooks: they can start the emulator that the component's identity requires.
// Source/workdir provisioning still precedes the hook so its files reach Terraform.
// Only runtime keys introduced by provisioning survive authenticated stack processing.
func (e *Executor) prepareTerraformTestFixtures(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts ...component.ExecutionOption) (map[string]any, error) {
	if info.SubCommand != "test" {
		return nil, nil
	}
	preparedHooks, ok := info.NodeHooks.(schema.ComponentNodeHooksWithPreparation)
	if !ok {
		return nil, nil
	}
	resolved, err := e.services.ProcessStacks(atmosConfig, *info, true, false, false, info.Skip, nil)
	if err != nil {
		return nil, err
	}
	needsPreAuth, err := preparedHooks.NeedsPreAuth(atmosConfig, &resolved)
	if err != nil || !needsPreAuth {
		return nil, err
	}
	*info = resolved
	if !info.ComponentIsEnabled {
		return nil, nil
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	return e.provisionAndRunTerraformTestFixtures(atmosConfig, info, opts...)
}

// provisionAndRunTerraformTestFixtures retains the runtime keys introduced by
// provisioning, so authenticated stack resolution cannot discard them.
func (e *Executor) provisionAndRunTerraformTestFixtures(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts ...component.ExecutionOption) (map[string]any, error) {
	original := maps.Clone(info.ComponentSection)
	if _, err := e.resolveAndProvisionComponentPath(component.ContextFromOptions(opts...), executionOutputWriters(opts...), atmosConfig, info); err != nil {
		return nil, err
	}
	state := make(map[string]any)
	for key, value := range info.ComponentSection {
		if _, exists := original[key]; !exists {
			state[key] = value
		}
	}
	return state, runPreparedTerraformHooks(atmosConfig, info, opts...)
}

// runPreparedTerraformHooks runs hooks with the execution directory already resolved.
func runPreparedTerraformHooks(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts ...component.ExecutionOption) error {
	preparedHooks, ok := info.NodeHooks.(schema.ComponentNodeHooksWithPreparation)
	if !ok {
		return nil
	}
	writers := executionOutputWriters(opts...)
	return preparedHooks.BeforePrepared(component.ContextFromOptions(opts...), atmosConfig, info, schema.ComponentNodeHookWriters{
		Stdout: writers.Stdout, Stderr: writers.Stderr,
	})
}
