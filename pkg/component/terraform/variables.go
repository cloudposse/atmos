package terraform

// terraform_execute_helpers.go contains helper functions extracted from ExecuteTerraform
// to reduce cyclomatic complexity and improve testability.
// Each function handles one discrete responsibility of the terraform execution pipeline.

import (
	"fmt"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/degradation"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	_ "github.com/cloudposse/atmos/pkg/provisioner/lock"   // register after.terraform.init providers-lock provisioner
	_ "github.com/cloudposse/atmos/pkg/provisioner/source" // register source provisioner
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/tfvars"
)

// ComputeTerraformSecretVarKeys records the set of top-level variable keys whose value
// contains a resolved secret, so they can be kept out of the on-disk varfile and injected
// as TF_VAR_<name> environment variables instead. It must run after secret resolution
// (ProcessStacks) and before auth credentials are registered with the masker, so that the
// varfile-write and env-assembly steps partition the variables identically.
func ComputeTerraformSecretVarKeys(info *schema.ConfigAndStacksInfo) {
	defer perf.Track(nil, "terraform.ComputeTerraformSecretVarKeys")()

	_, secret := tfvars.Partition(info.ComponentVarsSection, atmosio.ContainsSecret)
	keys := make(map[string]bool, len(secret))
	for k := range secret {
		keys[k] = true
	}

	// Terraform's `sensitive = true` is an explicit declaration that an input must
	// not be persisted in a generated tfvars file. When masking is disabled, retain
	// the established compatibility behavior that keeps typed sensitive values in
	// JSON rather than coercing them through TF_VAR_ environment variables.
	if atmosio.MaskingEnabled() {
		for k := range terraformSensitiveVarKeys(info) {
			if _, supplied := info.ComponentVarsSection[k]; supplied {
				keys[k] = true
			}
		}
	}

	if len(keys) == 0 {
		info.TerraformSecretVarKeys = nil
		return
	}
	info.TerraformSecretVarKeys = keys
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

	keys := make(map[string]bool)
	for name, variable := range module.Variables {
		if variable != nil && variable.Sensitive {
			keys[name] = true
		}
	}
	return keys
}

// RejectComputedTerraformVars ensures display-only degraded values cannot cross
// into Terraform through either tfvars JSON or TF_VAR_ environment variables.
func RejectComputedTerraformVars(vars map[string]any) error {
	defer perf.Track(nil, "terraform.RejectComputedTerraformVars")()

	for key, value := range vars {
		if containsComputedTerraformValue(value) {
			return fmt.Errorf("%w: %q", errUtils.ErrUnresolvedComputedTerraformVar, key)
		}
	}
	return nil
}

// containsComputedTerraformValue recursively detects display-only placeholders in Terraform input values.
func containsComputedTerraformValue(value any) bool {
	switch v := value.(type) {
	case degradation.AtmosComputedValue:
		return true
	case *degradation.AtmosComputedValue:
		return v != nil
	case map[string]any:
		for _, child := range v {
			if containsComputedTerraformValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsComputedTerraformValue(child) {
				return true
			}
		}
	}
	return false
}

// DiskSafeVars returns ComponentVarsSection with secret-bearing top-level keys removed,
// so resolved secrets are never written to the on-disk varfile. Returns the original map
// unchanged when no secret-bearing variables were detected.
func DiskSafeVars(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "terraform.DiskSafeVars")()

	testSecretVars := terraformTestSecretVars(info)
	if len(info.TerraformSecretVarKeys) == 0 && len(testSecretVars) == 0 {
		return info.ComponentVarsSection
	}
	safe := make(map[string]any, len(info.ComponentVarsSection))
	for k, v := range info.ComponentVarsSection {
		if info.TerraformSecretVarKeys[k] {
			continue
		}
		if _, collidesWithSecretTestVar := testSecretVars[k]; collidesWithSecretTestVar {
			continue
		}
		safe[k] = v
	}
	return safe
}

// varsWithoutKeys removes excluded top-level variables, returning the original map when no exclusions exist.
func varsWithoutKeys(vars map[string]any, excluded map[string]bool) map[string]any {
	if len(excluded) == 0 {
		return vars
	}
	// `excluded` can include declared inputs that this stack does not supply, so
	// use the source size as the allocation hint rather than subtracting it.
	safe := make(map[string]any, len(vars))
	for k, v := range vars {
		if !excluded[k] {
			safe[k] = v
		}
	}
	return safe
}

// SecretVarEnv returns the TF_VAR_<name> environment entries for the component's
// secret-bearing variables, for injection into the subprocess environment.
func SecretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	defer perf.Track(nil, "terraform.SecretVarEnv")()

	if len(info.TerraformSecretVarKeys) == 0 {
		return nil, nil
	}
	secret := make(map[string]any, len(info.TerraformSecretVarKeys))
	for k := range info.TerraformSecretVarKeys {
		if v, ok := info.ComponentVarsSection[k]; ok {
			secret[k] = v
		}
	}
	return tfvars.SecretEnv(secret)
}

// TerraformTestVars returns test.vars for the test subcommand, or nil for other commands and absent values.
func TerraformTestVars(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "terraform.TerraformTestVars")()

	if info == nil || info.SubCommand != "test" {
		return nil
	}
	testSection, ok := info.ComponentSection[cfg.TestSectionName].(map[string]any)
	if !ok {
		return nil
	}
	vars, ok := testSection[cfg.VarsSectionName].(map[string]any)
	if !ok || len(vars) == 0 {
		return nil
	}
	return vars
}

// HasTerraformTestVars reports whether the current test command supplies additional test variables.
func HasTerraformTestVars(info *schema.ConfigAndStacksInfo) bool {
	defer perf.Track(nil, "terraform.HasTerraformTestVars")()

	return len(TerraformTestVars(info)) > 0
}

// DiskSafeTerraformTestVars excludes secret-bearing values from the variables written to the test varfile.
func DiskSafeTerraformTestVars(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "terraform.DiskSafeTerraformTestVars")()

	vars := TerraformTestVars(info)
	if len(vars) == 0 {
		return nil
	}
	safe, _ := tfvars.Partition(vars, atmosio.ContainsSecret)
	return safe
}

// terraformTestSecretVars selects test variables that must be passed through the subprocess environment.
func terraformTestSecretVars(info *schema.ConfigAndStacksInfo) map[string]any {
	vars := TerraformTestVars(info)
	if len(vars) == 0 {
		return nil
	}
	_, secret := tfvars.Partition(vars, atmosio.ContainsSecret)
	return secret
}

// TerraformTestSecretVarEnv encodes secret-bearing test variables as TF_VAR_ environment entries.
func TerraformTestSecretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	defer perf.Track(nil, "terraform.TerraformTestSecretVarEnv")()

	secret := terraformTestSecretVars(info)
	if len(secret) == 0 {
		return nil, nil
	}
	return tfvars.SecretEnv(secret)
}
