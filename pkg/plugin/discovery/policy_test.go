package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

func discoveryPolicy(t *testing.T, predicate string) {
	t.Helper()
	t.Cleanup(ci.SwapRegistryForTest())
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: discovery}\nspec:\n  policies:\n    plugins:\n      on: plugin\n      action: deny\n      rule: |\n        " + predicate + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte(document), 0o600))
	restore, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(restore)
}

func TestCachedRepositoryRechecksCurrentSourcePolicy(t *testing.T) {
	f := newFixture(t)
	f.publish(t, "1.0.0", "Private plugin")
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "acme/fixture", results[0].Name)
	calls := f.requests.Load()
	f.fail.Store(true)
	discoveryPolicy(t, `input.repository == 'acme/fixture'`)
	for _, offline := range []bool{false, true} {
		f.service.Config.Plugins.Policy.Offline = offline
		results, err = f.service.Search(t.Context(), Request{})
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Empty(t, results, "a fresh discovery cache cannot override current admission")
	}
	require.Equal(t, calls, f.requests.Load(), "denied cached sources must not contact their registry")
}

func TestCachedCatalogLinksRecheckCurrentSourcePolicy(t *testing.T) {
	f := newFixture(t)
	linked := newFixture(t)
	release := f.publish(t, "1.0.0", "Linked plugin")
	reference := "oci://" + f.repository.RegistryStr() + "/product/catalog:stable"
	file := catalog.New("product")
	require.NoError(t, catalog.Add(file, "oci://"+linked.repository.Name(), release, f.service.Descriptors))
	require.NoError(t, catalog.Publish(t.Context(), f.service.Registry, reference, file, ""))
	f.service.Config.Plugins.Registries = map[string]schema.PluginRegistry{"product": {Catalog: reference}}
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "oci://"+linked.repository.Name(), results[0].Source)
	require.Zero(t, linked.requests.Load(), "catalog search must not contact linked repositories")
	calls := f.requests.Load()
	f.fail.Store(true)
	discoveryPolicy(t, `input.registry == '`+linked.repository.RegistryStr()+`'`)
	results, err = f.service.Search(t.Context(), Request{})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Empty(t, results, "an allowed catalog cannot authorize a denied linked repository")
	require.Equal(t, calls, f.requests.Load(), "a policy change applies without refreshing the catalog")
	require.Zero(t, linked.requests.Load())
}

func TestDiscoveryNetworkOriginAdmission(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "forced refresh"}[warm], func(t *testing.T) {
			f := newFixture(t)
			f.publish(t, "1.0.0", "Original")
			if warm {
				_, err := f.service.Search(t.Context(), Request{})
				require.NoError(t, err)
			}
			calls := f.requests.Load()
			discoveryPolicy(t, `input.origin == 'registry'`)
			results, err := f.service.Search(t.Context(), Request{Refresh: warm})
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Empty(t, results)
			require.Equal(t, calls, f.requests.Load(), "network access must use registry-origin admission")
			if warm {
				results, err = f.service.Search(t.Context(), Request{})
				require.NoError(t, err, "a policy permitting cache reuse still allows a fresh snapshot")
				require.Len(t, results, 1)
				require.Equal(t, "acme/fixture", results[0].Name)
				require.Equal(t, calls, f.requests.Load())
			}
		})
	}
}

func TestDiscoveryCacheDoesNotPersistAuthConfiguration(t *testing.T) {
	f := newFixture(t)
	const token = "fixture-only-discovery-secret"
	t.Setenv("ATMOS_DISCOVERY_TEST_TOKEN", token)
	f.service.Config.Auth.Identities = map[string]schema.Identity{
		"reader": {Kind: "oci/user", Spec: map[string]any{
			"registry": f.repository.RegistryStr(), "username": "private-reader", "password_env": "ATMOS_DISCOVERY_TEST_TOKEN",
		}},
	}
	f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	directory := filepath.Join(f.service.Config.Plugins.CacheDir, "discovery")
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	snapshots := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		snapshots++
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		require.NoError(t, err)
		for _, sensitive := range []string{token, "private-reader", "ATMOS_DISCOVERY_TEST_TOKEN", "password_env", "oci/user"} {
			require.NotContains(t, string(raw), sensitive)
			require.NotContains(t, entry.Name(), sensitive)
		}
	}
	require.Positive(t, snapshots, "verify actual persisted snapshots, not an empty directory")
}

func TestDiscoveryCanRequireFreshRegistryMetadata(t *testing.T) {
	f := newFixture(t)
	f.publish(t, "1.0.0", "Original")
	discoveryPolicy(t, `input.repository == 'acme/fixture' && input.origin == 'cache'`)
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err, "a cache restriction must not prohibit a fresh registry request")
	require.Len(t, results, 1)
	require.Equal(t, "acme/fixture", results[0].Name)
	calls := f.requests.Load()
	_, err = f.service.Search(t.Context(), Request{})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Equal(t, calls, f.requests.Load(), "a denied cache read does not refresh implicitly")
	results, err = f.service.Search(t.Context(), Request{Refresh: true})
	require.NoError(t, err, "explicit refresh uses registry-origin admission")
	require.Len(t, results, 1)
	require.Equal(t, "acme/fixture", results[0].Name)
	require.Greater(t, f.requests.Load(), calls)
}
