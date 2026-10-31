package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	artifactconfig "github.com/cloudposse/atmos/pkg/artifact/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prdRepositories is the YAML from the artifacts PRD "Public configuration" section, verbatim.
const prdRepositories = `artifacts:
  repositories:
    shared:
      kind: oci/registry
      identity: registry-publisher
      options:
        registry: ghcr.io
        prefix: acme/atmos
      defaults:
        metadata:
          labels:
            team: platform

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
`

// artifactsTestDir creates a config root whose atmos.yaml is main, isolated from the host.
func artifactsTestDir(t *testing.T, main string) string {
	t.Helper()
	setupTestAdapters()
	dir := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", dir)
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	writeArtifactsFile(t, filepath.Join(dir, "atmos.yaml"), "base_path: ./\n"+main)
	return dir
}

func writeArtifactsFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func loadArtifactsDir(t *testing.T, dir string) (schema.AtmosConfiguration, error) {
	t.Helper()
	return LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
}

func TestLoadConfig_ArtifactsPRDExampleLoadsUnchanged(t *testing.T) {
	dir := artifactsTestDir(t, prdRepositories)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)

	repos := cfg.Artifacts.Repositories
	require.Len(t, repos, 5)

	shared := repos["shared"]
	assert.Equal(t, artifact.RepositoryKindOCIRegistry, shared.Kind)
	assert.Equal(t, "registry-publisher", shared.Identity)
	assert.Equal(t, map[string]any{"registry": "ghcr.io", "prefix": "acme/atmos"}, shared.Options)
	require.NotNil(t, shared.Defaults)
	require.NotNil(t, shared.Defaults.Metadata)
	assert.Equal(t, map[string]string{"team": "platform"}, shared.Defaults.Metadata.Labels)

	backups := repos["backups"]
	assert.Equal(t, "tgz", backups.Packaging)
	assert.Equal(t, map[string]any{"purpose": "backup"}, backups.Options["tags"])

	// Relative local roots are anchored to the declaring file.
	assert.Equal(t, filepath.Join(dir, ".atmos", "artifacts"), repos["local"].Options["path"])
	assert.Equal(t, artifact.RepositoryKindOCILayout, repos["offline"].Kind)
	assert.Equal(t, filepath.Join(dir, ".atmos", "oci"), repos["offline"].Options["path"])

	archives := repos["archives"]
	assert.Equal(t, "0600", archives.Permissions)
	assert.Equal(t, filepath.Join(dir, ".atmos", "backups"), archives.Options["path"])
	// The key_pattern template survives config loading verbatim; it is never rendered here.
	assert.Equal(t, `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.tgz`, archives.Options["key_pattern"])

	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))
}

