package helm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	storeproviders "github.com/cloudposse/atmos/pkg/store/providers"
)

const (
	serviceAccountJSON = `{"type":"service_account","private_key":"-----BEGIN PRIVATE KEY-----\nAAAA\nBBBB\n-----END PRIVATE KEY-----\n","client_email":"service@example.iam.gserviceaccount.com"}`
	plainSecret        = "single-line-example-token"
	privateKey         = "-----BEGIN PRIVATE KEY-----\nAAAA\nBBBB\n-----END PRIVATE KEY-----\n"
)

// jsonDecodingStore reproduces the structured Get behavior of cloud secret stores while using
// the in-memory keychain backend. GetRaw preserves the opaque payload for !secret ... | raw.
type jsonDecodingStore struct {
	store.Store
}

func (s *jsonDecodingStore) Get(stack, component, key string) (any, error) {
	raw, err := s.GetRaw(stack, component, key)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return decoded, nil
	}
	return raw, nil
}

func (s *jsonDecodingStore) GetRaw(stack, component, key string) (string, error) {
	return s.Store.(store.RawStore).GetRaw(stack, component, key)
}

func TestNativeHelmSecretRawAndStructuredValuesMaskIndentedMultilineValues(t *testing.T) {
	fixture, atmosConfig, info := prepareHelmSecretValuesFixture(t)
	assertResolvedHelmSecretValues(t, info)
	rendered, renderedEnv := renderHelmSecretValuesFixture(t, fixture, atmosConfig, info)
	assert.Equal(t, serviceAccountJSON, envValue(t, renderedEnv, "SERVICE_ACCOUNT_JSON"), "Helm env.value must remain a scalar string")
	assertMaskedHelmSecretValues(t, rendered)
}

func prepareHelmSecretValuesFixture(t *testing.T) (string, *schema.AtmosConfiguration, schema.ConfigAndStacksInfo) {
	t.Helper()

	fixture, err := filepath.Abs(filepath.Join("..", "..", "..", "tests", "fixtures", "scenarios", "helm-secret-values"))
	require.NoError(t, err)
	t.Chdir(fixture)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", ".")

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "secret-values",
		ComponentType:    cfg.HelmComponentType,
		Stack:            "dev",
		SubCommand:       "template",
		SecretsMaskOnly:  true,
	}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	require.Equal(t, store.KindGCPSecret, atmosConfig.StoresConfig["gcp-secrets"].Kind)

	memoryStore, err := storeproviders.NewKeychainStore(&storeproviders.KeychainStoreOptions{Backend: keyring.TypeMemory})
	require.NoError(t, err)
	offlineStore := &jsonDecodingStore{Store: memoryStore}
	atmosConfig.Stores["gcp-secrets"] = offlineStore

	// Top-level declarations are stack scoped, so the component segment is intentionally empty.
	require.NoError(t, offlineStore.Set("dev", "", "service-account-json", serviceAccountJSON))
	require.NoError(t, offlineStore.Set("dev", "", "plain-token", plainSecret))
	require.NoError(t, offlineStore.Set("dev", "", "signing-key", privateKey))

	info.SecretsMaskOnly = false
	info, err = e.ProcessStacks(&atmosConfig, info, true, true, true, nil, auth.AuthManager(nil))
	require.NoError(t, err)
	return fixture, &atmosConfig, info
}

func assertResolvedHelmSecretValues(t *testing.T, info schema.ConfigAndStacksInfo) {
	t.Helper()

	values, ok := info.ComponentSection[cfg.ValuesSectionName].(map[string]any)
	require.True(t, ok)
	structured, ok := values["structured_service_account"].(map[string]any)
	require.True(t, ok, "bare !secret must preserve the existing structured store contract")
	assert.Equal(t, "service_account", structured["type"])
	env, ok := values["env"].([]any)
	require.True(t, ok)
	require.Len(t, env, 4)
	assert.Equal(t, serviceAccountJSON, envValue(t, env, "SERVICE_ACCOUNT_JSON"))
	assert.Equal(t, "service@example.iam.gserviceaccount.com", envValue(t, env, "CLIENT_EMAIL"))
	assert.Equal(t, plainSecret, envValue(t, env, "PLAIN_TOKEN"))
	assert.Equal(t, privateKey, envValue(t, env, "SIGNING_KEY"))
}

func renderHelmSecretValuesFixture(
	t *testing.T,
	fixture string,
	atmosConfig *schema.AtmosConfiguration,
	info schema.ConfigAndStacksInfo,
) (string, []any) {
	t.Helper()

	componentPath := filepath.Join(fixture, "components", "helm", "secret-values")
	spec, err := buildChartSpec(atmosConfig, &info, componentPath)
	require.NoError(t, err)
	rendered, err := renderManifest(context.Background(), spec)
	require.NoError(t, err)

	objects, err := manifest.DecodeObjects([]byte(rendered))
	require.NoError(t, err)
	require.Len(t, objects, 1)
	containers, found, err := unstructured.NestedSlice(objects[0].Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, containers)
	require.IsType(t, map[string]any{}, containers[0])
	container := containers[0].(map[string]any)
	require.IsType(t, []any{}, container["env"])
	return rendered, container["env"].([]any)
}

func assertMaskedHelmSecretValues(t *testing.T, rendered string) {
	t.Helper()

	masker := iolib.GetContext().Masker()
	t.Cleanup(func() {
		masker.Clear()
		masker.SetEnabled(true)
	})
	masked := masker.Mask(rendered)
	assert.NotContains(t, masked, "BEGIN PRIVATE KEY")
	assert.NotContains(t, masked, "AAAA")
	assert.NotContains(t, masked, "BBBB")
	assert.NotContains(t, masked, plainSecret)
	assert.Contains(t, masked, masker.Replacement())

	masker.SetEnabled(false)
	unmasked := masker.Mask(rendered)
	assert.Contains(t, unmasked, "BEGIN PRIVATE KEY")
	assert.Contains(t, unmasked, plainSecret)
}

func envValue(t *testing.T, env []any, name string) any {
	t.Helper()
	for _, item := range env {
		entry, ok := item.(map[string]any)
		if ok && entry["name"] == name {
			return entry["value"]
		}
	}
	t.Fatalf("environment entry %q not found", name)
	return nil
}
