package helm

import (
	"fmt"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// The `kube_version` and `api_versions` component settings are the native-Helm
// equivalent of `helm template --kube-version` and `--api-versions`. They set
// what a client-side render reports through `.Capabilities`, so charts that gate
// objects on `.Capabilities.APIVersions.Has` (for example a ServiceMonitor) or on
// the Kubernetes version render the same offline as they would against the
// target cluster. Helm honors them only in client-only mode; install and upgrade
// against a cluster always use the live API server's capabilities.

// resolveKubeVersion reads and parses the component's `kube_version`. A
// non-string value is rejected rather than ignored: an unquoted YAML `1.30`
// decodes as the float 1.3 and would otherwise render against the wrong version.
func resolveKubeVersion(section map[string]any) (*common.KubeVersion, error) {
	value, ok := section[cfg.HelmKubeVersionSectionName]
	if !ok || value == nil {
		return nil, nil //nolint:nilnil // An unset kube_version keeps Helm's built-in default.
	}
	version, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: expected a quoted string such as \"1.30.0\", got %T", errUtils.ErrHelmKubeVersionInvalid, value)
	}
	return parseKubeVersion(version)
}

// parseKubeVersion parses a Kubernetes version string with Helm's own parser.
// An empty string means "not set".
func parseKubeVersion(version string) (*common.KubeVersion, error) {
	if version == "" {
		return nil, nil //nolint:nilnil // An empty kube_version keeps Helm's built-in default.
	}
	parsed, err := common.ParseKubeVersion(version)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", errUtils.ErrHelmKubeVersionInvalid, version, err)
	}
	return parsed, nil
}

// applyCapabilityFlags layers `--kube-version` and `--api-versions` over the
// component's settings. CLI flags take precedence, so `--api-versions` replaces
// the component's list rather than appending to it.
func applyCapabilityFlags(spec *chartSpec, flags map[string]any) error {
	if value, ok := flags[cfg.HelmKubeVersionSectionName].(string); ok && value != "" {
		kubeVersion, err := parseKubeVersion(value)
		if err != nil {
			return err
		}
		spec.KubeVersion = kubeVersion
	}
	if values, ok := flags[cfg.HelmAPIVersionsSectionName].([]string); ok && len(values) > 0 {
		spec.APIVersions = values
	}
	return nil
}

// applyRenderCapabilities wires the resolved capabilities onto a client-side
// Install action.
func applyRenderCapabilities(client *action.Install, spec *chartSpec) {
	if spec.KubeVersion != nil {
		client.KubeVersion = spec.KubeVersion
	}
	if len(spec.APIVersions) > 0 {
		client.APIVersions = common.VersionSet(spec.APIVersions)
	}
}
