package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateRepositoryConfig_PackagingMatrix(t *testing.T) {
	tests := []struct {
		kind      string
		options   map[string]any
		packaging string
		valid     bool
	}{
		{kind: RepositoryKindLocalDir, packaging: "", valid: true},
		{kind: RepositoryKindLocalDir, packaging: "none", valid: true},
		{kind: RepositoryKindLocalDir, packaging: "tar"},
		{kind: RepositoryKindLocalDir, packaging: "tgz"},
		{kind: RepositoryKindLocalDir, packaging: "zip"},
		{kind: RepositoryKindLocalArchive, packaging: "", valid: true},
		{kind: RepositoryKindLocalArchive, packaging: "none"},
		{kind: RepositoryKindLocalArchive, packaging: "tar", valid: true},
		{kind: RepositoryKindLocalArchive, packaging: "tgz", valid: true},
		{kind: RepositoryKindLocalArchive, packaging: "zip", valid: true},
		{kind: RepositoryKindS3, packaging: "", valid: true},
		{kind: RepositoryKindS3, packaging: "none", valid: true},
		{kind: RepositoryKindS3, packaging: "tar", valid: true},
		{kind: RepositoryKindS3, packaging: "tgz", valid: true},
		{kind: RepositoryKindS3, packaging: "zip", valid: true},
		{kind: RepositoryKindS3, packaging: "rar"},
		{kind: RepositoryKindOCIRegistry, packaging: "", valid: true},
		{kind: RepositoryKindOCIRegistry, packaging: "none"},
		{kind: RepositoryKindOCIRegistry, packaging: "tar", valid: true},
		{kind: RepositoryKindOCILayout, packaging: "none"},
		{kind: RepositoryKindOCILayout, packaging: "zip", valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.packaging, func(t *testing.T) {
			desc, ok := BuiltinKind(tt.kind)
			require.True(t, ok)
			cfg := &schema.ArtifactRepositoryConfig{Kind: tt.kind, Packaging: tt.packaging, Options: validOptionsFor(tt.kind)}
			err := ValidateRepositoryConfig(desc, cfg)
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
		})
	}
}

func validOptionsFor(kind string) map[string]any {
	switch kind {
	case RepositoryKindS3:
		return map[string]any{"bucket": "acme-artifacts"}
	case RepositoryKindOCIRegistry:
		return map[string]any{"registry": "ghcr.io"}
	default:
		return map[string]any{"path": "out"}
	}
}

func TestValidateRepositoryConfig_Permissions(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		permissions string
		wantErr     bool
	}{
		{name: "archive accepts quoted octal", kind: RepositoryKindLocalArchive, permissions: "0600"},
		{name: "archive accepts three digits", kind: RepositoryKindLocalArchive, permissions: "640"},
		{name: "archive rejects preserve", kind: RepositoryKindLocalArchive, permissions: "preserve", wantErr: true},
		{name: "archive rejects privileged bits", kind: RepositoryKindLocalArchive, permissions: "4755", wantErr: true},
		{name: "archive rejects 0o prefix", kind: RepositoryKindLocalArchive, permissions: "0o600", wantErr: true},
		{name: "archive rejects decimal-looking 384", kind: RepositoryKindLocalArchive, permissions: "384", wantErr: true},
		{name: "directory rejects permissions", kind: RepositoryKindLocalDir, permissions: "0600", wantErr: true},
		{name: "s3 rejects permissions", kind: RepositoryKindS3, permissions: "0600", wantErr: true},
		{name: "registry rejects permissions", kind: RepositoryKindOCIRegistry, permissions: "0600", wantErr: true},
		{name: "layout rejects permissions", kind: RepositoryKindOCILayout, permissions: "0600", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, _ := BuiltinKind(tt.kind)
			err := ValidateRepositoryConfig(desc, &schema.ArtifactRepositoryConfig{Kind: tt.kind, Permissions: tt.permissions, Options: validOptionsFor(tt.kind)})
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateRepositoryConfig_JoinsEveryProblem(t *testing.T) {
	desc, _ := BuiltinKind(RepositoryKindLocalArchive)
	err := ValidateRepositoryConfig(desc, &schema.ArtifactRepositoryConfig{
		Kind: RepositoryKindLocalArchive, Packaging: "none", Permissions: "preserve", Options: map[string]any{"pth": "x"},
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "preserve")
	assert.Contains(t, err.Error(), "pth")
}

func TestValidateRepositoryConfig_KeyPatternAgreesWithEffectivePackaging(t *testing.T) {
	desc, _ := BuiltinKind(RepositoryKindLocalArchive)
	cfg := &schema.ArtifactRepositoryConfig{
		Kind: RepositoryKindLocalArchive, Packaging: "zip",
		Options: map[string]any{"path": "out", "key_pattern": "{{ .Name }}/{{ .Version }}.tgz"},
	}
	require.ErrorIs(t, ValidateRepositoryConfig(desc, cfg), errUtils.ErrArtifactConfigInvalid)

	cfg.Packaging = ""
	require.NoError(t, ValidateRepositoryConfig(desc, cfg), "the default packaging for local/archive is tgz")
}

func TestValidateRepositoryConfig_NilConfig(t *testing.T) {
	desc, _ := BuiltinKind(RepositoryKindLocalDir)
	require.ErrorIs(t, ValidateRepositoryConfig(desc, nil), errUtils.ErrArtifactConfigInvalid)
}

func TestKindDescriptorHelpers(t *testing.T) {
	desc, _ := BuiltinKind(RepositoryKindLocalArchive)
	assert.True(t, desc.UsesLocalPath("path"))
	assert.False(t, desc.UsesLocalPath("key_pattern"))
	assert.Equal(t, PackagingTgz, desc.EffectivePackaging(""))
	assert.Equal(t, PackagingZip, desc.EffectivePackaging("zip"))

	_, ok := BuiltinKind("nope")
	assert.False(t, ok)
	assert.Equal(t, []string{"aws/s3", "local/archive", "local/dir", "oci/layout", "oci/registry"}, BuiltinKinds())
}

func TestBuiltinKindsAreIndependentCopies(t *testing.T) {
	first, _ := BuiltinKind(RepositoryKindS3)
	first.Capabilities.Packagings[0] = PackagingZip
	second, _ := BuiltinKind(RepositoryKindS3)
	assert.Equal(t, PackagingNone, second.Capabilities.Packagings[0], "mutating a returned descriptor must not change the built-in set")
}
