package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// checkHelmfileConfig validates the helmfile configuration.
// Note: AWS auth and cluster name validation moved to runtime since they can be provided via CLI flags.
func checkHelmfileConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.checkHelmfileConfig")()

	if len(atmosConfig.Components.Helmfile.BasePath) < 1 {
		return fmt.Errorf("%w: must be provided in 'components.helmfile.base_path' config or 'ATMOS_COMPONENTS_HELMFILE_BASE_PATH' ENV variable",
			errUtils.ErrMissingHelmfileBasePath)
	}

	if atmosConfig.Components.Helmfile.UseEKS {
		if len(atmosConfig.Components.Helmfile.KubeconfigPath) < 1 {
			return fmt.Errorf("%w: must be provided in 'components.helmfile.kubeconfig_path' config or 'ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH' ENV variable",
				errUtils.ErrMissingHelmfileKubeconfigPath)
		}

		// HelmAwsProfilePattern check removed - deprecated, uses --identity flag or falls back to deprecated pattern at runtime.
		// ClusterNamePattern check removed - validation moved to runtime since it can come from:
		// 1. --cluster-name flag
		// 2. cluster_name config
		// 3. cluster_name_template config (Go template syntax)
		// 4. cluster_name_pattern config (deprecated, token replacement)
	}

	return nil
}
