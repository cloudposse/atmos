//go:build envtest

// Package kubernetes end-to-end tests run against a real kube-apiserver+etcd
// control plane started by controller-runtime's envtest. They are gated behind
// the `envtest` build tag so the default `go test` run (and Windows CI, which
// has no kube-apiserver build) never compiles or runs them:
//
//	go test -tags envtest ./pkg/component/kubernetes/...
//
// The control-plane binaries are provisioned on demand through the Atmos
// toolchain via an inline registry defined in this file — no Makefile target,
// no setup-envtest, no atmos.yaml. They are cached under the user cache dir so
// repeat runs are offline-fast.
//
// This tier exists to cover the parts of client.go that the in-memory fake
// dynamic client cannot: server-side-apply field ownership, dry-run diff
// semantics, real discovery/RESTMapper resolution, and CRD round-trips.
package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

const (
	// These identify the controller-tools GitHub repository whose releases
	// publish the envtest control-plane tarball (kube-apiserver+etcd+kubectl)
	// indexed by envtest-releases.yaml.
	envtestOwner = "kubernetes-sigs"
	envtestRepo  = "controller-tools"
	// Download the control plane at this Kubernetes version. The release tag is
	// `envtest-v<version>` and assets are `envtest-v<version>-<os>-<arch>.tar.gz`.
	envtestVersion = "1.31.0"

	// Set to "true" to turn a provisioning/start failure into a hard test
	// failure instead of a skip. CI sets this so a broken control plane fails
	// loudly rather than silently skipping all coverage.
	envtestRequiredEnv = "ATMOS_ENVTEST_REQUIRED"
)

// Control-plane state shared across the package's envtest tests, started once in
// envtestSetup (called from TestMain) and torn down after the run.
var (
	envtestCfg      *rest.Config
	envtestStartErr error
)

// envtestRegistry is an inline registry.ToolRegistry that describes the
// controller-tools envtest tarball. It lets the Atmos toolchain download and
// extract the three control-plane binaries without consulting the aqua registry
// or atmos.yaml.
type envtestRegistry struct{}

func (envtestRegistry) GetTool(_, _ string) (*registry.Tool, error) {
	return envtestTool(), nil
}

func (envtestRegistry) GetToolWithVersion(_, _, _ string) (*registry.Tool, error) {
	return envtestTool(), nil
}

func (envtestRegistry) GetLatestVersion(_, _ string) (string, error) {
	return envtestVersion, nil
}

func (envtestRegistry) LoadLocalConfig(_ string) error { return nil }

func (envtestRegistry) Search(_ context.Context, _ string, _ ...registry.SearchOption) ([]*registry.Tool, error) {
	return nil, nil
}

func (envtestRegistry) ListAll(_ context.Context, _ ...registry.ListOption) ([]*registry.Tool, error) {
	return nil, nil
}

func (envtestRegistry) GetMetadata(_ context.Context) (*registry.RegistryMetadata, error) {
	return &registry.RegistryMetadata{Name: "envtest-inline", Type: "atmos"}, nil
}

// envtestTool returns the package descriptor for the envtest tarball. With
// VersionPrefix "envtest-v" and version "1.31.0" the toolchain builds the tag
// `envtest-v1.31.0` and the asset `envtest-v1.31.0-<os>-<arch>.tar.gz`, then
// extracts the three binaries (the first is primary; the rest land beside it).
func envtestTool() *registry.Tool {
	return &registry.Tool{
		Type:          "github_release",
		RepoOwner:     envtestOwner,
		RepoName:      envtestRepo,
		VersionPrefix: "envtest-v",
		Asset:         "{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz",
		Format:        "tar.gz",
		BinaryName:    "kube-apiserver",
		Files: []registry.File{
			{Name: "kube-apiserver", Src: "controller-tools/envtest/kube-apiserver"},
			{Name: "etcd", Src: "controller-tools/envtest/etcd"},
			{Name: "kubectl", Src: "controller-tools/envtest/kubectl"},
		},
		// No windows: there is no official kube-apiserver build for it.
		SupportedEnvs: []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"},
	}
}

// provisionEnvtestBinaries installs the control-plane binaries via the toolchain
// and returns the directory that holds them (suitable for KUBEBUILDER_ASSETS).
func provisionEnvtestBinaries() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	// Share the CLI suite's toolchain cache location so downloads are reused.
	base := filepath.Join(cacheDir, "atmos", "test-toolchain")

	inst := toolchain.NewInstaller(
		toolchain.WithConfiguredRegistry(envtestRegistry{}),
		toolchain.WithBinDir(filepath.Join(base, "bin")),
		toolchain.WithCacheDir(filepath.Join(base, "cache")),
	)

	binaryPath, err := inst.Install(envtestOwner, envtestRepo, envtestVersion)
	if err != nil {
		return "", err
	}
	// All three binaries are extracted into the same version directory.
	return filepath.Dir(binaryPath), nil
}

