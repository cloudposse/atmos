package registry

import (
	"testing"

	getter "github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitModuleSource(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantBase   string
		wantSubdir string
	}{
		{"git no subdir", "git::https://github.com/org/repo.git?ref=v1", "git::https://github.com/org/repo.git?ref=v1", ""},
		{"git with subdir", "git::https://github.com/org/repo.git//modules/foo?ref=v1", "git::https://github.com/org/repo.git?ref=v1", "modules/foo"},
		{"http archive", "https://example.com/mod-1.0.0.tar.gz", "https://example.com/mod-1.0.0.tar.gz", ""},
		{"http archive subdir", "https://example.com/mod.tar.gz//sub", "https://example.com/mod.tar.gz", "sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, subdir := splitModuleSource(tt.source)
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantSubdir, subdir)
		})
	}
}

func TestEncodeDecodeModuleSource(t *testing.T) {
	const base = "git::https://github.com/org/repo.git?ref=v1.2.3"
	enc := encodeModuleSource(base)
	assert.NotContains(t, enc, "/", "encoding must be a single URL path segment")

	got, err := decodeModuleSource(enc)
	require.NoError(t, err)
	assert.Equal(t, base, got)

	_, err = decodeModuleSource("not valid base64!!")
	assert.ErrorIs(t, err, ErrInvalidModulePath)
}

func TestModuleSourceDigest_StableAndSubdirIndependent(t *testing.T) {
	const repo = "git::https://github.com/org/monorepo.git"
	base1, sub1 := splitModuleSource(repo + "//modules/foo?ref=v1")
	base2, sub2 := splitModuleSource(repo + "//modules/bar?ref=v1")

	require.Equal(t, "modules/foo", sub1)
	require.Equal(t, "modules/bar", sub2)
	// Different subdirs of the same repo+ref share a base, so they key to the same
	// cached source (fetched once).
	assert.Equal(t, moduleSourceDigest(base1), moduleSourceDigest(base2))
	// A different ref keys differently.
	assert.NotEqual(t, moduleSourceDigest(base1), moduleSourceDigest("git::https://github.com/org/monorepo.git?ref=v2"))
}

// TestModuleSourceProxyURL_RoundTripsThroughGoGetter verifies that the X-Terraform-Get
// value we emit is parsed back by go-getter (the Terraform side) into the _source GET
// URL plus the original subdir — so the proxy receives a clean two-segment path and the
// client extracts the subdir after unpacking.
func TestModuleSourceProxyURL_RoundTripsThroughGoGetter(t *testing.T) {
	const proxyBase = "https://127.0.0.1:55555/"
	const original = "git::https://github.com/org/repo.git//modules/foo?ref=v1"

	base, subdir := splitModuleSource(original)
	rewritten := moduleSourceProxyURL(proxyBase, base, subdir)

	getURL, gotSubdir := getter.SourceDirSubdir(rewritten)
	assert.Equal(t, "modules/foo", gotSubdir, "subdir must survive for client-side extraction")
	assert.Equal(t, proxyBase+"modules/"+moduleSourceSegment+"/"+encodeModuleSource(base)+sourceExt, getURL)
}

func TestParseProviderArchive(t *testing.T) {
	ref, ok := parseProviderArchive("terraform-provider-aws_5.95.0_linux_amd64.zip")
	assert.True(t, ok)
	assert.Equal(t, "5.95.0", ref.version)
	assert.Equal(t, "linux", ref.os)
	assert.Equal(t, "amd64", ref.arch)

	_, ok = parseProviderArchive("not-a-provider.zip")
	assert.False(t, ok)
}
