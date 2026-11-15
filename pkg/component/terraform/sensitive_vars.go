package terraform

import (
	"path/filepath"

	tfconfig "github.com/hashicorp/terraform-config-inspect/tfconfig"

	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	schema "github.com/cloudposse/atmos/pkg/schema"
)

// redactedVarValue replaces the value of a secret variable in debug output.
const redactedVarValue = "<redacted>"

// redactSecretVars returns a copy of vars with the values of secret-bearing and declared-sensitive
// keys replaced by a placeholder, so debug output never carries literals that are kept out of the
// varfile. The input map is never modified, and it is returned as is when nothing is secret.
func redactSecretVars(info *schema.ConfigAndStacksInfo, vars map[string]any) map[string]any {
	declared := map[string]bool(nil)
	if atmosio.MaskingEnabled() {
		declared = terraformSensitiveVarKeys(info)
	}
	if len(info.TerraformSecretVarKeys) == 0 && len(declared) == 0 {
		return vars
	}
	redacted := make(map[string]any, len(vars))
	for k, v := range vars {
		if info.TerraformSecretVarKeys[k] || declared[k] {
			v = redactedVarValue
		}
		redacted[k] = v
	}
	return redacted
}

// terraformSensitiveVarKeys returns the root-module variable names declared
// `sensitive = true`. Component metadata is populated during stack processing
// with terraform-config-inspect, so this adds no extra filesystem parsing on
// the execution hot path.
func terraformSensitiveVarKeys(info *schema.ConfigAndStacksInfo) map[string]bool {
	if info == nil {
		return nil
	}
	componentInfo, ok := info.ComponentSection["component_info"].(map[string]any)
	if !ok {
		return nil
	}
	module, ok := componentInfo["terraform_config"].(*tfconfig.Module)
	if !ok || module == nil {
		return nil
	}

	return sensitiveModuleVarKeys(module)
}

// sensitiveModuleVarKeys returns the variable names a module declares `sensitive = true`.
func sensitiveModuleVarKeys(module *tfconfig.Module) map[string]bool {
	keys := make(map[string]bool)
	for name, variable := range module.Variables {
		if variable != nil && variable.Sensitive {
			keys[name] = true
		}
	}
	return keys
}

// declaredSensitiveVarKeys returns the sensitive variable names for a component, preferring the
// module inspected during stack processing. Batch generation renders components from the raw stacks
// map, which has no component_info, so it falls back to inspecting the component directory.
func declaredSensitiveVarKeys(info *schema.ConfigAndStacksInfo, componentPath string) map[string]bool {
	if keys := terraformSensitiveVarKeys(info); keys != nil {
		return keys
	}
	module, diags := tfconfig.LoadModule(componentPath)
	if diags.HasErrors() || module == nil {
		log.Debug("Could not inspect the Terraform module for sensitive variables", "path", componentPath, "error", diags.Err())
		return nil
	}
	return sensitiveModuleVarKeys(module)
}

// refreshTerraformModuleInfo reloads component_info.terraform_config from the provisioned
// component directory when stack processing inspected a different or missing directory, as
// happens for JIT sources and workdirs. Sensitive-variable detection then reflects the module
// Terraform will actually run. A module that cannot be inspected leaves the existing value.
func refreshTerraformModuleInfo(info *schema.ConfigAndStacksInfo, componentPath string) {
	componentInfo, ok := info.ComponentSection["component_info"].(map[string]any)
	if !ok {
		return
	}
	module, _ := componentInfo["terraform_config"].(*tfconfig.Module)
	recordedPath, _ := componentInfo[cfg.ComponentPathSectionName].(string)
	if module != nil && filepath.Clean(recordedPath) == filepath.Clean(componentPath) {
		return
	}
	loadTerraformModuleInfo(info, componentPath)
}

// loadTerraformModuleInfo unconditionally reloads component_info.terraform_config from the
// component directory. A module that cannot be inspected leaves the existing value.
func loadTerraformModuleInfo(info *schema.ConfigAndStacksInfo, componentPath string) {
	componentInfo, ok := info.ComponentSection["component_info"].(map[string]any)
	if !ok {
		return
	}
	refreshed, diags := tfconfig.LoadModule(componentPath)
	if diags.HasErrors() || refreshed == nil {
		log.Debug("Could not inspect the provisioned Terraform module", "path", componentPath, "error", diags.Err())
		return
	}
	componentInfo["terraform_config"] = refreshed
}
