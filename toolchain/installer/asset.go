package installer

import (
	"fmt"
	"runtime"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// assetTemplateData holds the data passed to asset URL templates.
type assetTemplateData struct {
	Version   string
	SemVer    string
	OS        string
	Arch      string
	RepoOwner string
	RepoName  string
	Format    string
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
	return executeAssetTemplate(tool.Asset, tool, data)
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

	// On Windows, add .exe to raw binary asset names that don't have an archive extension.
	// This follows Aqua's behavior where Windows binaries need .exe extension in the download URL.
	// See: https://aquaproj.github.io/docs/reference/windows-support/
	if !hasArchiveExtension(assetName) {
		assetName = EnsureWindowsExeExtension(assetName)
	}

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		tool.RepoOwner, tool.RepoName, data.Version, assetName)

	return url, nil
}

// hasArchiveExtension checks if the asset name has a known archive extension.
func hasArchiveExtension(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz") ||
		strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".gz") ||
		strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".pkg")
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

	// Get OS and Arch, applying any replacements from the tool config.
	osVal := runtime.GOOS
	archVal := runtime.GOARCH
	if tool.Replacements != nil {
		if replacement, ok := tool.Replacements[osVal]; ok {
			osVal = replacement
		}
		if replacement, ok := tool.Replacements[archVal]; ok {
			archVal = replacement
		}
	}

	return &assetTemplateData{
		Version:   releaseVersion,
		SemVer:    semVer,
		OS:        osVal,
		Arch:      archVal,
		RepoOwner: tool.RepoOwner,
		RepoName:  tool.RepoName,
		Format:    tool.Format,
	}
}

// assetTemplateFuncs returns the template functions for asset URL templates.
func assetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"trimV": func(s string) string {
			return strings.TrimPrefix(s, VersionPrefix)
		},
		"trimPrefix": func(pfx, s string) string {
			return strings.TrimPrefix(s, pfx)
		},
		"trimSuffix": func(suffix, s string) string {
			return strings.TrimSuffix(s, suffix)
		},
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		// Conditional helpers for platform-specific asset patterns.
		"eq": func(a, b string) bool {
			return a == b
		},
		"ne": func(a, b string) bool {
			return a != b
		},
		// ternary returns trueVal if condition is true, otherwise falseVal.
		"ternary": func(condition bool, trueVal, falseVal string) string {
			if condition {
				return trueVal
			}
			return falseVal
		},
	}
}

// executeAssetTemplate executes an asset URL template.
func executeAssetTemplate(templateStr string, tool *registry.Tool, data *assetTemplateData) (string, error) {
	defer perf.Track(nil, "executeAssetTemplate")()

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