// envtestSetup provisions binaries, starts the control plane, and writes a
// kubeconfig the component's client loader (clientcmd, via KUBECONFIG) can use.
// On failure it records envtestStartErr; requireEnvtest then skips (or fails
// hard when ATMOS_ENVTEST_REQUIRED=true). The returned func tears everything
// down.
func envtestSetup() func() {
	assetsDir, err := provisionEnvtestBinaries()
	if err != nil {
		envtestStartErr = err
		return func() {}
	}
	// envtest locates kube-apiserver+etcd via this env var. TestMain has no
	// *testing.T, so t.Setenv is unavailable; the value is process-global for the
	// control-plane lifetime.
	//nolint:lintroller // Set in TestMain (no *testing.T); process-global for the control-plane lifetime.
	os.Setenv("KUBEBUILDER_ASSETS", assetsDir)

	env := &envtest.Environment{BinaryAssetsDirectory: assetsDir}
	cfg, err := env.Start()
	if err != nil {
		envtestStartErr = err
		return func() {}
	}
	envtestCfg = cfg

	kubeconfigPath := filepath.Join(os.TempDir(), "atmos-envtest-kubeconfig")
	if err := writeKubeconfig(cfg, kubeconfigPath); err != nil {
		envtestStartErr = err
		_ = env.Stop()
		return func() {}
	}
	// newSDKClient() loads config from KUBECONFIG via clientcmd default rules.
	//nolint:lintroller // Set in TestMain (no *testing.T); must persist for all tests, cleared at teardown.
	os.Setenv("KUBECONFIG", kubeconfigPath)

	return func() {
		_ = env.Stop()
		_ = os.Remove(kubeconfigPath)
	}
}

// writeKubeconfig serializes a rest.Config (envtest uses client-cert auth) into
// a kubeconfig file on disk.
func writeKubeconfig(cfg *rest.Config, path string) error {
	const name = "envtest"
	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Clusters[name] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: cfg.CAData,
	}
	kubeconfig.AuthInfos[name] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
	}
	kubeconfig.Contexts[name] = &clientcmdapi.Context{
		Cluster:  name,
		AuthInfo: name,
	}
	kubeconfig.CurrentContext = name
	return clientcmd.WriteToFile(*kubeconfig, path)
}

// requireEnvtest ensures the control plane is available, skipping (or failing in
// CI) otherwise. It returns a raw dynamic client for direct assertions that
// bypass the component under test.
func requireEnvtest(t *testing.T) dynamic.Interface {
	t.Helper()
	if envtestStartErr != nil {
		if os.Getenv(envtestRequiredEnv) == "true" {
			t.Fatalf("envtest control plane unavailable (and %s=true): %v", envtestRequiredEnv, envtestStartErr)
		}
		t.Skipf("envtest control plane unavailable: %v", envtestStartErr)
	}
	require.NotNil(t, envtestCfg, "envtest config must be set when start succeeded")
	dyn, err := dynamic.NewForConfig(envtestCfg)
	require.NoError(t, err)
	return dyn
}

// newClient builds a component sdkClient from the envtest kubeconfig. Each call
// constructs a fresh RESTMapper, which the CRD round-trip relies on to pick up
// newly registered resources.
func newClient(t *testing.T) *sdkClient {
	t.Helper()
	client, err := newSDKClient()
	require.NoError(t, err)
	return client
}

// applyObjects applies objects through the component and asserts success.
func applyObjects(t *testing.T, client *sdkClient, objs ...*unstructured.Unstructured) []objectResult {
	t.Helper()
	results, err := client.Apply(context.Background(), objs)
	require.NoError(t, err)
	return results
}

var (
	configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	crdGVR       = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
)

func configMapObject(name string, data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "default",
		},
		"data": data,
	}}
}

func TestEnvtestApplyRoundTripAndFieldOwnership(t *testing.T) {
	dyn := requireEnvtest(t)
	ctx := context.Background()

	client := newClient(t)
	cm := configMapObject("atmos-e2e-apply", map[string]any{"key": "value"})
	results := applyObjects(t, client, cm)
	require.Len(t, results, 1)
	assert.Equal(t, "applied", results[0].Action)
	assert.Equal(t, "atmos-e2e-apply", results[0].Name)

	// Read the live object directly and verify both the data and that the real
	// API server recorded the "atmos" field manager — the server-side-apply
	// ownership the fake client does not implement.
	got, err := dyn.Resource(configMapGVR).Namespace("default").Get(ctx, "atmos-e2e-apply", metav1.GetOptions{})
	require.NoError(t, err)

	gotData, _, err := unstructured.NestedStringMap(got.Object, "data")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"key": "value"}, gotData)

	assert.Contains(t, managedFieldManagers(got), fieldManager, "server-side apply must record the atmos field manager")
}

