package installer

import (
	"runtime"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// ApplyPlatformOverrides applies platform-specific overrides to the tool configuration.
// If an override matches the current GOOS/GOARCH, it updates the tool's Asset, Format,
// Files, and Replacements fields accordingly. First matching override wins.
func ApplyPlatformOverrides(tool *registry.Tool) {
	defer perf.Track(nil, "installer.ApplyPlatformOverrides")()

	if len(tool.Overrides) == 0 {
		return
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for i := range tool.Overrides {
		override := &tool.Overrides[i]
		if matchesOverride(override, goos, goarch) {
			applyOverride(tool, override)
			log.Debug("Applied platform override",
				"goos", goos,
				"goarch", goarch,
				"overrideGOOS", override.GOOS,
				"overrideGOARCH", override.GOARCH,
				"newAsset", tool.Asset,
				"newFormat", tool.Format)
			return // First match wins.
		}
	}
}

// matchesOverride checks if an override matches the current platform.
// If the override has an Envs list, it uses isPlatformMatch for each entry.
// Otherwise, falls back to GOOS/GOARCH matching (empty fields are wildcards).
func matchesOverride(override *registry.Override, goos, goarch string) bool {
	if len(override.Envs) > 0 {
		for _, env := range override.Envs {
			if isPlatformMatch(env, goos, goarch) {
				return true
			}
		}
		return false
	}
	return matchesPlatform(override.GOOS, override.GOARCH, goos, goarch)
}

// matchesPlatform checks if an override matches the current platform.
// Empty override fields match any value (wildcard).
func matchesPlatform(overrideGOOS, overrideGOARCH, goos, goarch string) bool {
	goosMatch := overrideGOOS == "" || overrideGOOS == goos
	goarchMatch := overrideGOARCH == "" || overrideGOARCH == goarch
	return goosMatch && goarchMatch
}

// applyOverride applies an override's fields to the tool.
// Only non-empty override fields are applied.
func applyOverride(tool *registry.Tool, override *registry.Override) {
	if override.Asset != "" {
		tool.Asset = override.Asset
	}
	if override.Format != "" {
		tool.Format = override.Format
	}
	if len(override.Files) > 0 {
		tool.Files = override.Files
	}
	if len(override.Replacements) > 0 {
		// Merge override replacements with existing ones (override takes precedence).
		if tool.Replacements == nil {
			tool.Replacements = make(map[string]string)
		}
		for k, v := range override.Replacements {
			tool.Replacements[k] = v
		}
	}
	if override.Checksum.Type != "" || override.Checksum.Asset != "" || override.Checksum.URL != "" || override.Checksum.Enabled != nil {
		tool.Checksum = override.Checksum
	}
	if override.Cosign.Enabled != nil || len(override.Cosign.Opts) > 0 {
		tool.Cosign = override.Cosign
	}
	if override.SLSAProvenance.Enabled != nil || override.SLSAProvenance.Asset != "" || override.SLSAProvenance.URL != "" {
		tool.SLSAProvenance = override.SLSAProvenance
	}
	if override.Minisign.Enabled != nil || override.Minisign.Asset != "" || override.Minisign.URL != "" || override.Minisign.PublicKey != "" {
		tool.Minisign = override.Minisign
	}
	if override.GitHubArtifactAttestations.Enabled != nil || override.GitHubArtifactAttestations.SignerWorkflow != "" {
		tool.GitHubArtifactAttestations = override.GitHubArtifactAttestations
	}
}
