package artifact

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: a rename of these schema fields must fail the build.
var _ = schema.ArtifactRepositoryConfig{Kind: "", Options: nil, Packaging: "", Permissions: "", Catalog: ""}

func TestDecodeOptions_IsStrict(t *testing.T) {
	tests := []struct {
		name     string
		raw      map[string]any
		wantErr  string
		wantHint bool
	}{
		{name: "valid", raw: map[string]any{"bucket": "acme-artifacts", "region": "us-east-2"}},
		{name: "unknown key is a typo", raw: map[string]any{"buckt": "acme-artifacts"}, wantErr: "buckt"},
		{name: "wrong type", raw: map[string]any{"bucket": 42}, wantErr: "bucket"},
		{name: "credential-looking key gets a hint", raw: map[string]any{"bucket": "acme-artifacts", "secret_key": "x"}, wantErr: "secret_key", wantHint: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var options S3Options
			err := DecodeOptions(tt.raw, &options)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, "acme-artifacts", options.Bucket)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, tt.wantHint, strings.Contains(err.Error(), "auth identity"))
		})
	}
}

func TestOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		wantErr string
	}{
		{name: "local dir ok", options: &LocalDirOptions{Path: ".atmos/artifacts"}},
		{name: "local dir missing path", options: &LocalDirOptions{}, wantErr: `"path" is required`},
		{name: "local dir NUL", options: &LocalDirOptions{Path: "a\x00b"}, wantErr: "NUL"},
		{name: "relative path with traversal is a legitimate destination", options: &LocalDirOptions{Path: "../shared/artifacts"}},
		{name: "archive missing path", options: &LocalArchiveOptions{}, wantErr: `"path" is required`},
		{name: "layout missing path", options: &OCILayoutOptions{}, wantErr: `"path" is required`},
		{name: "s3 ok", options: &S3Options{Bucket: "acme-artifacts", Prefix: "atmos", Region: "us-east-2", Tags: map[string]string{"purpose": "backup"}}},
		{name: "s3 missing bucket", options: &S3Options{}, wantErr: `"bucket" is required`},
		{name: "s3 bad bucket", options: &S3Options{Bucket: "Bad_Bucket"}, wantErr: "not a valid S3 bucket name"},
		{name: "s3 bad region", options: &S3Options{Bucket: "acme-artifacts", Region: "US East"}, wantErr: "not a valid region"},
		{name: "s3 prefix traversal", options: &S3Options{Bucket: "acme-artifacts", Prefix: "a/../b"}, wantErr: "prefix"},
		{name: "s3 absolute prefix", options: &S3Options{Bucket: "acme-artifacts", Prefix: "/atmos"}, wantErr: "prefix"},
		{name: "s3 trailing slash prefix", options: &S3Options{Bucket: "acme-artifacts", Prefix: "atmos/"}, wantErr: "prefix"},
		{name: "s3 reserved prefix", options: &S3Options{Bucket: "acme-artifacts", Prefix: "atmos/.atmos-artifacts"}, wantErr: "reserved"},
		{name: "s3 reserved tag prefix", options: &S3Options{Bucket: "acme-artifacts", Tags: map[string]string{"aws:foo": "x"}}, wantErr: "reserved"},
		{name: "s3 oversized tag value", options: &S3Options{Bucket: "acme-artifacts", Tags: map[string]string{"k": strings.Repeat("x", 257)}}, wantErr: "value exceeds"},
		{name: "s3 too many tags", options: &S3Options{Bucket: "acme-artifacts", Tags: elevenTags()}, wantErr: "exceed the S3 limit"},
		{name: "s3 bad key pattern", options: &S3Options{Bucket: "acme-artifacts", KeyPattern: "../x"}, wantErr: "segment"},
		{name: "registry ok", options: &OCIRegistryOptions{Registry: "ghcr.io", Prefix: "acme/atmos"}},
		{name: "registry with port", options: &OCIRegistryOptions{Registry: "localhost:5000"}},
		{name: "registry missing", options: &OCIRegistryOptions{}, wantErr: `"registry" is required`},
		{name: "registry with scheme", options: &OCIRegistryOptions{Registry: "https://ghcr.io"}, wantErr: "without a scheme"},
		{name: "registry with credentials", options: &OCIRegistryOptions{Registry: "user:pw@ghcr.io"}, wantErr: "without a scheme"},
		{name: "registry uppercase prefix", options: &OCIRegistryOptions{Registry: "ghcr.io", Prefix: "Acme/Atmos"}, wantErr: "not a valid OCI repository path"},
		{name: "registry prefix traversal", options: &OCIRegistryOptions{Registry: "ghcr.io", Prefix: "acme/../x"}, wantErr: "prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func elevenTags() map[string]string {
	tags := make(map[string]string)
	for _, key := range strings.Split("a b c d e f g h i j k", " ") {
		tags[key] = "v"
	}
	return tags
}

func TestValidateKeyPattern(t *testing.T) {
	prd := `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.tgz`
	tests := []struct {
		name       string
		pattern    string
		packaging  Packaging
		wantErr    string
		wantUnsafe bool
	}{
		{name: "empty is fine", pattern: ""},
		{name: "PRD example", pattern: prd, packaging: PackagingTgz},
		{name: "tar.gz extension for tgz", pattern: "{{ .Name }}/{{ .Version }}.tar.gz", packaging: PackagingTgz},
		{name: "extension conflicts with packaging", pattern: "{{ .Name }}/{{ .Version }}.zip", packaging: PackagingTgz, wantErr: "does not match packaging"},
		{name: "archive extension with directory packaging", pattern: "{{ .Name }}/{{ .Version }}.tgz", packaging: PackagingNone, wantErr: "does not match packaging"},
		{name: "extension not checked without packaging", pattern: "{{ .Name }}.zip"},
		{name: "date layout slashes inside action are not segments", pattern: `{{ .CreatedAt | date "2006/01/02" }}/x`},
		{name: "unknown field", pattern: "{{ .Owner }}/x", wantErr: "unknown field .Owner"},
		{name: "unbalanced open", pattern: "{{ .Name /x", wantErr: "unbalanced"},
		{name: "unbalanced close", pattern: ".Name }}/x", wantErr: "unbalanced"},
		{name: "absolute", pattern: "/{{ .Name }}", wantErr: "relative", wantUnsafe: true},
		{name: "traversal", pattern: "{{ .Name }}/../x", wantErr: "segment", wantUnsafe: true},
		{name: "empty segment", pattern: "a//b", wantErr: "segment", wantUnsafe: true},
		{name: "reserved namespace", pattern: ".atmos-artifacts/{{ .Name }}", wantErr: "reserved", wantUnsafe: true},
		{name: "backslash", pattern: `a\b`, wantErr: "backslash", wantUnsafe: true},
		{name: "field name inside string literal is not a field", pattern: `{{ printf ".Owner" }}/{{ .Name }}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeyPattern(tt.pattern, tt.packaging)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, tt.wantUnsafe, strings.Contains(err.Error(), errUtils.ErrArtifactUnsafePath.Error()))
		})
	}
}

func TestValidatePrefix(t *testing.T) {
	require.NoError(t, ValidatePrefix(""))
	require.NoError(t, ValidatePrefix("acme/atmos"))
	for _, bad := range []string{"/abs", "a/./b", "a//b", "..", `a\b`, "a/.atmos-artifacts"} {
		err := ValidatePrefix(bad)
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath, bad)
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, bad)
	}
}

func TestRegistryHostOf(t *testing.T) {
	host, ok := RegistryHostOf("oci://GHCR.io/acme/atmos/catalog:stable")
	require.True(t, ok)
	assert.Equal(t, "ghcr.io", host)
	for _, bad := range []string{"https://ghcr.io/x", "oci:///x", "oci://user:pw@ghcr.io/x", "::"} {
		_, ok := RegistryHostOf(bad)
		assert.False(t, ok, bad)
	}
}

// TestBuiltinKindsMatchTypedOptions keeps each descriptor's path options and typed options struct in step.
func TestBuiltinKindsMatchTypedOptions(t *testing.T) {
	require.NotEmpty(t, BuiltinKinds())
	for _, kind := range BuiltinKinds() {
		desc, ok := BuiltinKind(kind)
		require.True(t, ok, kind)
		require.NotNil(t, desc.NewOptions, kind)
		tags := mapstructureTags(desc.NewOptions())
		for _, path := range desc.PathOptions {
			assert.Contains(t, tags, path, "%s declares path option %q that its typed options do not decode", kind, path)
		}
		assert.True(t, desc.Capabilities.HasPackaging(desc.DefaultPackaging), "%s default packaging must be supported", kind)
		assert.NotEmpty(t, desc.Capabilities.LockScopes, kind)
	}
}

func mapstructureTags(options Options) []string {
	rt := reflect.TypeOf(options).Elem()
	tags := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		tags = append(tags, rt.Field(i).Tag.Get("mapstructure"))
	}
	return tags
}

// TestValidationCreatesNothing proves validation is free of filesystem side effects.
func TestValidationCreatesNothing(t *testing.T) {
	root := t.TempDir()
	cfg := &schema.ArtifactRepositoryConfig{
		Kind:        RepositoryKindLocalArchive,
		Permissions: "0600",
		Options:     map[string]any{"path": filepath.Join(root, "not", "created", "yet"), "key_pattern": "{{ .Name }}/{{ .Version }}.tgz"},
	}
	desc, _ := BuiltinKind(RepositoryKindLocalArchive)
	require.NoError(t, ValidateRepositoryConfig(desc, cfg))

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "validation must not create destination directories")
}
