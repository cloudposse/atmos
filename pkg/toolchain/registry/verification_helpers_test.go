package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerificationConfigPredicates(t *testing.T) {
	enabled := true

	assert.False(t, HasDownloadedFile(nil))
	assert.True(t, HasDownloadedFile(&DownloadedFile{RepoOwner: "owner"}))

	assert.False(t, HasChecksumConfig(nil))
	assert.True(t, HasChecksumConfig(&ChecksumConfig{Enabled: &enabled}))
	assert.True(t, HasChecksumConfig(&ChecksumConfig{Pattern: ChecksumPattern{File: "tool"}}))
	assert.True(t, HasChecksumConfig(&ChecksumConfig{Replacements: map[string]string{"darwin": "macos"}}))
	assert.True(t, HasChecksumConfig(&ChecksumConfig{Cosign: CosignConfig{Bundle: DownloadedFile{URL: "bundle"}}}))
	assert.True(t, HasChecksumConfig(&ChecksumConfig{Minisign: MinisignConfig{PublicKey: "key"}}))
	assert.True(t, HasChecksumConfig(&ChecksumConfig{GitHubArtifactAttestations: GitHubArtifactAttestations{SignerWorkflow: "release.yaml"}}))

	assert.False(t, HasCosignConfig(nil))
	assert.True(t, HasCosignConfig(&CosignConfig{Opts: []string{"--signature", "sig"}}))
	assert.True(t, HasCosignConfig(&CosignConfig{Certificate: DownloadedFile{Asset: "cert"}}))

	assert.False(t, HasSLSAProvenance(nil))
	assert.True(t, HasSLSAProvenance(&SLSAProvenance{RepoName: "tool"}))
	assert.True(t, HasSLSAProvenance(&SLSAProvenance{SourceTag: "v1.0.0"}))

	assert.False(t, HasMinisignConfig(nil))
	assert.True(t, HasMinisignConfig(&MinisignConfig{RepoOwner: "owner"}))
	assert.True(t, HasMinisignConfig(&MinisignConfig{URL: "tool.minisig"}))

	assert.False(t, HasGitHubArtifactAttestations(nil))
	assert.True(t, HasGitHubArtifactAttestations(&GitHubArtifactAttestations{PredicateType: "https://slsa.dev/provenance/v1"}))
}

func TestSearchAndListOptions(t *testing.T) {
	search := &SearchConfig{}
	WithLimit(10)(search)
	WithOffset(2)(search)
	WithInstalledOnly(true)(search)
	WithAvailableOnly(true)(search)

	assert.Equal(t, 10, search.Limit)
	assert.Equal(t, 2, search.Offset)
	assert.True(t, search.InstalledOnly)
	assert.True(t, search.AvailableOnly)

	list := &ListConfig{}
	WithListLimit(20)(list)
	WithListOffset(4)(list)
	WithSort("name")(list)

	assert.Equal(t, 20, list.Limit)
	assert.Equal(t, 4, list.Offset)
	assert.Equal(t, "name", list.Sort)
}
