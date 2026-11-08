package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// shouldProcessStacks determines whether to process stacks and check stack configuration.
// Based on the command type and provided arguments.
func shouldProcessStacks(info *schema.ConfigAndStacksInfo) (shouldProcess bool, shouldCheckStack bool) {
	// For clean command, special logic applies.
	if info.SubCommand == "clean" {
		// Only process if component is provided.
		shouldProcess = info.ComponentFromArg != ""
		// Only check stack if stack is provided.
		shouldCheckStack = info.Stack != ""
		return shouldProcess, shouldCheckStack
	}

	// For all other commands, always process and check stack.
	return true, true
}

// Preserve the historical CLI error text during the component extraction.
//
//nolint:staticcheck // ST1005: changing capitalization would change existing command output.
var errTerraformBasePathMissing = errors.New("Base path to terraform components must be provided in 'components.terraform.base_path' config or " +
	"'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV variable")

//nolint:gocritic // Preserve the existing value-based configuration contract.
func checkTerraformConfig(atmosConfig schema.AtmosConfiguration) error {
	if len(atmosConfig.Components.Terraform.BasePath) < 1 {
		return errTerraformBasePathMissing
	}

	return nil
}

// cleanTerraformWorkspace delegates workspace cleanup to the Terraform component.
//
//nolint:gocritic // Preserve the existing value-based workspace cleanup contract.
func cleanTerraformWorkspace(_ schema.AtmosConfiguration, componentPath string) {
	CleanTerraformWorkspace(componentPath)
}

// isTerraformCurrentWorkspace reports whether the given workspace name matches the workspace
// recorded in the .terraform/environment file inside componentPath.
// This is used to detect the edge case where the environment file already names the target
// workspace but the corresponding state directory was deleted (e.g. by a previous test or a
// partial cleanup on Windows).  In that scenario `terraform workspace new <name>` returns exit
// code 1 even though we are already in the right workspace, so we should not treat the failure
// as a fatal error.
//
// TF_DATA_DIR resolution: the envList parameter carries the subprocess env vars (typically
// info.ComponentEnvList).  If TF_DATA_DIR is set there, it takes precedence over the parent
// process env, ensuring this helper reads the same data directory that the terraform subprocess
// would use.  A relative TF_DATA_DIR is joined to componentPath, matching Terraform's own
// resolution relative to the process working directory.
func (e *Executor) isTerraformCurrentWorkspace(componentPath, workspace string, envList []string) bool {
	tfDataDir := e.services.EnvVarFromList(envList, "TF_DATA_DIR")
	if tfDataDir == "" {
		//nolint:forbidigo // TF_DATA_DIR is a Terraform convention, not an Atmos config var.
		tfDataDir = os.Getenv("TF_DATA_DIR")
	}
	if tfDataDir == "" {
		tfDataDir = ".terraform"
	}
	if !filepath.IsAbs(tfDataDir) {
		tfDataDir = filepath.Join(componentPath, tfDataDir)
	}
	envFile := filepath.Join(filepath.Clean(tfDataDir), "environment")
	data, err := os.ReadFile(envFile)
	if err != nil {
		// Only treat a missing file as "default workspace active".
		// Other errors (permission denied, I/O error) are not equivalent
		// to the workspace being "default" and must return false.
		if errors.Is(err, os.ErrNotExist) && workspace == "default" {
			return true
		}
		return false
	}
	// An empty file also indicates the default workspace.
	recorded := strings.TrimSpace(string(data))
	if recorded == "" {
		return workspace == "default"
	}
	return recorded == workspace
}

// generateBackendConfig delegates artifact generation to the Terraform component.
func generateBackendConfig(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	return GenerateBackendConfig(config, info, workingDir)
}

// generateProviderOverrides delegates artifact generation to the Terraform component.
func generateProviderOverrides(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	return GenerateProviderOverrides(config, info, workingDir)
}

// generateRequiredProviders delegates artifact generation to the Terraform component.
func generateRequiredProviders(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	return GenerateRequiredProviders(config, info, workingDir)
}

// needProcessTemplatesAndYamlFunctions checks if a Terraform command requires the `Go` templates and Atmos YAML functions to be processed.
func needProcessTemplatesAndYamlFunctions(command string) bool {
	commandsThatNeedFuncProcessing := []string{
		"init",
		"plan",
		"apply",
		"deploy",
		"destroy",
		"generate",
		"output",
		"clean",
		"shell",
		"write",
		"force-unlock",
		"import",
		"refresh",
		"show",
		"taint",
		"untaint",
		"validate",
		"state list",
		"state mv",
		"state pull",
		"state push",
		"state replace-provider",
		"state rm",
		"state show",
	}
	return slices.Contains(commandsThatNeedFuncProcessing, command)
}

