package helm

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// deliveryTimeout bounds an external target delivery (clone + commit + push).
const deliveryTimeout = 10 * time.Minute

// targetKey is the shared key for the `--target` flag and the summary entry that
// records the resolved provision target.
const targetKey = "target"

// deliverApply resolves the selected provision target for an apply/deploy and
// delivers to it. The implicit/selected "kubernetes" (cluster) kind installs or
// upgrades the Helm release directly; any other kind (e.g. "git") receives the
// rendered manifests as a producer-agnostic ProvisionArtifact via the registry.
func deliverApply(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	flags map[string]any,
	spec *chartSpec,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "helm.deliverApply")()

	summary := map[string]any{}
	provisionSection, _ := info.ComponentSection["provision"].(map[string]any)
	flagTarget, _ := flags[targetKey].(string)

	selected, err := target.SelectTarget(provisionSection, flagTarget)
	if err != nil {
		return summary, err
	}
	summary[targetKey] = selected.Name
	if summary[targetKey] == "" {
		summary[targetKey] = selected.Kind
	}

	// Cluster delivery installs/upgrades the Helm release directly.
	if selected.Kind == target.KindKubernetes {
		rendered, err := applyHelmRelease(context.Background(), spec, false)
		summary["manifest_bytes"] = len(rendered)
		if objects, decodeErr := manifest.DecodeObjects([]byte(rendered)); decodeErr == nil {
			addObjectsToSummary(summary, objects)
		}
		return summary, err
	}

	return deliverToExternalTarget(atmosConfig, info, selected, spec, summary)
}

// deliverToExternalTarget renders the Helm release to manifests and delivers them
// to a non-cluster provision target (e.g. a Git deployment repository) as a
// producer-agnostic ProvisionArtifact via the target registry.
func deliverToExternalTarget(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	selected *target.SelectedTarget,
	spec *chartSpec,
	summary map[string]any,
) (map[string]any, error) {
	objects, err := renderObjects(spec)
	if err != nil {
		return summary, err
	}
	addObjectsToSummary(summary, objects)
	files, err := manifest.ArtifactFiles(objects)
	if err != nil {
		return summary, err
	}
	summary["manifest_bytes"] = totalManifestBytes(files)

	artifact := target.ProvisionArtifact{
		Kind:   target.ArtifactKindKubernetesManifests,
		Format: target.FormatYAML,
		Files:  files,
		Metadata: target.ArtifactMetadata{
			Component: info.ComponentFromArg,
			Stack:     info.Stack,
			Target:    selected.Name,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()

	return summary, target.Deliver(ctx, selected.Kind, &target.DeliverInput{
		AtmosConfig:  atmosConfig,
		TargetName:   selected.Name,
		TargetConfig: selected.Config,
		Artifact:     artifact,
		EnvProvider:  authManagerFor(info),
	})
}

// totalManifestBytes sums the byte length of all rendered manifest files.
func totalManifestBytes(files map[string][]byte) int {
	total := 0
	for _, data := range files {
		total += len(data)
	}
	return total
}

// authManagerFor returns the Atmos Auth manager as an identity-environment
// provider when configured, so targets that authenticate via Atmos Auth receive
// the composed environment.
func authManagerFor(info *schema.ConfigAndStacksInfo) target.IdentityEnvironmentProvider {
	if mgr, ok := info.AuthManager.(auth.AuthManager); ok {
		return mgr
	}
	return nil
}
