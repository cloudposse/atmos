package sops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/secrets/providers"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// newAgeProvider creates a SOPS provider backed by a freshly generated age key written to
// SOPS_AGE_KEY_FILE, with the recipient passed explicitly via spec.age_recipients. This exercises
// the full in-process encrypt/decrypt path with NO `sops` binary and no committed fixtures.
func newAgeProvider(t *testing.T) (providers.Provider, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(keyFile, []byte(identity.String()+"\n"), 0o600))
	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)

	file := filepath.Join(dir, "dev.enc.yaml")
	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{
				"file":           file,
				"age_recipients": identity.Recipient().String(),
			},
		},
	}
	p, err := New(&schema.AtmosConfiguration{}, "dev-sops", section)
	require.NoError(t, err)
	return p, file
}

// newAgeProviderWithFile is like newAgeProvider but lets the caller control the spec.file
// template (joined under a temp dir), so tests can exercise stack-scoped (no component in the
// path) vs component-scoped (component in the path) storage layouts.
func newAgeProviderWithFile(t *testing.T, nameTemplate string) providers.Provider {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(keyFile, []byte(identity.String()+"\n"), 0o600))
	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)

	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{
				"file":           filepath.Join(dir, nameTemplate),
				"age_recipients": identity.Recipient().String(),
			},
		},
	}
	p, err := New(&schema.AtmosConfiguration{}, "dev-sops", section)
	require.NoError(t, err)
	return p
}

func TestSopsProvider_RoundTrip(t *testing.T) {
	p, file := newAgeProvider(t)
	datadog := providers.Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}
	redis := providers.Coordinate{Stack: "dev", Component: "api", Key: "REDIS_URL"}

	// Not initialized before the file exists.
	ok, err := p.Status(datadog)
	require.NoError(t, err)
	assert.False(t, ok)

	// Set creates the encrypted file in-process.
	require.NoError(t, p.Set(datadog, "dd-abc123secret"))
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "ENC[", "value must be encrypted at rest")
	assert.NotContains(t, string(raw), "dd-abc123secret", "plaintext must not leak to disk")

	// Get round-trips the value.
	got, err := p.Get(datadog)
	require.NoError(t, err)
	assert.Equal(t, "dd-abc123secret", got)

	ok, err = p.Status(datadog)
	require.NoError(t, err)
	assert.True(t, ok)

	// A second key coexists; setting it does not disturb the first.
	require.NoError(t, p.Set(redis, "redis://prod:6379"))
	gotRedis, err := p.Get(redis)
	require.NoError(t, err)
	assert.Equal(t, "redis://prod:6379", gotRedis)
	gotDatadog, err := p.Get(datadog)
	require.NoError(t, err)
	assert.Equal(t, "dd-abc123secret", gotDatadog)

	// Delete removes only the targeted key.
	require.NoError(t, p.Delete(datadog))
	_, err = p.Get(datadog)
	require.ErrorIs(t, err, ErrSecretNotInitialized)
	gotRedis, err = p.Get(redis)
	require.NoError(t, err)
	assert.Equal(t, "redis://prod:6379", gotRedis)

	// Reset wipes the whole file back to a clean, empty document.
	resettable, ok := p.(providers.FileResettable)
	require.True(t, ok, "sops provider must implement providers.FileResettable")
	require.NoError(t, resettable.Reset(redis))
	_, err = p.Get(redis)
	require.ErrorIs(t, err, ErrSecretNotInitialized)
}

func TestSopsProvider_GetWithoutKeyFails(t *testing.T) {
	p, _ := newAgeProvider(t)
	coord := providers.Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}
	require.NoError(t, p.Set(coord, "dd-abc123secret"))

	// Without the age identity, decryption genuinely fails (proves retrieval decrypts).
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent.txt"))
	_, err := p.Get(coord)
	require.ErrorIs(t, err, ErrSopsDecrypt)

	// Status swallows the decrypt failure as "not initialized".
	ok, statusErr := p.Status(coord)
	require.NoError(t, statusErr)
	assert.False(t, ok)
}