// isWorkspacesEnabled checks if Terraform workspaces are enabled for a component.
// Workspaces are enabled by default except for:
// 1. When explicitly disabled via workspaces_enabled: false in `atmos.yaml`.
// 2. When using HTTP backend (which doesn't support workspaces).
func isWorkspacesEnabled(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	return naming.WorkspacesEnabled(atmosConfig, info)
}

// walkTerraformComponents iterates over all Terraform components in the provided stacks map.
// For each component it calls the provided function, stopping if the function returns an error.
func walkTerraformComponents(
	stacks map[string]any,
	fn func(stackName, componentName string, componentSection map[string]any) error,
) error {
	for stackName, stackSection := range stacks {
		stackSectionMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}

		componentsSection, ok := stackSectionMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}

		terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any)
		if !ok {
			continue
		}

		for componentName, compSection := range terraformSection {
			componentSection, ok := compSection.(map[string]any)
			if !ok {
				continue
			}

			if err := fn(stackName, componentName, componentSection); err != nil {
				return err
			}
		}
	}

	return nil
}

// ComponentStack identifies a concrete terraform component in a stack.
type ComponentStack struct {
	Component string
	Stack     string
}

// ListTerraformComponentTargets returns the concrete, enabled terraform components
// to act on, filtered by stack, an explicit component list, and an optional YQ query.
// Order is unspecified — it suits order-independent operations like cache mirroring
// (which, unlike plan/apply, has no inter-component dependencies). Auth and YAML
// functions are disabled: only structural component selection is needed.
func (e *Executor) ListTerraformComponentTargets(atmosConfig *schema.AtmosConfiguration, filterStack string, components []string, query string) ([]ComponentStack, error) {
	defer perf.Track(atmosConfig, "exec.ListTerraformComponentTargets")()

	stacks, err := e.services.ExecuteDescribeStacksWithAuthDisabled(
		atmosConfig, filterStack, components, []string{cfg.TerraformComponentType},
		nil, false, false, false, false, nil, nil, true,
	)
	if err != nil {
		return nil, err
	}

	var targets []ComponentStack
	walkErr := walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		include, err := e.mirrorTargetIncluded(atmosConfig, componentName, componentSection, query)
		if err != nil {
			return err
		}
		if include {
			targets = append(targets, ComponentStack{Component: componentName, Stack: stackName})
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// targets are built from map iteration, whose order varies across processes. Sort
	// them so mirror execution order and --format json|yaml output are deterministic
	// (and snapshot-stable).
	sortComponentStacks(targets)

	return targets, nil
}

// sortComponentStacks orders targets deterministically by stack, then component.
func sortComponentStacks(targets []ComponentStack) {
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Stack != targets[j].Stack {
			return targets[i].Stack < targets[j].Stack
		}
		return targets[i].Component < targets[j].Component
	})
}

// mirrorTargetIncluded reports whether a component should be a mirror target: it must
// be concrete (not abstract), enabled, and match the optional YQ query.
func (e *Executor) mirrorTargetIncluded(atmosConfig *schema.AtmosConfiguration, componentName string, componentSection map[string]any, query string) (bool, error) {
	if metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
		if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
			return false, nil
		}
		if !e.services.IsComponentEnabled(metadataSection, componentName) {
			return false, nil
		}
	}
	if query == "" {
		return true, nil
	}
	queryResult, err := u.EvaluateYqExpression(atmosConfig, componentSection, query)
	if err != nil {
		return false, err
	}
	passed, ok := queryResult.(bool)
	return ok && passed, nil
}

// parseUploadStatusFlag parses the upload status flag from the arguments.
// It supports --flag, --flag=true, and --flag=false forms.
// Returns true if the flag is present and not explicitly set to false.
func parseUploadStatusFlag(args []string, flagName string) bool {
	flagPrefix := "--" + flagName + "="

	// Check for --flag (without value, defaults to true).
	if slices.Contains(args, "--"+flagName) {
		return true
	}

	// Check for --flag=value forms.
	for _, arg := range args {
		if strings.HasPrefix(arg, flagPrefix) {
			value := strings.TrimPrefix(arg, flagPrefix)
			// Parse boolean value, default to true if not a valid boolean.
			return value != "false"
		}
	}

	return false
}
