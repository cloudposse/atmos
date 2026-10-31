package config

import (
	"encoding/json"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prdDocument is every YAML example from the artifacts PRD that sits under `artifacts:`, merged
// into one document: the public-configuration repositories plus the catalog, verification, and
// locking examples.
const prdDocument = `
verification:
  require_signature: true
  trusted_identities:
    - identity: https://github.com/acme/images/.github/workflows/release.yml@refs/heads/main
      issuer: https://token.actions.githubusercontent.com
  trusted_keys: []
repositories:
  shared:
    kind: oci/registry
    identity: registry-publisher
    options:
      registry: ghcr.io
      prefix: acme/atmos
    defaults:
      metadata:
        description: Shared platform artifacts
        labels:
          team: platform
    catalog: oci://ghcr.io/acme/atmos/catalog:stable
    locking:
      required_scope: repository
      repository: coordination
  backups:
    kind: aws/s3
    identity: backup-writer
    packaging: tgz
    options:
      bucket: acme-artifacts
      prefix: atmos
      region: us-east-2
      tags:
        purpose: backup
  local:
    kind: local/dir
    options:
      path: .atmos/artifacts
  archives:
    kind: local/archive
    packaging: tgz
    permissions: "0600"
    options:
      path: .atmos/backups
      key_pattern: '{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.tgz'
  offline:
    kind: oci/layout
    options:
      path: .atmos/oci
  coordination:
    kind: aws/s3
    identity: lock-writer
    options:
      bucket: acme-artifact-coordination
    verification:
      require_signature: true
      trusted_keys:
        - /etc/atmos/cosign.pub
`

func decodeYAMLDocument(t *testing.T, document string) (schema.ArtifactConfig, map[string]any) {
	t.Helper()
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(document), &raw))
	cfg, err := Decode(raw)
	require.NoError(t, err)
	return cfg, raw
}

func TestDecode_PRDDocumentLoadsUnchanged(t *testing.T) {
	cfg, _ := decodeYAMLDocument(t, prdDocument)

	require.Len(t, cfg.Repositories, 6)
	archives := cfg.Repositories["archives"]
	assert.Equal(t, "0600", archives.Permissions)
	assert.Equal(t, `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.tgz`, archives.Options["key_pattern"], "key_pattern survives decoding verbatim")
	assert.Equal(t, "Shared platform artifacts", cfg.Repositories["shared"].Defaults.Metadata.Description)
	assert.Equal(t, []string{"/etc/atmos/cosign.pub"}, cfg.Repositories["coordination"].Verification.TrustedKeys)
	assert.Empty(t, cfg.Verification.TrustedKeys)
	require.Len(t, cfg.Verification.TrustedIdentities, 1)
	assert.Equal(t, "https://token.actions.githubusercontent.com", cfg.Verification.TrustedIdentities[0].Issuer)
}

// TestDecode_YAMLJSONAndMapstructureAgree decodes the same document through YAML, JSON, and a
// plain mapstructure round trip and requires identical values.
func TestDecode_YAMLJSONAndMapstructureAgree(t *testing.T) {
	fromYAML, raw := decodeYAMLDocument(t, prdDocument)

	// YAML round trip: marshal the typed value and decode it again.
	yamlBytes, err := yaml.Marshal(fromYAML)
	require.NoError(t, err)
	var yamlRoundTrip schema.ArtifactConfig
	require.NoError(t, yaml.Unmarshal(yamlBytes, &yamlRoundTrip))
	assert.Equal(t, fromYAML, yamlRoundTrip)

	// JSON round trip.
	jsonBytes, err := json.Marshal(fromYAML)
	require.NoError(t, err)
	var jsonRoundTrip schema.ArtifactConfig
	require.NoError(t, json.Unmarshal(jsonBytes, &jsonRoundTrip))
	assert.Equal(t, fromYAML, jsonRoundTrip)

	// The raw map re-encoded as JSON, then decoded strictly, matches too.
	var rawFromJSON map[string]any
	require.NoError(t, json.Unmarshal(mustJSON(t, raw), &rawFromJSON))
	fromJSONMap, err := Decode(rawFromJSON)
	require.NoError(t, err)
	assert.Equal(t, fromYAML, fromJSONMap)

	// mapstructure struct -> map -> struct round trip with weak typing disabled.
	var asMap map[string]any
	require.NoError(t, mapstructure.Decode(fromYAML, &asMap))
	var mapstructureRoundTrip schema.ArtifactConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &mapstructureRoundTrip, TagName: "mapstructure"})
	require.NoError(t, err)
	require.NoError(t, decoder.Decode(asMap))
	assert.Equal(t, fromYAML, mapstructureRoundTrip)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	require.NoError(t, err)
	return out
}

