package toolchain

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

// buildAssetURL constructs the asset download URL based on tool configuration.
func (i *Installer) buildAssetURL(tool *registry.Tool, version string) (string, error) {
	defer perf.Track(nil, "Installer.buildAssetURL")()

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

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		tool.RepoOwner, tool.RepoName, data.Version, assetName)

	return url, nil
}

// buildTemplateData creates template data for asset URL building.
func buildTemplateData(tool *registry.Tool, version string) *assetTemplateData {
	defer perf.Track(nil, "buildTemplateData")()

	prefix := tool.VersionPrefix
	if prefix == "" {
		prefix = versionPrefix
	}

	releaseVersion := version
	if !strings.HasPrefix(releaseVersion, prefix) {
		releaseVersion = prefix + releaseVersion
	}

	semVer := strings.TrimPrefix(releaseVersion, prefix)

	return &assetTemplateData{
		Version:   releaseVersion,
		SemVer:    semVer,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		RepoOwner: tool.RepoOwner,
		RepoName:  tool.RepoName,
		Format:    tool.Format,
	}
}

// assetTemplateFuncs returns the template functions for asset URL templates.
func assetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"trimV": func(s string) string {
			return strings.TrimPrefix(s, versionPrefix)
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
