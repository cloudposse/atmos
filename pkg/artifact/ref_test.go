package artifact

import (
	"errors"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const testSHA = "sha256:0d6e4079e36703ebd37c00722f5891d28b0e2811dc114b129215123adcce3605"

func TestParseRef(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Ref
	}{
		{"nested name and version", "components/terraform/vpc:1.2.0", Ref{Name: "components/terraform/vpc", Version: "1.2.0"}},
		{"single segment", "vpc:v1", Ref{Name: "vpc", Version: "v1"}},
		{"CI build identifier version", "app:build-2026.10.30_7", Ref{Name: "app", Version: "build-2026.10.30_7"}},
		{"digest reference", "components/vpc@" + testSHA, Ref{Name: "components/vpc", Digest: testSHA}},
		{"separators in segments", "a.b/c_d/e__f/g--h:1", Ref{Name: "a.b/c_d/e__f/g--h", Version: "1"}},
		{"128 character version", "a:" + strings.Repeat("v", 128), Ref{Name: "a", Version: strings.Repeat("v", 128)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRef(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.input, got.String(), "String round-trips the textual form")
			assert.Equal(t, tt.want.Digest != "", got.IsDigest())
		})
	}
}

func TestParseRefRejects(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		unsafe bool
	}{
		{name: "empty", input: ""},
		{name: "no version or digest", input: "components/vpc"},
		{name: "empty version", input: "vpc:"},
		{name: "empty name", input: ":1.0"},
		{name: "absolute path", input: "/abs/path:1", unsafe: true},
		{name: "traversal segment", input: "a/../b:1", unsafe: true},
		{name: "leading traversal", input: "../a:1", unsafe: true},
		{name: "dot segment", input: "a/./b:1", unsafe: true},
		{name: "empty segment", input: "a//b:1", unsafe: true},
		{name: "trailing slash", input: "a/b/:1", unsafe: true},
		{name: "backslash", input: `a\b:1`, unsafe: true},
		{name: "uppercase is not normalized", input: "Components/Vpc:1"},
		{name: "space", input: "a b:1"},
		{name: "reserved namespace shaped name", input: ".atmos-artifacts/x:1", unsafe: false},
		{name: "version with plus", input: "a:1.0+build"},
		{name: "version starting with dash", input: "a:-1"},
		{name: "version starting with dot", input: "a:.1"},
		{name: "version too long", input: "a:" + strings.Repeat("v", 129)},
		{name: "version and digest", input: "a:1@" + testSHA},
		{name: "unsupported digest algorithm", input: "a@sha512:" + strings.Repeat("a", 128)},
		{name: "short digest", input: "a@sha256:abc"},
		{name: "uppercase digest hex", input: "a@sha256:" + strings.ToUpper(strings.TrimPrefix(testSHA, "sha256:"))},
		{name: "digest without name", input: "@" + testSHA},
		{name: "name too long", input: strings.Repeat("a/", 128) + "a:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRef(tt.input)
			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
			assert.Equal(t, tt.unsafe, isUnsafe(err), "unsafe-path classification of %v", err)
		})
	}
}

func isUnsafe(err error) bool {
	return errors.Is(err, errUtils.ErrArtifactUnsafePath)
}

func TestRefValidate(t *testing.T) {
	tests := []struct {
		name string
		ref  Ref
		ok   bool
	}{
		{"version", Ref{Name: "a", Version: "1"}, true},
		{"digest", Ref{Name: "a", Digest: testSHA}, true},
		{"neither", Ref{Name: "a"}, false},
		{"both", Ref{Name: "a", Version: "1", Digest: digest.Digest(testSHA)}, false},
		{"bad name", Ref{Name: "A", Version: "1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.Validate()
			if tt.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
		})
	}
}

func TestValidateKind(t *testing.T) {
	for _, kind := range []string{"files", "component/terraform", "toolchain.package", "ci/planfile", "catalog"} {
		assert.NoError(t, ValidateKind(kind), kind)
	}
	for _, kind := range []string{"", "Files", "a//b", "/a", "a/", "a b", strings.Repeat("k", MaxKindLength+1)} {
		assert.ErrorIs(t, ValidateKind(kind), errUtils.ErrArtifactRecordInvalid, kind)
	}
}
