package kubernetes

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"

	// Blank import registers the "git" provision target kind so it is available
	// for delivery whenever Kubernetes components are executed.
	_ "github.com/cloudposse/atmos/pkg/provisioner/target/git"
)

// deliveryTimeout bounds an external target delivery (clone + commit + push).
const deliveryTimeout = 10 * time.Minute

// deliverApply resolves the selected provision target for an apply/deploy and
// delivers the rendered objects to it. The implicit/selected "kubernetes" kind
// applies directly to the cluster using the in-memory objects (no YAML round
// trip); any other kind (e.g. "git") is delivered the rendered manifests as a
// producer-agnostic ProvisionArtifact through the target registry.
func deliverApply(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	flags map[string]any,
	objects []*unstructured.Unstructured,
) ([]objectResult, error) {
	defer perf.Track(atmosConfig, "kubernetes.deliverApply")()

	provisionSection, _ := info.ComponentSection["provision"].(map[string]any)
	flagTarget, _ := flags["target"].(string)

	selected, err := target.SelectTarget(provisionSection, flagTarget)
	if err != nil {
		return nil, err
	}

	// Cluster delivery keeps the existing SDK apply path with the in-memory objects.
	if selected.Kind == target.KindKubernetes {
		return runApply(objects)
	}

	artifact, err := buildKubernetesArtifact(objects, info, selected.Name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()

	if err := target.Deliver(ctx, selected.Kind, &target.DeliverInput{
		AtmosConfig:  atmosConfig,
		TargetName:   selected.Name,
		TargetConfig: selected.Config,
		Artifact:     artifact,
		EnvProvider:  authManagerFor(info),
	}); err != nil {
		return nil, err
	}
	return objectsToResults("delivered", objects), nil
}

// buildKubernetesArtifact serializes the rendered objects into a deterministic
// per-object file set for delivery to an external target.
func buildKubernetesArtifact(
	objects []*unstructured.Unstructured,
	info *schema.ConfigAndStacksInfo,
	targetName string,
) (target.ProvisionArtifact, error) {
	files := make(map[string][]byte, len(objects))
	for i, obj := range objects {
		data, err := objectYAML(obj)
		if err != nil {
			return target.ProvisionArtifact{}, err
		}
		files[objectManifestFileName(i, obj)] = data
	}

	return target.ProvisionArtifact{
		Kind:   target.ArtifactKindKubernetesManifests,
		Format: target.FormatYAML,
		Files:  files,
		Metadata: target.ArtifactMetadata{
			Component: info.ComponentFromArg,
			Stack:     info.Stack,
			Target:    targetName,
		},
	}, nil
}

// authManagerFor returns the Atmos Auth manager as an identity-environment
// provider when one is configured, so targets that authenticate via Atmos Auth
// (e.g. the git target's GitHub STS) receive the composed environment.
func authManagerFor(info *schema.ConfigAndStacksInfo) target.IdentityEnvironmentProvider {
	if mgr, ok := info.AuthManager.(auth.AuthManager); ok {
		return mgr
	}
	return nil
}
