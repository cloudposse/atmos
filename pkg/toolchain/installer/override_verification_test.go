package installer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

func TestApplyOverrideVerificationFields(t *testing.T) {
	enabled := true
	tool := &registry.Tool{
		Replacements: map[string]string{"amd64": "x86_64"},
	}
	override := &registry.Override{
		Asset:        "tool_{{.Version}}.tar.gz",
		Format:       "tar.gz",
		Files:        []registry.File{{Name: "tool", Src: "bin/tool"}},
		Replacements: map[string]string{"darwin": "macos"},
		Checksum: registry.ChecksumConfig{
			Enabled:   &enabled,
			Algorithm: "sha512",
		},
		Cosign: registry.CosignConfig{
			Bundle: registry.DownloadedFile{URL: "bundle"},
		},
		SLSAProvenance: registry.SLSAProvenance{
			SourceURI: "github.com/owner/tool",
		},
		Minisign: registry.MinisignConfig{
			PublicKey: "RW...",
		},
		GitHubArtifactAttestations: registry.GitHubArtifactAttestations{
			SignerWorkflow: "release.yaml",
		},
	}

	applyOverride(tool, override)

	assert.Equal(t, override.Asset, tool.Asset)
	assert.Equal(t, override.Format, tool.Format)
	assert.Equal(t, override.Files, tool.Files)
	assert.Equal(t, "x86_64", tool.Replacements["amd64"])
	assert.Equal(t, "macos", tool.Replacements["darwin"])
	assert.Equal(t, "sha512", tool.Checksum.Algorithm)
	assert.Equal(t, "bundle", tool.Cosign.Bundle.URL)
	assert.Equal(t, "github.com/owner/tool", tool.SLSAProvenance.SourceURI)
	assert.Equal(t, "RW...", tool.Minisign.PublicKey)
	assert.Equal(t, "release.yaml", tool.GitHubArtifactAttestations.SignerWorkflow)
}
