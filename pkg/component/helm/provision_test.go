package helm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeTarget is a registrable provision target that records what it received and
// can be told to fail. It implements both Provisioner and Fetcher.
type fakeTarget struct {
	delivered     *target.DeliverInput
	fetched       *target.FetchInput
	deliverErr    error
	fetchArtifact target.ProvisionArtifact
	fetchErr      error
}

func (f *fakeTarget) Deliver(_ context.Context, in *target.DeliverInput) error {
	f.delivered = in
	return f.deliverErr
}

func (f *fakeTarget) Fetch(_ context.Context, in *target.FetchInput) (target.ProvisionArtifact, error) {
	f.fetched = in
	return f.fetchArtifact, f.fetchErr
}

// registerFakeTarget registers ft under kind and returns the kind name.
func registerFakeTarget(t *testing.T, kind string, ft *fakeTarget) {
	t.Helper()
	target.Register(kind, ft)
}

func stubRenderChartManifest(t *testing.T, manifest string, err error) {
	t.Helper()
	original := renderChartManifest
	t.Cleanup(func() { renderChartManifest = original })
	renderChartManifest = func(context.Context, *chartSpec) (string, error) {
		return manifest, err
	}
}

func TestDeliverToExternalTarget_DeliversRenderedManifests(t *testing.T) {
	ft := &fakeTarget{}
	registerFakeTarget(t, "helm-external-test", ft)
	stubRenderChartManifest(t, helmExecutorManifest, nil)

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "apps/app", Stack: "dev"}
	selected := &target.SelectedTarget{Name: "deploy-repo", Kind: "helm-external-test", Config: map[string]any{}}

	summary, err := deliverToExternalTarget(&schema.AtmosConfiguration{}, info, selected, &chartSpec{Chart: "demo"}, map[string]any{})
	require.NoError(t, err)

	require.NotNil(t, ft.delivered)
	assert.Equal(t, target.ArtifactKindKubernetesManifests, ft.delivered.Artifact.Kind)
	assert.Equal(t, target.FormatYAML, ft.delivered.Artifact.Format)
	assert.Equal(t, "apps/app", ft.delivered.Artifact.Metadata.Component)
	assert.Equal(t, "dev", ft.delivered.Artifact.Metadata.Stack)
	assert.Equal(t, "deploy-repo", ft.delivered.Artifact.Metadata.Target)
	require.NotEmpty(t, ft.delivered.Artifact.Files)

	assert.Equal(t, 1, summary["object_count"])
	assert.Equal(t, []string{"ConfigMap"}, summary["object_kinds"])
	assert.Equal(t, len(helmExecutorManifest), summary["manifest_bytes"])
}

func TestDeliverToExternalTarget_RenderError(t *testing.T) {
	stubRenderChartManifest(t, "", errors.New("render boom"))

	_, err := deliverToExternalTarget(
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{},
		&target.SelectedTarget{Name: "deploy-repo", Kind: "helm-external-test"},
		&chartSpec{Chart: "demo"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render boom")
}

func TestDeliverToExternalTarget_DeliverError(t *testing.T) {
	ft := &fakeTarget{deliverErr: errors.New("push rejected")}
	registerFakeTarget(t, "helm-external-err", ft)
	stubRenderChartManifest(t, helmExecutorManifest, nil)

	_, err := deliverToExternalTarget(
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{ComponentFromArg: "apps/app"},
		&target.SelectedTarget{Name: "deploy-repo", Kind: "helm-external-err"},
		&chartSpec{Chart: "demo"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "push rejected")
}

// TestDeliverApply_RoutesToExternalTarget exercises the non-cluster apply path:
// SelectTarget resolves the configured default to a non-Kubernetes kind, which
// is delivered via deliverToExternalTarget.
func TestDeliverApply_RoutesToExternalTarget(t *testing.T) {
	ft := &fakeTarget{}
	registerFakeTarget(t, "helm-apply-external", ft)
	stubRenderChartManifest(t, helmExecutorManifest, nil)

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "apps/app",
		Stack:            "dev",
		ComponentSection: map[string]any{
			"provision": map[string]any{
				"default": "deploy-repo",
				"targets": map[string]any{
					"deploy-repo": map[string]any{"kind": "helm-apply-external"},
				},
			},
		},
	}

	summary, err := deliverApply(&schema.AtmosConfiguration{}, info, map[string]any{}, &chartSpec{Chart: "demo"})
	require.NoError(t, err)
	assert.Equal(t, "deploy-repo", summary[targetKey])
	require.NotNil(t, ft.delivered)
	assert.Equal(t, "deploy-repo", ft.delivered.TargetName)
}

func TestDeliverApply_SelectTargetError(t *testing.T) {
	// An explicitly requested target that is not configured fails to resolve.
	_, err := deliverApply(
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
		map[string]any{"target": "does-not-exist"},
		&chartSpec{Chart: "demo"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNotFound)
}

func TestTotalManifestBytes(t *testing.T) {
	assert.Zero(t, totalManifestBytes(nil))
	files := map[string][]byte{
		"a.yaml": []byte("hello"),
		"b.yaml": []byte("world!"),
	}
	assert.Equal(t, len("hello")+len("world!"), totalManifestBytes(files))
}

func TestAuthManagerFor_NilAndWrongType(t *testing.T) {
	assert.Nil(t, authManagerFor(&schema.ConfigAndStacksInfo{}))
	assert.Nil(t, authManagerFor(&schema.ConfigAndStacksInfo{AuthManager: "not-a-manager"}))
}
