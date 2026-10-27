package exec

import (
	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// IsOpenTofu is the compatibility entry point for Terraform tool detection.
func IsOpenTofu(config *schema.AtmosConfiguration, tools *dependencies.ToolchainEnvironment) bool {
	defer perf.Track(nil, "exec.IsOpenTofu")()

	return terraformcomponent.IsOpenTofu(config, tools)
}

// allDiagnosticsAreModuleSourceInterpolation identifies module-source interpolation diagnostics that Atmos can tolerate.
func allDiagnosticsAreModuleSourceInterpolation(diagnostics tfconfig.Diagnostics) bool {
	return terraformcomponent.AllDiagnosticsAreModuleSourceInterpolation(diagnostics)
}

// GetTerraformEnvCliArgs parses Terraform's CLI environment arguments.
func GetTerraformEnvCliArgs() []string {
	defer perf.Track(nil, "exec.GetTerraformEnvCliArgs")()

	return terraformcomponent.GetTerraformEnvCliArgs()
}

// GetTerraformEnvCliVars resolves variable assignments from Terraform's CLI environment arguments.
func GetTerraformEnvCliVars() (map[string]any, error) {
	defer perf.Track(nil, "exec.GetTerraformEnvCliVars")()

	return terraformcomponent.GetTerraformEnvCliVars()
}

// SetLastAuthContext records credentials for post-execution hooks.
func SetLastAuthContext(ctx *schema.AuthContext, manager any) {
	defer perf.Track(nil, "exec.SetLastAuthContext")()

	terraformcomponent.SetLastAuthContext(ctx, manager)
}

// GetLastAuthContext retrieves credentials recorded for post-execution hooks.
func GetLastAuthContext() (*schema.AuthContext, any) {
	defer perf.Track(nil, "exec.GetLastAuthContext")()

	return terraformcomponent.GetLastAuthContext()
}

// ClearLastAuthContext clears post-execution credentials.
func ClearLastAuthContext() {
	defer perf.Track(nil, "exec.ClearLastAuthContext")()

	terraformcomponent.ClearLastAuthContext()
}

// TerraformDeclaredVarType returns a variable's declared HCL type.
func TerraformDeclaredVarType(section map[string]any, name string) (string, bool) {
	defer perf.Track(nil, "exec.TerraformDeclaredVarType")()

	return terraformcomponent.TerraformDeclaredVarType(section, name)
}

// GetComponentRemoteStateBackendStaticType returns configured static state outputs.
func GetComponentRemoteStateBackendStaticType(sections *map[string]any) map[string]any {
	defer perf.Track(nil, "exec.GetComponentRemoteStateBackendStaticType")()

	return terraformcomponent.GetComponentRemoteStateBackendStaticType(sections)
}
