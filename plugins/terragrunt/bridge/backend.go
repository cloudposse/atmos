package atmosbridge

import (
	"maps"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
)

// BackendConfig strips upstream bootstrap-only settings without creating cloud
// resources. Values remain native so the host can generate JSON backend blocks.
func BackendConfig(remote *remotestate.Config) map[string]any {
	var values map[string]any
	switch remote.BackendName {
	case "s3":
		values = s3.Config(remote.BackendConfig).GetTFInitArgs()
		// Upstream init arguments encode nested blocks as HCL strings. Preserve
		// their original structure for Atmos's JSON backend generator instead.
		for _, key := range []string{"assume_role", "assume_role_with_web_identity"} {
			if original, ok := remote.BackendConfig[key].(map[string]any); ok {
				values[key] = maps.Clone(original)
			}
		}
	case "gcs":
		values = gcs.Config(remote.BackendConfig).GetTFInitArgs()
	case "azurerm":
		values = azurerm.Config(remote.BackendConfig).GetTFInitArgs()
	default:
		values = maps.Clone(remote.BackendConfig)
	}
	return values
}
