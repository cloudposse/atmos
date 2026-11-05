package configschema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	stjsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginSignaturePolicyValidatesAgainstGeneratedAndCommittedSchemas(t *testing.T) {
	repoRoot, err := RepoRoot()
	require.NoError(t, err)
	committed, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(EmbeddedPath)))
	require.NoError(t, err)
	for name, raw := range map[string][]byte{"generated": generatedSchema(t), "committed": committed} {
		t.Run(name, func(t *testing.T) {
			compiler := stjsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource("atmos-config.json", bytes.NewReader(raw)))
			compiled, err := compiler.Compile("atmos-config.json")
			require.NoError(t, err)
			for _, tc := range pluginSignatureSchemaCases() {
				t.Run(tc.name, func(t *testing.T) {
					var document map[string]any
					require.NoError(t, yaml.Unmarshal([]byte(tc.config), &document))
					err := compiled.Validate(document)
					if tc.valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
			assertPublisherConstraintsExcluded(t, raw)
		})
	}
}

type pluginSignatureSchemaCase struct {
	name, config string
	valid        bool
}

func pluginSignatureSchemaCases() []pluginSignatureSchemaCase {
	return []pluginSignatureSchemaCase{
		{"signed project", signedPluginConfiguration, true},
		{"partial policy", `plugins: {policy: {require_signature: true}}`, true},
		{"deferred publisher list", `plugins: {policy: {trusted_publishers: "!include publishers.yaml"}}`, true},
		{"deferred signature flag", `plugins: {policy: {require_signature: "!env REQUIRE_SIGNATURE"}}`, true},
		{"signature flag string", `plugins: {policy: {require_signature: "yes"}}`, false},
		{"offline object", `plugins: {policy: {offline: {enabled: true}}}`, false},
		{"package path number", `plugins: {packages: {corporate: {path: 42}}}`, false},
		{"package path with release", `plugins: {packages: {corporate: {source: "oci://registry.example.com/plugins/oidc", version: "^0.1.0", path: build/corporate-oidc}}}`, true},
		{"publisher list object", `plugins: {policy: {trusted_publishers: {publisher: acme}}}`, false},
		{"publisher entry scalar", `plugins: {policy: {trusted_publishers: [acme]}}`, false},
		{"publisher namespace number", `plugins: {policy: {trusted_publishers: [{publisher: 42}]}}`, false},
		{"identity array", `plugins: {policy: {trusted_publishers: [{identity: [builder@example.com]}]}}`, false},
		{"issuer boolean", `plugins: {policy: {trusted_publishers: [{issuer: true}]}}`, false},
		{"numeric package version", `plugins: {packages: {corporate: {version: 42}}}`, false},
		{"registry identity", `plugins: {registries: {corporate: {catalog: "oci://registry.example.com/catalog:stable", identity: registry-reader}}}`, true},
		{"registry host only", `plugins: {registries: {private: {host: ghcr.io, identity: reader}}}`, true},
		{"indexed repositories", `plugins: {registries: {private: {repositories: ["oci://ghcr.io/acme/one", "oci://ghcr.io/acme/two"]}}, discovery: {cache_ttl: 15m}}`, true},
		{"repository scalar", `plugins: {registries: {private: {repositories: 42}}}`, false},
		{"repository number", `plugins: {registries: {private: {repositories: [42]}}}`, false},
		{"discovery ttl number", `plugins: {discovery: {cache_ttl: 42}}`, false},
		{"registry identity number", `plugins: {registries: {corporate: {identity: 42}}}`, false},
		{"concurrency object", `plugins: {max_concurrency: {count: 4}}`, false},
	}
}

func assertPublisherConstraintsExcluded(t *testing.T, raw []byte) {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal(raw, &document))
	definitions, ok := document["$defs"].(map[string]any)
	require.True(t, ok)
	policy, ok := definitions["PluginPolicy"].(map[string]any)
	require.True(t, ok)
	properties, ok := policy["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, properties, "trusted_publishers")
	require.Contains(t, properties, "require_signature")
	for _, field := range []string{"publisher_constraints", "PublisherConstraints"} {
		require.NotContains(t, properties, field, "inherited policy state must not become an authored configuration field")
	}
}

func TestRuntimePublisherConstraintsAreNeverSerializedAsAuthoredPolicy(t *testing.T) {
	publisher := schema.PluginPublisher{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}
	policy := schema.PluginPolicy{RequireSignature: true, TrustedPublishers: []schema.PluginPublisher{publisher}, PublisherConstraints: [][]schema.PluginPublisher{{publisher}}}
	encodedYAML, err := yaml.Marshal(policy)
	require.NoError(t, err)
	encodedJSON, err := json.Marshal(policy)
	require.NoError(t, err)
	for _, raw := range [][]byte{encodedYAML, encodedJSON} {
		var document map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &document))
		require.True(t, document["require_signature"].(bool))
		require.Contains(t, document, "trusted_publishers")
		require.NotContains(t, document, "publisher_constraints")
		require.NotContains(t, document, "PublisherConstraints")
	}
}

const signedPluginConfiguration = `
plugins:
  max_concurrency: 4
  cache_dir: .cache/plugins
  packages:
    corporate:
      source: oci://ghcr.io/acme/oidc
      version: "^1.0.0"
  policy:
    require_signature: true
    offline: false
    trusted_publishers:
      - publisher: acme
        identity: https://github.com/acme/oidc/.github/workflows/release.yaml@refs/tags/*
        issuer: https://token.actions.githubusercontent.com
`