func TestDecode_OriginsAreNeverSerialized(t *testing.T) {
	cfg, _ := decodeYAMLDocument(t, prdDocument)
	repo := cfg.Repositories["local"]
	repo.PathOrigin = NewOrigin("/etc/atmos/atmos.yaml", true)
	cfg.Repositories["local"] = repo
	cfg.Verification.KeysOrigin = NewOrigin("/etc/atmos/atmos.yaml", false)

	jsonBytes := mustJSON(t, cfg)
	yamlBytes, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	for _, encoded := range []string{string(jsonBytes), string(yamlBytes)} {
		assert.NotContains(t, encoded, "/etc/atmos/atmos.yaml")
		assert.NotContains(t, encoded, "origin")
	}
}

func TestDecode_RejectsUnquotedNumericPermissions(t *testing.T) {
	tests := []struct {
		name     string
		document string
	}{
		{name: "octal literal", document: "repositories:\n  a:\n    kind: local/archive\n    permissions: 0600\n"},
		{name: "decimal literal", document: "repositories:\n  a:\n    kind: local/archive\n    permissions: 384\n"},
		{name: "float", document: "repositories:\n  a:\n    kind: local/archive\n    permissions: 6.0\n"},
		{name: "bool", document: "repositories:\n  a:\n    kind: local/archive\n    permissions: true\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(tt.document), &raw))
			_, err := Decode(raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), "quoted octal string")
			assert.Contains(t, err.Error(), `"a"`)
		})
	}

	var raw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte("repositories:\n  a:\n    kind: local/archive\n    permissions: \"0600\"\n"), &raw))
	cfg, err := Decode(raw)
	require.NoError(t, err)
	assert.Equal(t, "0600", cfg.Repositories["a"].Permissions)
}

func TestDecode_StrictAboutUnknownAndMistypedFields(t *testing.T) {
	tests := []struct {
		name     string
		document string
		wantErr  string
	}{
		{name: "unknown repository field", document: "repositories:\n  a:\n    kind: local/dir\n    pakaging: none\n", wantErr: "pakaging"},
		{name: "unknown top level field", document: "repositorys: {}\n", wantErr: "repositorys"},
		{name: "unknown verification field", document: "verification:\n  require_sig: true\n", wantErr: "require_sig"},
		{name: "non-boolean require_signature", document: "verification:\n  require_signature: sometimes\n", wantErr: "require_signature"},
		{name: "non-string label value", document: "repositories:\n  a:\n    kind: local/dir\n    defaults:\n      metadata:\n        labels:\n          cost: 1234\n", wantErr: "cost"},
		{name: "repositories not a map", document: "repositories: [a]\n", wantErr: "repositories"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(tt.document), &raw))
			_, err := Decode(raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDecode_EmptySectionIsTheZeroValue(t *testing.T) {
	cfg, err := Decode(nil)
	require.NoError(t, err)
	assert.Equal(t, schema.ArtifactConfig{}, cfg)
	cfg, err = Decode(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, cfg.Repositories)
}

func TestDecode_KeysAreCaseSensitiveForAliasesLabelsAndTags(t *testing.T) {
	cfg, _ := decodeYAMLDocument(t, `
repositories:
  Prod.US:
    kind: aws/s3
    options:
      bucket: acme-artifacts
      tags:
        Purpose: Backup
    defaults:
      metadata:
        labels:
          Team.Name: Platform
`)
	repo, ok := cfg.Repositories["Prod.US"]
	require.True(t, ok)
	assert.Equal(t, map[string]string{"Team.Name": "Platform"}, repo.Defaults.Metadata.Labels)
	assert.Equal(t, map[string]any{"Purpose": "Backup"}, repo.Options["tags"])
}

func TestDecodedPRDDocumentValidatesWithAnchoredPaths(t *testing.T) {
	cfg, _ := decodeYAMLDocument(t, prdDocument)
	origin := NewOrigin("/project/atmos.yaml", false)
	for alias, repo := range cfg.Repositories {
		repo.PathOrigin = origin
		cfg.Repositories[alias] = repo
	}
	require.NoError(t, Validate(&cfg))
}
