package exec

import (
	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformDeclaredVarType returns the raw HCL type text (e.g. "string",
// "number", "list(string)") componentSection's resolved Terraform component
// declares for varName in its variables.tf, as already captured by
// terraform-config-inspect during stack processing (see
// terraformSensitiveVarKeys for the same map shape). Returns ("", false)
// when the component isn't Terraform, its module didn't parse, the variable
// isn't declared, or it's declared with no explicit type (implicit any).
func TerraformDeclaredVarType(componentSection map[string]any, varName string) (string, bool) {
	defer perf.Track(nil, "exec.TerraformDeclaredVarType")()

	componentInfo, ok := componentSection[componentInfoKey].(map[string]any)
	if !ok {
		return "", false
	}
	module, ok := componentInfo[terraformConfigKey].(*tfconfig.Module)
	if !ok || module == nil {
		return "", false
	}
	variable, ok := module.Variables[varName]
	if !ok || variable == nil || variable.Type == "" {
		return "", false
	}
	return variable.Type, true
}
