package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// checkHelmfileConfig validates the helmfile configuration.
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

		if len(atmosConfig.Components.Helmfile.HelmAwsProfilePattern) < 1 {
			return fmt.Errorf("%w: must be provided in 'components.helmfile.helm_aws_profile_pattern' config or 'ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN' ENV variable",
				errUtils.ErrMissingHelmfileAwsProfilePattern)
		}

		if len(atmosConfig.Components.Helmfile.ClusterNamePattern) < 1 {
			return fmt.Errorf("%w: must be provided in 'components.helmfile.cluster_name_pattern' config or 'ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN' ENV variable",
				errUtils.ErrMissingHelmfileClusterNamePattern)
		}
	}

	return nil
}
