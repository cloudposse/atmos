package installer

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// assetTemplateData holds the data passed to asset URL templates.
type assetTemplateData struct {
	Version         string
	SemVer          string
	OS              string // OS after replacements.
	Arch            string // Arch after replacements.
	GOOS            string // Raw runtime.GOOS before replacements.
	GOARCH          string // Raw runtime.GOARCH before replacements.
	RepoOwner       string
	RepoName        string
	Format          string
	Asset           string // Rendered asset name (populated in second pass).
	AssetWithoutExt string // Asset with file extension removed (populated in second pass).
}

// BuildAssetURL constructs the asset download URL based on tool configuration.
func (i *Installer) BuildAssetURL(tool *registry.Tool, version string) (string, error) {
	defer perf.Track(nil, "Installer.BuildAssetURL")()

	switch tool.Type {
	case "http":
		return i.buildHTTPAssetURL(tool, version)
	case "github_release":
		return i.buildGitHubReleaseURL(tool, version)
	default:
		return "", fmt.Errorf("%w: unsupported tool type: %s", ErrInvalidToolSpec, tool.Type)
	}
}

// buildHTTPAssetURL builds an asset URL for HTTP type tools.
func (i *Installer) buildHTTPAssetURL(tool *registry.Tool, version string) (string, error) {
	defer perf.Track(nil, "Installer.buildHTTPAssetURL")()

	if tool.Asset == "" {
		return "", fmt.Errorf("%w: Asset URL template is required for HTTP type tools", ErrInvalidToolSpec)
	}

	data := buildTemplateData(tool, version)
	url, err := executeAssetTemplate(tool.Asset, tool, data)
	if err != nil {
		return "", err
	}

	// On Windows, add .exe to raw binary URLs that don't have any extension.
	// This follows Aqua's behavior where Windows binaries need .exe extension in the download URL.
	// Only apply when: not an archive AND has no extension (avoids .msi.exe, etc.).
	// See: https://aquaproj.github.io/docs/reference/windows-support/.
	if !hasArchiveExtension(url) && filepath.Ext(url) == "" {
		url = EnsureWindowsExeExtension(url)
	}

	return url, nil
}

// buildGitHubReleaseURL builds an asset URL for GitHub release type tools.
func (i *Installer) buildGitHubReleaseURL(tool *registry.Tool, version string) (string, error) {
	defer perf.Track(nil, "Installer.buildGitHubReleaseURL")()

	if tool.RepoOwner == "" || tool.RepoName == "" {
		return "", fmt.Errorf("%w: RepoOwner and RepoName must be set for github_release type (got RepoOwner=%q, RepoName=%q)",
			ErrInvalidToolSpec, tool.RepoOwner, tool.RepoName)
	}

	assetTemplate := tool.Asset
	if assetTemplate == "" {
		assetTemplate = "{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
	}

	data := buildTemplateData(tool, version)
	assetName, err := executeAssetTemplate(assetTemplate, tool, data)
	if err != nil {
		return "", err
	}

	// On Windows, add .exe to raw binary asset names that don't have any extension.
	// This follows Aqua's behavior where Windows binaries need .exe extension in the download URL.
	// Only apply when: not an archive AND has no extension (avoids .msi.exe, etc.).
	// See: https://aquaproj.github.io/docs/reference/windows-support/.
	if !hasArchiveExtension(assetName) && filepath.Ext(assetName) == "" {
		assetName = EnsureWindowsExeExtension(assetName)
	}

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		tool.RepoOwner, tool.RepoName, data.Version, assetName)

	return url, nil
}

// archiveExtensions contains known archive file extensions.
var archiveExtensions = []string{
	".tar.gz", ".tgz", ".zip", ".gz",
	".tar.xz", ".txz", ".tar.bz2", ".tbz", ".tbz2",
	".bz2", ".xz", ".7z", ".tar", ".pkg",
}

