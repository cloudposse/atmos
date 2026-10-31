package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: renaming a schema field these tests rely on must fail the build.
var _ = schema.ArtifactRepositoryConfig{Kind: "", Identity: "", Packaging: "", Permissions: "", Catalog: ""}

var _ = schema.ArtifactLockingConfig{RequiredScope: "", Repository: ""}

func localRepo(path string) schema.ArtifactRepositoryConfig {
	return schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": path}}
}

func s3Repo() schema.ArtifactRepositoryConfig {
	return schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Options: map[string]any{"bucket": "acme-artifact-coordination"}}
}

func registryRepo() schema.ArtifactRepositoryConfig {
	return schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": "ghcr.io", "prefix": "acme/atmos"}}
}

func validateOne(t *testing.T, alias string, repo *schema.ArtifactRepositoryConfig, opts ...Option) error {
	t.Helper()
	return Validate(&schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{alias: *repo}}, opts...)
}

func TestValidate_NilAndEmptyConfigurationAreValid(t *testing.T) {
	require.NoError(t, Validate(nil))
	require.NoError(t, Validate(&schema.ArtifactConfig{}))
}

func TestValidate_RepositoryBasics(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		alias   string
		mutate  func(*schema.ArtifactRepositoryConfig)
		wantErr string
	}{
		{name: "valid local", alias: "local"},
		{name: "alias with dots and case", alias: "Prod.US-1"},
		{name: "alias with slash", alias: "a/b", wantErr: "alias must start"},
		{name: "alias leading dash", alias: "-x", wantErr: "alias must start"},
		{name: "missing kind", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Kind = "" }, wantErr: "kind is required"},
		{name: "unknown kind", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Kind = "ftp/server" }, wantErr: `unknown kind "ftp/server"`},
		{name: "missing required option", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Options = nil }, wantErr: `"path" is required`},
		{name: "unknown option", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Options["bucket"] = "b" }, wantErr: "bucket"},
		{name: "credentials in options", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Options["access_key"] = "AKIA" }, wantErr: "auth identity"},
		{name: "identity with whitespace", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Identity = "a b" }, wantErr: "without whitespace"},
		{name: "identity that is a URL", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Identity = "https://u:p@host" }, wantErr: "credential or URL"},
		{name: "permissions on a directory", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Permissions = "0600" }, wantErr: "does not accept permissions"},
		{name: "packaging outside the matrix", alias: "x", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Packaging = "tgz" }, wantErr: "does not support packaging"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := localRepo(root)
			if tt.mutate != nil {
				tt.mutate(&repo)
			}
			err := validateOne(t, tt.alias, &repo)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidate_JoinsErrorsAcrossRepositoriesInAliasOrder(t *testing.T) {
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"zeta":  {Kind: "bad/one"},
		"alpha": {Kind: "bad/two"},
	}}
	err := Validate(cfg)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	message := err.Error()
	assert.Less(t, strings.Index(message, `"alpha"`), strings.Index(message, `"zeta"`))
}

func TestValidate_IdentityLookupIsOptional(t *testing.T) {
	repo := localRepo(t.TempDir())
	repo.Identity = "registry-publisher"
	require.NoError(t, validateOne(t, "x", &repo), "identity is resolved lazily; shape only by default")

	err := validateOne(t, "x", &repo, WithIdentityLookup(func(string) bool { return false }))
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "not defined under auth.identities")

	require.NoError(t, validateOne(t, "x", &repo, WithIdentityLookup(func(name string) bool { return name == "registry-publisher" })))
}

func TestValidate_UsesInjectedRegistryDescriptor(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, registry.Register(&artifact.KindDescriptor{Kind: "gcp/gcs", Capabilities: artifact.Capabilities{Packagings: []artifact.Packaging{artifact.PackagingTgz}}, DefaultPackaging: artifact.PackagingTgz}, func(context.Context, string, *schema.ArtifactRepositoryConfig, ...artifact.RepositoryOption) (artifact.Repository, error) {
		return nil, nil
	}))
	require.NoError(t, validateOne(t, "x", &schema.ArtifactRepositoryConfig{Kind: "gcp/gcs"}, WithRegistry(registry)))
	require.Error(t, validateOne(t, "x", &schema.ArtifactRepositoryConfig{Kind: "gcp/gcs"}, WithRegistry(artifact.NewRepositoryRegistry())))
}

