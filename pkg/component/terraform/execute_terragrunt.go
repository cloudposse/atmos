package terraform

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/terragruntexec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// terragruntTool is the tool name resolved through the toolchain; tests replace it with a test executable.
var terragruntTool = terragruntexec.Binary

// terragruntHost runs terragrunt commands through the Atmos shell layer, so output is streamed through
// the UI, secrets are masked, interrupts are forwarded, and exit codes survive.
type terragruntHost struct {
	executor    *Executor
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	opts        []component.ExecutionOption
}

// Version asks the executable for its version in the same environment the unit will run in.
func (h *terragruntHost) Version(ctx context.Context, binary string, env []string) (string, error) {
	defer perf.Track(h.atmosConfig, "terraform.terragruntHost.Version")()

	command := exec.CommandContext(ctx, binary, "--version")
	base := h.info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	command.Env = append(append([]string{}, base...), env...)
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// Execute runs one terragrunt command to completion.
func (h *terragruntHost) Execute(_ context.Context, command *terragruntexec.Command) error {
	defer perf.Track(h.atmosConfig, "terraform.terragruntHost.Execute")()

	opts := append([]component.ExecutionOption{}, h.opts...)
	opts = append(opts, component.WithEnvironment(h.info.SanitizedEnv))
	return h.executor.services.ExecuteShellCommand(*h.atmosConfig, command.Binary, command.Args, command.WorkingDir, command.Env, command.DryRun, h.info.RedirectStdErr, opts...)
}

// terragruntUnitRun carries how ExecuteTerraform reached executable mode.
type terragruntUnitRun struct {
	// shouldProcess reports whether stacks were processed for the component.
	shouldProcess bool
	// runHooks runs the before-hooks that wait for preparation; false when test fixtures already ran them.
	runHooks bool
}

// executeTerragruntUnit runs a unit in executable mode: one pinned `terragrunt` command in the unit
// directory, with the authentication, toolchain, and environment Atmos resolved for the component.
// Native preparation, generated files, varfiles, and backend files do not apply; Terragrunt owns
// hooks, error handling, excludes, and IAM roles here.
func (e *Executor) executeTerragruntUnit(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, run terragruntUnitRun, opts ...component.ExecutionOption) error {
	if err := terragruntexec.CheckSupported(info); err != nil {
		return err
	}
	if err := checkComponentRestrictions(info); err != nil {
		return err
	}
	tenv, err := e.prepareConfigurationEnvironment(ctx, atmosConfig, info, run.shouldProcess)
	if err != nil {
		return err
	}
	// Persist the auth context so PostRunE hooks, such as store hooks that read outputs, use the
	// identity this unit ran with instead of ambient credentials.
	SetLastAuthContext(info.AuthContext, info.AuthManager)
	// Before-hooks that wait for preparation (init, destroy, test) run here, as in native mode, so a
	// hook that rejects the operation stops Terragrunt from running.
	if run.runHooks {
		if err := runPreparedTerraformHooks(atmosConfig, info, opts...); err != nil {
			return err
		}
	}
	binary := terragruntTool
	if tenv != nil {
		binary = tenv.Resolve(binary)
	}
	configuration, _ := info.ComponentSettingsSection["configuration"].(map[string]any)
	unit, _ := configuration["source"].(string)
	provider, _ := configuration["provider"].(string)
	section := providerSection(atmosConfig, provider)
	features, _ := atmosConfig.Components.Terraform.ConfigurationProviders[section].Options["features"].(map[string]any)
	request := &terragruntexec.Request{
		Component: info.ComponentFromArg, Stack: info.Stack, Binary: binary, Engine: info.Command,
		Unit: unit, Root: atmosConfig.BasePathAbsolute, SubCommand: info.SubCommand, SubCommand2: info.SubCommand2,
		Arguments: info.AdditionalArgsAndFlags, Options: info.ComponentProviderOptions[section], Features: features,
		Environment: info.ComponentEnvList, DryRun: info.DryRun, SkipInit: info.SkipInit,
	}
	err = terragruntexec.Run(ctx, &terragruntHost{executor: e, atmosConfig: atmosConfig, info: info, opts: opts}, request)
	info.ExecMetadataRawExitCode = resolveExitCode(err)
	if err = e.finishMainTerraformCommand(atmosConfig, info, err, false); err != nil {
		return err
	}
	if !info.DryRun {
		invalidateTerraformStateCache(info.Stack, info.ComponentFromArg)
		tfoutput.InvalidateComponentOutputs(info.Stack, info.ComponentFromArg)
	}
	return nil
}

// providerSection returns the key under `components.terraform` that enabled the provider.
func providerSection(atmosConfig *schema.AtmosConfiguration, provider string) string {
	for section, settings := range atmosConfig.Components.Terraform.ConfigurationProviders {
		name := section
		if settings.Provider != "" {
			name = settings.Provider
		}
		if settings.Enabled && name == provider {
			return section
		}
	}
	return provider
}
