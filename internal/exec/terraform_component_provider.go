package exec

import (
	"fmt"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The host composes the built-in provider with the legacy lifecycle. The provider
// package itself does not import internal/exec. Further extraction replaces this
// runtime without changing registry callers.
func init() {
	if err := component.Register(terraformcomponent.New(terraformRuntime{})); err != nil {
		panic(err)
	}
}

type terraformRuntime struct{}

// Execute forwards the provider request to the host lifecycle, preserving options and caller cancellation.
func (terraformRuntime) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "exec.terraformRuntime.Execute")()

	// Context on the provider request is authoritative. Retain every other option
	// (including reporting callbacks) without resolving and reconstructing it.
	opts := append(slices.Clone(ctx.Options), WithProcessContext(ctx.GoContext()))
	return executeTerraform(ctx.ConfigAndStacksInfo, opts...)
}

// GenerateArtifacts writes configuration files in the prepared component working directory.
func (terraformRuntime) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "exec.terraformRuntime.GenerateArtifacts")()

	info := ctx.ConfigAndStacksInfo
	return generateConfigFiles(ctx.AtmosConfig, &info, ctx.WorkingDir)
}

// ExecuteTerraform is a compatibility entry point for existing CLI, scheduler,
// plan-diff, and migration callers. All of them now select the registered provider.
//
//nolint:gocritic // Preserve the public value-based API while routing existing callers through the registry.
func ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "exec.DispatchTerraform")()

	provider, ok := component.GetProvider("terraform")
	if !ok {
		return fmt.Errorf("%w: terraform", errUtils.ErrComponentProviderNotFound)
	}
	return provider.Execute(&component.ExecutionContext{
		Context: shellCommandContext(opts...), Options: opts,
		ComponentType: "terraform", Component: info.ComponentFromArg,
		Stack: info.Stack, Command: info.Command, SubCommand: info.SubCommand,
		ComponentConfig: info.ComponentSection, ConfigAndStacksInfo: info,
		Args: info.AdditionalArgsAndFlags,
	})
}
