package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginPolicyCannotBeWeakenedByLaterConfiguration(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	resetMergedConfigFiles(v)
	t.Cleanup(func() { mergedFilesReg.finish(v) })
	directory := t.TempDir()
	files := []string{
		`Plugins:
  Policy:
    require_signature: true
    offline: true
    trusted_publishers:
      - {publisher: acme, identity: "https://github.com/acme/*", issuer: "https://token.actions.githubusercontent.com"}
`,
		`plugins:
  policy:
    require_signature: false
    offline: false
    trusted_publishers:
      - {publisher: acme, identity: "https://github.com/acme/oidc/release", issuer: "https://token.actions.githubusercontent.com"}
`,
	}
	for index, body := range files {
		path := filepath.Join(directory, []string{"machine.yaml", "project.yaml"}[index])
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		require.NoError(t, mergeConfigFile(path, v))
	}
	var config schema.AtmosConfiguration
	require.NoError(t, v.Unmarshal(&config, atmosDecodeHook()))
	require.NoError(t, applyPluginPolicy(v, &config))
	policy := config.Plugins.Policy
	require.True(t, policy.RequireSignature)
	require.True(t, policy.Offline)
	require.Len(t, policy.PublisherConstraints, 2)
	require.Equal(t, "https://github.com/acme/*", policy.PublisherConstraints[0][0].Identity)
	require.Equal(t, "https://github.com/acme/oidc/release", policy.PublisherConstraints[1][0].Identity)
}

func TestPluginPolicyRejectsMalformedEarlierLayer(t *testing.T) {
	v := viper.New()
	resetMergedConfigFiles(v)
	t.Cleanup(func() { mergedFilesReg.finish(v) })
	mergedFilesReg.track(v, "machine.yaml", []byte("plugins:\n  policy:\n    require_signature: not-a-bool\n"))
	var config schema.AtmosConfiguration
	require.Error(t, applyPluginPolicy(v, &config))
}

func TestPluginPolicyResolvesIncludedPolicyAndEnvironment(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PLUGIN_TEST_REQUIRE_SIGNATURE", "true")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "publishers.yaml"), []byte("- {publisher: acme, identity: builder@example.com, issuer: 'https://issuer.example.com'}\n"), 0o600))
	policy := "require_signature: !env PLUGIN_TEST_REQUIRE_SIGNATURE\ntrusted_publishers: !include publishers.yaml\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(policy), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte("policy: !include policy.yaml | eval\n"), 0o600))
	for _, content := range []string{"plugins:\n  policy: !include policy.yaml | eval\n", "plugins: !include plugins.yaml | eval\n"} {
		layer, err := pluginPolicyFromSource(mergedConfigSource{path: filepath.Join(directory, "atmos.yaml"), content: content})
		require.NoError(t, err)
		require.True(t, layer.RequireSignature)
		require.Equal(t, []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}, layer.TrustedPublishers)
	}
	_, err := pluginPolicyFromSource(mergedConfigSource{path: filepath.Join(directory, "atmos.yaml"), content: "plugins:\n  policy: !include missing.yaml\n"})
	require.Error(t, err, "unavailable policy must never silently remove trust restrictions")
}