func TestLoadConfig_ArtifactsCatalogAndVerificationExamplesLoad(t *testing.T) {
	dir := artifactsTestDir(t, `artifacts:
  verification:
    require_signature: true
    trusted_identities:
      - identity: https://github.com/acme/images/.github/workflows/release.yml@refs/heads/main
        issuer: https://token.actions.githubusercontent.com
    trusted_keys: []
  repositories:
    shared:
      kind: oci/registry
      identity: registry-reader
      options:
        registry: ghcr.io
        prefix: acme/atmos
      catalog: oci://ghcr.io/acme/atmos/catalog:stable
      locking:
        required_scope: repository
        repository: coordination
    coordination:
      kind: aws/s3
      identity: lock-writer
      options:
        bucket: acme-artifact-coordination
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))

	require.NotNil(t, cfg.Artifacts.Verification)
	require.NotNil(t, cfg.Artifacts.Verification.RequireSignature)
	assert.True(t, *cfg.Artifacts.Verification.RequireSignature)
	assert.Equal(t, "oci://ghcr.io/acme/atmos/catalog:stable", cfg.Artifacts.Repositories["shared"].Catalog)
	assert.Equal(t, "coordination", cfg.Artifacts.Repositories["shared"].Locking.Repository)
}

func TestLoadConfig_ArtifactsPreserveKeyCase(t *testing.T) {
	dir := artifactsTestDir(t, `artifacts:
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
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	repo, ok := cfg.Artifacts.Repositories["Prod.US"]
	require.True(t, ok, "repository alias keeps its case and dot, got %v", cfg.Artifacts.Repositories)
	assert.Equal(t, map[string]any{"Purpose": "Backup"}, repo.Options["tags"])
	assert.Equal(t, map[string]string{"Team.Name": "Platform"}, repo.Defaults.Metadata.Labels)
}

func TestLoadConfig_ArtifactsImportedPathsResolveAgainstDeclaringFile(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - ./first/artifacts.yaml\n  - ./second/artifacts.yaml\n")
	writeArtifactsFile(t, filepath.Join(dir, "first", "artifacts.yaml"), `artifacts:
  repositories:
    one:
      kind: local/dir
      options:
        path: store-first
    two:
      kind: local/archive
      identity: declared-in-first
`)
	writeArtifactsFile(t, filepath.Join(dir, "second", "artifacts.yaml"), `artifacts:
  repositories:
    one:
      identity: declared-in-second
    two:
      options:
        path: store-second
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)

	one := cfg.Artifacts.Repositories["one"]
	two := cfg.Artifacts.Repositories["two"]
	// Merged fields come from different files, and each path uses the file that supplied that path.
	assert.Equal(t, artifact.RepositoryKindLocalDir, one.Kind)
	assert.Equal(t, "declared-in-second", one.Identity)
	assert.Equal(t, filepath.Join(dir, "first", "store-first"), one.Options["path"])
	assert.Equal(t, filepath.Join(dir, "first"), one.PathOrigin.Dir)

	assert.Equal(t, artifact.RepositoryKindLocalArchive, two.Kind)
	assert.Equal(t, "declared-in-first", two.Identity)
	assert.Equal(t, filepath.Join(dir, "second", "store-second"), two.Options["path"])
	assert.Equal(t, filepath.Join(dir, "second"), two.PathOrigin.Dir)
	assert.False(t, two.PathOrigin.Remote)

	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))
}

func TestLoadConfig_ArtifactsLaterFileOverridesPathAndOrigin(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - ./base/artifacts.yaml\n"+`artifacts:
  repositories:
    local:
      options:
        path: override
`)
	writeArtifactsFile(t, filepath.Join(dir, "base", "artifacts.yaml"), `artifacts:
  repositories:
    local:
      kind: local/dir
      options:
        path: from-base
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "override"), cfg.Artifacts.Repositories["local"].Options["path"])
	assert.Equal(t, dir, cfg.Artifacts.Repositories["local"].PathOrigin.Dir)
}