func TestEnvtestApplyIdempotentDiffNoChange(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	client := newClient(t)
	cm := configMapObject("atmos-e2e-idempotent", map[string]any{"a": "1"})
	applyObjects(t, client, cm)

	// Re-applying the identical object is a no-op against a real server.
	results := applyObjects(t, client, cm)
	require.Len(t, results, 1)
	assert.Equal(t, "applied", results[0].Action)

	diff, err := client.Diff(ctx, []*unstructured.Unstructured{cm})
	require.NoError(t, err)
	require.Len(t, diff, 1)
	assert.Equal(t, "no-change", diff[0].Action)
}

func TestEnvtestDiffReportsCreateAndChanged(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	client := newClient(t)

	// A never-applied object diffs as a create.
	fresh := configMapObject("atmos-e2e-diff", map[string]any{"v": "first"})
	diff, err := client.Diff(ctx, []*unstructured.Unstructured{fresh})
	require.NoError(t, err)
	require.Len(t, diff, 1)
	assert.Equal(t, "create", diff[0].Action)

	// Apply it, then a differing version diffs as changed.
	applyObjects(t, client, fresh)

	changed := configMapObject("atmos-e2e-diff", map[string]any{"v": "second"})
	diff, err = client.Diff(ctx, []*unstructured.Unstructured{changed})
	require.NoError(t, err)
	require.Len(t, diff, 1)
	assert.Equal(t, "changed", diff[0].Action)
}

func TestEnvtestDeleteExistingAndMissing(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	client := newClient(t)
	cm := configMapObject("atmos-e2e-delete", map[string]any{"x": "y"})
	applyObjects(t, client, cm)

	results, err := client.Delete(ctx, []*unstructured.Unstructured{cm})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "deleted", results[0].Action)

	// Deleting the now-absent object is tolerated and reported as not-found.
	results, err = client.Delete(ctx, []*unstructured.Unstructured{cm})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "not-found", results[0].Action)
}

func TestEnvtestValidateAcceptsValidRejectsInvalid(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	client := newClient(t)

	valid := configMapObject("atmos-e2e-valid", map[string]any{"ok": "yes"})
	results, err := client.Validate(ctx, []*unstructured.Unstructured{valid})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "valid", results[0].Action)

	// A Pod with no containers is schema-invalid; the server rejects it on
	// dry-run apply, which Validate aggregates into a returned error.
	invalidPod := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "atmos-e2e-invalid",
			"namespace": "default",
		},
		"spec": map[string]any{
			"containers": []any{}, // required to be non-empty
		},
	}}
	_, err = client.Validate(ctx, []*unstructured.Unstructured{invalidPod})
	require.Error(t, err)
}

func TestEnvtestCRDRoundTrip(t *testing.T) {
	dyn := requireEnvtest(t)
	ctx := context.Background()

	const (
		group   = "e2e.atmos.tools"
		plural  = "widgets"
		crdName = plural + "." + group
	)

	crd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": crdName},
		"spec": map[string]any{
			"group": group,
			"names": map[string]any{
				"plural":   plural,
				"singular": "widget",
				"kind":     "Widget",
				"listKind": "WidgetList",
			},
			"scope": "Namespaced",
			"versions": []any{
				map[string]any{
					"name":    "v1",
					"served":  true,
					"storage": true,
					"schema": map[string]any{
						"openAPIV3Schema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"spec": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"size": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
				},
			},
		},
	}}

	applyObjects(t, newClient(t), crd)

	// Wait until the API server reports the CRD as Established before resolving
	// the custom resource through discovery.
	require.Eventually(t, func() bool {
		got, getErr := dyn.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
		if getErr != nil {
			return false
		}
		conditions, _, _ := unstructured.NestedSlice(got.Object, "status", "conditions")
		for _, c := range conditions {
			cond, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if cond["type"] == "Established" && cond["status"] == "True" {
				return true
			}
		}
		return false
	}, 30*time.Second, 250*time.Millisecond, "CRD never became Established")

	// A fresh client has a fresh RESTMapper that must discover the new CRD.
	widget := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": group + "/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name":      "atmos-e2e-widget",
			"namespace": "default",
		},
		"spec": map[string]any{"size": "large"},
	}}
	results := applyObjects(t, newClient(t), widget)
	require.Len(t, results, 1)
	assert.Equal(t, "applied", results[0].Action)

	widgetGVR := schema.GroupVersionResource{Group: group, Version: "v1", Resource: plural}
	got, err := dyn.Resource(widgetGVR).Namespace("default").Get(ctx, "atmos-e2e-widget", metav1.GetOptions{})
	require.NoError(t, err)
	size, _, _ := unstructured.NestedString(got.Object, "spec", "size")
	assert.Equal(t, "large", size)
}

// managedFieldManagers returns the distinct field-manager names recorded on an
// object's metadata.managedFields.
func managedFieldManagers(obj *unstructured.Unstructured) []string {
	var managers []string
	for _, mf := range obj.GetManagedFields() {
		managers = append(managers, mf.Manager)
	}
	return managers
}