func TestValidateRepository_UnknownAlias(t *testing.T) {
	err := NewValidator().ValidateRepository(&schema.ArtifactConfig{}, "nope")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestValidate_RelativePathsNeedALocalDeclaringFile(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		origin  schema.ArtifactConfigOrigin
		path    string
		wantErr string
	}{
		{name: "relative with local origin", origin: NewOrigin(filepath.Join(dir, "atmos.yaml"), false), path: "store"},
		{name: "absolute needs no origin", path: filepath.Join(dir, "store")},
		{name: "relative from remote import", origin: NewOrigin(filepath.Join(dir, "tmp", "remote.yaml"), true), path: "store", wantErr: "remote import"},
		{name: "relative with unknown origin", path: "store", wantErr: "no declaring configuration file"},
		{name: "absolute from remote import is allowed", origin: NewOrigin(filepath.Join(dir, "tmp", "remote.yaml"), true), path: filepath.Join(dir, "store")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := localRepo(tt.path)
			repo.PathOrigin = tt.origin
			err := validateOne(t, "x", &repo)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAnchorPath(t *testing.T) {
	dir := t.TempDir()
	origin := NewOrigin(filepath.Join(dir, "conf", "atmos.yaml"), false)
	assert.Equal(t, filepath.Join(dir, "conf"), origin.Dir)

	got, err := AnchorPath(origin, filepath.Join("..", "shared", "store"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "shared", "store"), got)

	absolute := filepath.Join(dir, "abs", "..", "abs2")
	got, err = AnchorPath(origin, absolute)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "abs2"), got)

	assert.True(t, NewOrigin("", true).Remote)
	assert.True(t, NewOrigin("", false).IsZero())
}

func TestValidate_DefaultsMetadata(t *testing.T) {
	repo := localRepo(t.TempDir())
	repo.Defaults = &schema.ArtifactRepositoryDefaults{Metadata: &schema.ArtifactMetadataDefaults{
		Description: "backups",
		Labels:      map[string]string{"team": "platform", "bad=key": "x", "also": "bad,value"},
	}}
	err := validateOne(t, "x", &repo)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "defaults.metadata.labels")
	assert.Contains(t, err.Error(), "bad=key")
	assert.Contains(t, err.Error(), `"also"`)

	repo.Defaults.Metadata.Labels = map[string]string{"team": "platform"}
	require.NoError(t, validateOne(t, "x", &repo))

	repo.Defaults.Metadata.Description = strings.Repeat("d", 4097)
	require.ErrorIs(t, validateOne(t, "x", &repo), errUtils.ErrArtifactConfigInvalid)
}

func TestValidate_CatalogReferences(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*schema.ArtifactRepositoryConfig)
		wantErr string
	}{
		{name: "PRD example", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://ghcr.io/acme/atmos/catalog:stable" }},
		{name: "digest reference", mutate: func(r *schema.ArtifactRepositoryConfig) {
			r.Catalog = "oci://ghcr.io/acme/atmos/catalog@sha256:" + strings.Repeat("a", 64)
		}},
		{name: "host compared case-insensitively", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://GHCR.io/acme/catalog:stable" }},
		{name: "another registry host", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://evil.example/acme/catalog:stable" }, wantErr: "does not match the repository registry"},
		{name: "wrong scheme", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "https://ghcr.io/acme/catalog:stable" }, wantErr: "oci://"},
		{name: "no tag or digest", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://ghcr.io/acme/catalog" }, wantErr: "no implicit latest"},
		{name: "credentials", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://user:hunter2@ghcr.io/acme/catalog:stable" }, wantErr: "credentials"},
		{name: "query", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://ghcr.io/acme/catalog:stable?token=x" }, wantErr: "credentials, a query"},
		{name: "bad tag", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://ghcr.io/acme/catalog:-bad" }, wantErr: "tag syntax"},
		{name: "bad digest", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://ghcr.io/acme/catalog@sha256:xyz" }, wantErr: "digest"},
		{name: "uppercase path", mutate: func(r *schema.ArtifactRepositoryConfig) { r.Catalog = "oci://ghcr.io/Acme/catalog:stable" }, wantErr: "lowercase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := registryRepo()
			tt.mutate(&repo)
			err := validateOne(t, "shared", &repo)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NotContains(t, err.Error(), "hunter2", "credentials are never echoed")
		})
	}

	s3 := s3Repo()
	s3.Catalog = "oci://ghcr.io/acme/catalog:stable"
	err := validateOne(t, "backups", &s3)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "does not support a catalog")
}

func TestValidate_PerformsNoFilesystemSideEffects(t *testing.T) {
	root := t.TempDir()
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"local":    localRepo(filepath.Join(root, "dest", "local")),
		"archives": {Kind: artifact.RepositoryKindLocalArchive, Permissions: "0600", Options: map[string]any{"path": filepath.Join(root, "dest", "archives"), "key_pattern": "{{ .Name }}.tgz"}},
		"layout":   {Kind: artifact.RepositoryKindOCILayout, Options: map[string]any{"path": filepath.Join(root, "dest", "layout")}},
	}}
	require.NoError(t, Validate(cfg))

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "validating or describing configuration must not create destination directories")
}