func TestLoadConfig_ArtifactsAnchorTrustedKeys(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - ./trust/artifacts.yaml\n")
	writeArtifactsFile(t, filepath.Join(dir, "trust", "artifacts.yaml"), `artifacts:
  verification:
    trusted_keys:
      - keys/cosign.pub
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "trust", "keys", "cosign.pub")}, cfg.Artifacts.Verification.TrustedKeys)
}

func TestLoadConfig_ArtifactsRejectsUnquotedNumericPermissions(t *testing.T) {
	dir := artifactsTestDir(t, `artifacts:
  repositories:
    archives:
      kind: local/archive
      permissions: 0600
      options:
        path: out
`)
	_, err := loadArtifactsDir(t, dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "quoted octal string")
}

func TestLoadConfig_ArtifactsRejectsUnknownField(t *testing.T) {
	dir := artifactsTestDir(t, `artifacts:
  repositories:
    local:
      kind: local/dir
      pakaging: none
`)
	_, err := loadArtifactsDir(t, dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "pakaging")
}

func TestLoadConfig_ArtifactsAbsentLeavesZeroValue(t *testing.T) {
	dir := artifactsTestDir(t, "")
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.Empty(t, cfg.Artifacts.Repositories)
	assert.Nil(t, cfg.Artifacts.Verification)
}

// remoteTestAdapter simulates a remote import: it copies a canned document into the import temp
// directory, exactly as the go-getter adapter downloads into an ephemeral location.
type remoteTestAdapter struct {
	documents map[string]string
}

func (a *remoteTestAdapter) Schemes() []string { return []string{"remote-test://"} }

func (a *remoteTestAdapter) Resolve(_ context.Context, importPath, _, tempDir string, _, _ int, _ *schema.AtmosConfiguration) ([]ResolvedPaths, error) {
	name := strings.TrimPrefix(importPath, "remote-test://")
	target := filepath.Join(tempDir, name+".yaml")
	if err := os.WriteFile(target, []byte(a.documents[name]), 0o600); err != nil {
		return nil, err
	}
	return []ResolvedPaths{{FilePath: target, ImportPaths: importPath, ImportType: REMOTE}}, nil
}

func registerRemoteArtifacts(documents map[string]string) {
	RegisterImportAdapter(&remoteTestAdapter{documents: documents})
}

const remoteRelativeDocument = `artifacts:
  repositories:
    remote:
      kind: local/dir
      options:
        path: relative-from-remote
`

func TestLoadConfig_ArtifactsRemoteRelativePathIsNeverAnchoredToDownloadDir(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - remote-test://shared\n")
	registerRemoteArtifacts(map[string]string{"shared": remoteRelativeDocument})

	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)

	repo := cfg.Artifacts.Repositories["remote"]
	assert.Equal(t, "relative-from-remote", repo.Options["path"], "the ephemeral download directory must not become a write root")
	assert.True(t, repo.PathOrigin.Remote)

	err = artifactconfig.Validate(&cfg.Artifacts)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
	assert.Contains(t, err.Error(), "remote import")
}

func TestLoadConfig_ArtifactsRemoteDeclaredPathAcceptsLocalOverride(t *testing.T) {
	dir := artifactsTestDir(t, `import:
  - remote-test://shared
artifacts:
  repositories:
    remote:
      options:
        path: local-override
`)
	registerRemoteArtifacts(map[string]string{"shared": remoteRelativeDocument})

	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)

	repo := cfg.Artifacts.Repositories["remote"]
	assert.Equal(t, filepath.Join(dir, "local-override"), repo.Options["path"])
	assert.False(t, repo.PathOrigin.Remote)
	assert.Equal(t, artifact.RepositoryKindLocalDir, repo.Kind, "the remote file still supplies the other fields")
	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))
}

func TestLoadConfig_ArtifactsRemoteAbsolutePathIsAccepted(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - remote-test://shared\n")
	absolute := filepath.Join(t.TempDir(), "store")
	registerRemoteArtifacts(map[string]string{"shared": strings.Replace(remoteRelativeDocument, "relative-from-remote", filepath.ToSlash(absolute), 1)})

	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.Equal(t, absolute, cfg.Artifacts.Repositories["remote"].Options["path"])
	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))
}

func TestLoadConfig_ArtifactsLocalImportIsNotEphemeral(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - ./local/artifacts.yaml\n")
	writeArtifactsFile(t, filepath.Join(dir, "local", "artifacts.yaml"), remoteRelativeDocument)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.False(t, cfg.Artifacts.Repositories["remote"].PathOrigin.Remote)
	assert.Equal(t, filepath.Join(dir, "local", "relative-from-remote"), cfg.Artifacts.Repositories["remote"].Options["path"])
}

func TestLoadConfig_ArtifactsAtmosDFragmentAnchorsToItsOwnDirectory(t *testing.T) {
	dir := artifactsTestDir(t, "")
	writeArtifactsFile(t, filepath.Join(dir, ".atmos.d", "artifacts.yaml"), `artifacts:
  repositories:
    fragment:
      kind: local/dir
      options:
        path: store
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".atmos.d", "store"), cfg.Artifacts.Repositories["fragment"].Options["path"])
}

// TestLoadConfig_ArtifactsDescribeRoundTrip covers `atmos describe config`: the loaded section
// serializes under `artifacts` and reads back as the same values, without leaking load-time origins.
func TestLoadConfig_ArtifactsDescribeRoundTrip(t *testing.T) {
	dir := artifactsTestDir(t, prdRepositories)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(cfg)
	require.NoError(t, err)
	var describedJSON struct {
		Artifacts schema.ArtifactConfig `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal(jsonBytes, &describedJSON))

	yamlBytes, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	var describedYAML struct {
		Artifacts schema.ArtifactConfig `yaml:"artifacts"`
	}
	require.NoError(t, yaml.Unmarshal(yamlBytes, &describedYAML))

	expected := cfg.Artifacts
	for alias, repo := range expected.Repositories {
		repo.PathOrigin = schema.ArtifactConfigOrigin{}
		expected.Repositories[alias] = repo
	}
	assert.Equal(t, expected, describedJSON.Artifacts)
	assert.Equal(t, expected, describedYAML.Artifacts)
	assert.NotContains(t, string(jsonBytes), "PathOrigin")
}
