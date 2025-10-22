package terraform_backend

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ReadTerraformBackendFunc defines a function type to read Terraform state from a backend.
// The authContext parameter is optional and provides Atmos-managed auth credentials.
type ReadTerraformBackendFunc func(*schema.AtmosConfiguration, *map[string]any, *schema.AuthContext) ([]byte, error)

// terraformBackends is a map of backend types to the functions to read Terraform state.
var terraformBackends = map[string]ReadTerraformBackendFunc{}

// RegisterTerraformBackends registers Terraform backends.
func RegisterTerraformBackends() {
	defer perf.Track(nil, "terraform_backend.RegisterTerraformBackends")()

	// Register only once.
	if len(terraformBackends) > 0 {
		return
	}

	terraformBackends[cfg.BackendTypeLocal] = ReadTerraformBackendLocal
	terraformBackends[cfg.BackendTypeS3] = ReadTerraformBackendS3

	// Add other backends once they are implemented.
	terraformBackends[cfg.BackendTypeAzurerm] = nil
	terraformBackends[cfg.BackendTypeGCS] = nil
}

// GetTerraformBackendReadFunc accepts a backend type and returns a function to read the state file from the backend.
func GetTerraformBackendReadFunc(backendType string) func(*schema.AtmosConfiguration, *map[string]any, *schema.AuthContext) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.GetTerraformBackendReadFunc")()

	if backendFunc, ok := terraformBackends[backendType]; ok {
		return backendFunc
	}
	return nil
}
