package terraform

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/terragruntexec"
	"github.com/cloudposse/atmos/pkg/dependencies"
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
	lookupErr   error
}

// Version asks the executable for its version in the same environment the unit will run in.
func (h *terragruntHost) Version(ctx context.Context, binary string, env []string) (string, error) {
	defer perf.Track(h.atmosConfig, "terraform.terragruntHost.Version")()

	if h.lookupErr != nil {
		return "", h.lookupErr
	}
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
	binary, lookupErr := terragruntExecutable(tenv, info)
	request := terragruntRequest(atmosConfig, info, binary)
	// Scope the capture to this run, so the exec-metadata report carries its stdout and stderr.
	var stdout, stderr bytes.Buffer
	hostOpts := append(slices.Clone(opts), component.WithMetadataOutputCapture(&stdout, &stderr))
	err = terragruntexec.Run(ctx, &terragruntHost{executor: e, atmosConfig: atmosConfig, info: info, opts: hostOpts, lookupErr: lookupErr}, request)
	info.ExecMetadataRawOutput = combineExecMetadataOutput(&stdout, &stderr)
	info.ExecMetadataRawExitCode = resolveExitCode(err)
	if !info.DryRun {
		invalidateTerraformStateCache(info.Stack, info.ComponentFromArg)
		tfoutput.InvalidateComponentOutputs(info.Stack, info.ComponentFromArg)
	}
	return e.finishMainTerraformCommand(atmosConfig, info, err, false)
}

func terragruntExecutable(tenv *dependencies.ToolchainEnvironment, info *schema.ConfigAndStacksInfo) (string, error) {
	binary := terragruntTool
	if tenv != nil {
		if pinned, ok := tenv.ResolvePinned(binary); ok {
			binary = pinned
		}
	}
	if filepath.IsAbs(binary) {
		return binary, nil
	}
	resolved, err := resolveTerragruntPath(binary, info.SanitizedEnv, info.ComponentEnvList)
	if err != nil {
		return binary, err
	}
	return resolved, nil
}

func terragruntRequest(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, binary string) *terragruntexec.Request {
	configuration, _ := info.ComponentSettingsSection["configuration"].(map[string]any)
	unit, _ := configuration["source"].(string)
	provider, _ := configuration["provider"].(string)
	section := providerSection(atmosConfig, provider)
	features, _ := atmosConfig.Components.Terraform.ConfigurationProviders[section].Options["features"].(map[string]any)
	return &terragruntexec.Request{
		Component: info.ComponentFromArg, Stack: info.Stack, Binary: binary, Engine: info.Command,
		Unit: unit, Root: atmosConfig.BasePathAbsolute, SubCommand: info.SubCommand, SubCommand2: info.SubCommand2,
		Arguments: info.AdditionalArgsAndFlags, Options: info.ComponentProviderOptions[section], Features: features,
		Environment: info.ComponentEnvList, DryRun: info.DryRun,
		SkipInit: info.SkipInit || (info.SubCommand == "plan" && atmosConfig.Components.Terraform.EffectiveInitMode() == schema.TerraformInitModeNever),
	}
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

// resolveTerragruntPath searches the same environment passed to the subprocess.
func resolveTerragruntPath(binary string, base, overrides []string) (string, error) {
	if base == nil {
		base = os.Environ()
	}
	path, _ := lastEnvListValue(base)
	if value, ok := lastEnvListValue(overrides); ok {
		path = value
	}
	if strings.ContainsAny(binary, `/\`) {
		return filepath.Abs(binary)
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		candidate, err := filepath.Abs(filepath.Join(dir, binary))
		if err != nil {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("%s: %w", binary, exec.ErrNotFound)
}
