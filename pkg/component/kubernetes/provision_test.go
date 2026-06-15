package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthManagerForReturnsNilWhenNoManager(t *testing.T) {
	// No auth manager is set on the info, so no environment provider is supplied.
	assert.Nil(t, authManagerFor(&schema.ConfigAndStacksInfo{}))

	// A value that is not an auth.AuthManager must not be treated as one.
	assert.Nil(t, authManagerFor(&schema.ConfigAndStacksInfo{AuthManager: "not-a-manager"}))
}

func TestAuthManagerForReturnsConfiguredManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockManager := authtypes.NewMockAuthManager(ctrl)

	provider := authManagerFor(&schema.ConfigAndStacksInfo{AuthManager: mockManager})
	require.NotNil(t, provider)
	assert.Equal(t, target.IdentityEnvironmentProvider(mockManager), provider)
}

// captureProvisioner records the DeliverInput it receives so tests can assert
// the executor built and routed the artifact correctly.
type captureProvisioner struct {
	last *target.DeliverInput
}

func (c *captureProvisioner) Deliver(_ context.Context, in *target.DeliverInput) error {
	c.last = in
	return nil
}

func newObject(kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
	}}
}

func TestDeliverApplyRoutesToExternalTarget(t *testing.T) {
	const kind = "test-capture-kind"
	capture := &captureProvisioner{}
	target.Register(kind, capture)

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "argocd",
		Stack:            "dev",
		ComponentSection: map[string]any{
			"provision": map[string]any{
				"targets": map[string]any{
					"deployment-repo": map[string]any{
						"kind": kind,
						"path": "clusters/dev/argocd",
					},
				},
			},
		},
	}
	flags := map[string]any{"target": "deployment-repo"}
	objects := []*unstructured.Unstructured{
		newObject("Namespace", "atmos-demo"),
		newObject("Service", "demo"),
	}

	err := deliverApply(&schema.AtmosConfiguration{}, info, flags, objects)
	require.NoError(t, err)
	require.NotNil(t, capture.last, "external target must receive a delivery")

	got := capture.last
	assert.Equal(t, "deployment-repo", got.TargetName)
	assert.Equal(t, target.ArtifactKindKubernetesManifests, got.Artifact.Kind)
	assert.Equal(t, target.FormatYAML, got.Artifact.Format)
	assert.Len(t, got.Artifact.Files, 2, "one file per object")
	assert.Equal(t, "argocd", got.Artifact.Metadata.Component)
	assert.Equal(t, "dev", got.Artifact.Metadata.Stack)
	assert.Equal(t, "deployment-repo", got.Artifact.Metadata.Target)
	// Each rendered file should contain the serialized object.
	var combined string
	for _, content := range got.Artifact.Files {
		combined += string(content)
	}
	assert.Contains(t, combined, "kind: Namespace")
	assert.Contains(t, combined, "kind: Service")
}

func TestDeliverApplyUnknownTargetErrors(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"provision": map[string]any{"targets": map[string]any{}},
		},
	}
	flags := map[string]any{"target": "does-not-exist"}

	err := deliverApply(&schema.AtmosConfiguration{}, info, flags, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNotFound)
}

func TestBuildKubernetesArtifactDeterministicFiles(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "demo", Stack: "dev"}
	objects := []*unstructured.Unstructured{
		newObject("Namespace", "ns"),
		newObject("ConfigMap", "cm"),
	}
	artifact, err := buildKubernetesArtifact(objects, info, "cluster")
	require.NoError(t, err)
	require.Len(t, artifact.Files, 2)
	// Filenames are sequence-prefixed so ordering is deterministic.
	assert.Contains(t, artifact.Files, "001_v1_Namespace_ns.yaml")
	assert.Contains(t, artifact.Files, "002_v1_ConfigMap_cm.yaml")
}