// hasArchiveExtension checks if the asset name has a known archive extension.
func hasArchiveExtension(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// buildTemplateData creates template data for asset URL building.
// Following Aqua behavior: version_prefix defaults to empty, not "v".
// Templates use {{trimV .Version}} or {{.SemVer}} when they need the version without prefix.
func buildTemplateData(tool *registry.Tool, version string) *assetTemplateData {
	defer perf.Track(nil, "buildTemplateData")()

	// Use version_prefix only if explicitly set in registry definition.
	// This matches Aqua's behavior where version_prefix defaults to empty.
	prefix := tool.VersionPrefix

	releaseVersion := version
	if prefix != "" && !strings.HasPrefix(releaseVersion, prefix) {
		releaseVersion = prefix + releaseVersion
	}

	// SemVer is the version without any prefix.
	semVer := version
	if prefix != "" {
		semVer = strings.TrimPrefix(releaseVersion, prefix)
	}

	// Get OS and Arch, applying emulation fallbacks and replacements.
	osVal := runtime.GOOS
	archVal := runtime.GOARCH

	// Rosetta 2 fallback: on darwin/arm64, always use amd64 when rosetta2 is enabled.
	// This matches upstream aquaproj/aqua behavior (no arm64 replacement check).
	if tool.Rosetta2 && osVal == "darwin" && archVal == "arm64" {
		archVal = "amd64"
	}

	// Windows ARM emulation fallback: always use amd64 when enabled on windows/arm64.
	if tool.WindowsArmEmulation && osVal == "windows" && archVal == "arm64" {
		archVal = "amd64"
	}

	// Apply replacements from the tool config.
	if tool.Replacements != nil {
		if replacement, ok := tool.Replacements[osVal]; ok {
			osVal = replacement
		}
		if replacement, ok := tool.Replacements[archVal]; ok {
			archVal = replacement
		}
	}

	// Apply per-OS format overrides (e.g., zip on Windows, tar.gz on Linux).
	format := tool.Format
	for _, fo := range tool.FormatOverrides {
		if fo.GOOS == runtime.GOOS {
			format = fo.Format
			break
		}
	}

	return &assetTemplateData{
		Version:   releaseVersion,
		SemVer:    semVer,
		OS:        osVal,
		Arch:      archVal,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		RepoOwner: tool.RepoOwner,
		RepoName:  tool.RepoName,
		Format:    format,
	}
}

// assetTemplateFuncs returns the template functions for asset URL templates.
// Uses Sprig v3 hermetic text functions as the base (matching Aqua upstream), with Aqua-specific overrides.
// HermeticTxtFuncMap is used instead of TxtFuncMap to exclude env/expandenv, preventing
// asset URL templates from reading arbitrary process environment variables (CWE-526).
func assetTemplateFuncs() template.FuncMap {
	funcs := sprig.HermeticTxtFuncMap()

	// Override with Aqua-specific functions that have different argument order
	// or behavior than Sprig equivalents.
	funcs["trimV"] = func(s string) string {
		return strings.TrimPrefix(s, VersionPrefix)
	}
	funcs["trimPrefix"] = func(pfx, s string) string {
		return strings.TrimPrefix(s, pfx)
	}
	funcs["trimSuffix"] = func(suffix, s string) string {
		return strings.TrimSuffix(s, suffix)
	}
	funcs["replace"] = func(old, new, s string) string {
		return strings.ReplaceAll(s, old, new)
	}

	return funcs
}

// executeAssetTemplate executes an asset URL template with two-pass rendering.
// Pass 1: Render with base variables.
// Pass 2: If template references {{.Asset}} or {{.AssetWithoutExt}}, inject those and re-render.
func executeAssetTemplate(templateStr string, tool *registry.Tool, data *assetTemplateData) (string, error) {
	defer perf.Track(nil, "executeAssetTemplate")()

	// Pass 1: Render with base data.
	result, err := renderAssetTemplate(templateStr, tool, data)
	if err != nil {
		return "", err
	}

	// Pass 2: If template references Asset or AssetWithoutExt, inject and re-render.
	if strings.Contains(templateStr, ".Asset") {
		data.Asset = result
		data.AssetWithoutExt = stripFileExtension(result)
		result, err = renderAssetTemplate(templateStr, tool, data)
		if err != nil {
			return "", err
		}
	}

	return result, nil
}

// renderAssetTemplate parses and executes an asset URL template once.
func renderAssetTemplate(templateStr string, tool *registry.Tool, data *assetTemplateData) (string, error) {
	tmpl, err := template.New("asset").Funcs(assetTemplateFuncs()).Parse(templateStr)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrAssetTemplateInvalid).
			WithExplanationf("Asset template for `%s/%s` contains invalid Go template syntax", tool.RepoOwner, tool.RepoName).
			WithExample("# Valid asset template:\nasset: \"https://releases.example.com/{{.RepoName}}/v{{.Version}}/{{.RepoName}}_{{.OS}}_{{.Arch}}.tar.gz\"").
			WithHint("Check the tool definition in the registry for syntax errors").
			WithHint("Verify template variables: {{.Version}}, {{.SemVer}}, {{.OS}}, {{.Arch}}, {{.RepoOwner}}, {{.RepoName}}").
			WithContext("tool", fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)).
			WithContext("template", templateStr).
			WithContext("parse_error", err.Error()).
			WithExitCode(2).
			Err()
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, data); err != nil {
		return "", errUtils.Build(errUtils.ErrAssetTemplateInvalid).
			WithExplanationf("Asset template for `%s/%s` failed to execute", tool.RepoOwner, tool.RepoName).
			WithHint("Template executed but produced invalid output").
			WithHint("Check template logic and variable availability").
			WithContext("tool", fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName)).
			WithContext("template", templateStr).
			WithContext("execution_error", err.Error()).
			WithExitCode(2).
			Err()
	}

	return result.String(), nil
}

// stripFileExtension removes the file extension from an asset name.
// Handles compound extensions like .tar.gz, .tar.xz, .tar.bz2.
func stripFileExtension(name string) string {
	compoundExts := []string{".tar.gz", ".tar.xz", ".tar.bz2"}
	lower := strings.ToLower(name)
	for _, ext := range compoundExts {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	ext := filepath.Ext(name)
	if ext != "" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}