func TestSopsProvider_DeleteMissingFileIsNoOp(t *testing.T) {
	p, _ := newAgeProvider(t)
	// No file created yet; deleting is idempotent.
	require.NoError(t, p.Delete(providers.Coordinate{Stack: "dev", Component: "api", Key: "NOPE"}))
}

func TestSopsProvider_FilePathTemplate(t *testing.T) {
	p := &sopsProvider{file: "secrets/{{ .atmos_stack }}.{{ .atmos_component }}.enc.yaml"}
	got, err := p.resolveFile(providers.Coordinate{Stack: "dev", Component: "api"})
	require.NoError(t, err)
	assert.Equal(t, filepath.FromSlash("secrets/dev.api.enc.yaml"), filepath.FromSlash(got))

	// An unknown template variable is a hard error (missingkey=error).
	bad := &sopsProvider{file: "secrets/{{ .not_a_var }}.enc.yaml"}
	_, err = bad.resolveFile(providers.Coordinate{Stack: "dev", Component: "api"})
	require.ErrorIs(t, err, ErrSopsFilePathTemplate)
}

// TestSopsProvider_StackScopedSharing proves that when the file template omits the component,
// all components in a stack share one encrypted file: a second component writing the same key
// overwrites the first (stack-scoped storage).
func TestSopsProvider_StackScopedSharing(t *testing.T) {
	p := newAgeProviderWithFile(t, "{{ .atmos_stack }}.enc.yaml")
	sp := p.(*sopsProvider)

	api := providers.Coordinate{Stack: "dev", Component: "api", Key: "SHARED_TOKEN"}
	web := providers.Coordinate{Stack: "dev", Component: "web", Key: "SHARED_TOKEN"}

	apiFile, err := sp.resolveFile(api)
	require.NoError(t, err)
	webFile, err := sp.resolveFile(web)
	require.NoError(t, err)
	assert.Equal(t, apiFile, webFile, "no component in template => one shared file per stack")

	require.NoError(t, p.Set(api, "from-api"))
	require.NoError(t, p.Set(web, "from-web"))

	// Same key in a shared file => the second write overwrote the first.
	got, err := p.Get(api)
	require.NoError(t, err)
	assert.Equal(t, "from-web", got, "shared file: web's write overwrites api's value")
}

// TestSopsProvider_ComponentScopedIsolation proves that when the file template includes the
// component, each component gets its own encrypted file with independent values
// (component-scoped storage) and cannot read a sibling component's secret.
func TestSopsProvider_ComponentScopedIsolation(t *testing.T) {
	p := newAgeProviderWithFile(t, "{{ .atmos_stack }}.{{ .atmos_component }}.enc.yaml")
	sp := p.(*sopsProvider)

	api := providers.Coordinate{Stack: "dev", Component: "api", Key: "API_KEY"}
	web := providers.Coordinate{Stack: "dev", Component: "web", Key: "WEB_KEY"}

	apiFile, err := sp.resolveFile(api)
	require.NoError(t, err)
	webFile, err := sp.resolveFile(web)
	require.NoError(t, err)
	assert.NotEqual(t, apiFile, webFile, "component in template => distinct files per component")

	require.NoError(t, p.Set(api, "api-secret"))
	require.NoError(t, p.Set(web, "web-secret"))

	gotAPI, err := p.Get(api)
	require.NoError(t, err)
	assert.Equal(t, "api-secret", gotAPI)
	gotWeb, err := p.Get(web)
	require.NoError(t, err)
	assert.Equal(t, "web-secret", gotWeb)

	// Component isolation: web's file exists but does not contain api's key.
	_, err = p.Get(providers.Coordinate{Stack: "dev", Component: "web", Key: "API_KEY"})
	require.ErrorIs(t, err, ErrSecretNotInitialized)
}
