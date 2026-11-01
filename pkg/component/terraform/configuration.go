package terraform

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func prepareConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return nil, nil
	}
	if config.ComponentConfigurations == nil {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	if info.DryRun {
		return nil, nil
	}
	// Plugins do not inherit PATH. Resolve the selected host executable before
	// Terragrunt evaluates version constraints or dependency outputs.
	binary, err := exec.LookPath(info.Command)
	if err != nil {
		return nil, fmt.Errorf("%w: Terraform executable: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	info.Command = binary
	var prepared *schema.PreparedComponentConfiguration
	if usesConfigurationPlan(info) {
		if err = useRetainedWorkdir(config, info); err != nil {
			return nil, err
		}
		prepared, err = restoreConfigurationPlan(ctx, config, info)
	} else {
		prepared, err = config.ComponentConfigurations.Prepare(ctx, info)
	}
	if err != nil || prepared == nil {
		return prepared, err
	}
	info.ComponentVarsSection = prepared.Inputs
	info.ComponentExecution = prepared.Execution
	info.ComponentBackendType, info.ComponentBackendSection = prepared.BackendType, prepared.Backend
	info.TerraformWorkspace = prepared.Workspace
	info.ComponentSection["vars"] = prepared.Inputs
	source, _ := info.ComponentSection["source"].(map[string]any)
	source = maps.Clone(source)
	if source == nil {
		source = make(map[string]any)
	}
	source["uri"] = prepared.Source
	if isLocalModuleSource(prepared.Source) {
		// A local module changes without its location changing, so the retained workdir
		// would keep the code of the first run. Refreshing is a cheap local copy.
		source["ttl"] = "0s"
	}
	info.ComponentSection["source"] = source
	info.ComponentSection["backend_type"] = prepared.BackendType
	info.ComponentSection["backend"] = prepared.Backend
	config.Components.Terraform.AutoGenerateBackendFile = !prepared.BackendGenerated
	return prepared, nil
}

// isLocalModuleSource reports whether the evaluated module source is a path on this machine.
func isLocalModuleSource(uri string) bool {
	return strings.HasPrefix(uri, ".") || filepath.IsAbs(uri)
}

// useRetainedWorkdir points the invocation at the directory the reviewed plan was created in.
// A new process has not provisioned anything, so the path is derived the way provisioning does.
func useRetainedWorkdir(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	fallback, err := u.GetComponentPath(config, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}
	_, err = component.ResolveRetainedWorkdir(config, info, cfg.TerraformComponentType, fallback)
	return err
}

func configurationArtifactName(info *schema.ConfigAndStacksInfo, suffix string) (string, bool) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return "", false
	}
	identity := sha256.Sum256([]byte(info.Stack + "\x00" + info.ComponentFromArg))
	return fmt.Sprintf("atmos-%x%s", identity, suffix), true
}

// prepareConfigurationEnvironment applies the same authentication, RC, cache,
// and toolchain environment as Terraform before the evaluator can spawn children.
func (e *Executor) prepareConfigurationEnvironment(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, shouldProcess bool) (*dependencies.ToolchainEnvironment, error) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned || info.DryRun {
		return nil, nil
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.prepareAuthEnvironmentFn(config, info); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tenv *dependencies.ToolchainEnvironment
	if shouldProcess {
		var err error
		tenv, err = e.resolveConfigurationToolsFn(config, info)
		if err != nil {
			return nil, err
		}
		info.Command = tenv.Resolve(info.Command)
	}
	disableTerraformPluginCacheForExecution(config, info)
	if err := assembleComponentEnvVars(config, info, tenv); err != nil {
		return nil, err
	}
	return tenv, nil
}
