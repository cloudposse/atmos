package output

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Validation describes the validated exports without dumping schema metadata by default.
func Validation(definition *manifest.Config, bindings []extension.Binding, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Validation")()

	rows := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		rows = append(rows, map[string]any{"capability": binding.Capability, "export": binding.Name})
	}
	if outputFormat == string(format.FormatTable) {
		ui.Successf("Validated **%s** version **%s**: manifest and extension schemas are valid.", definition.Metadata.Name, definition.Metadata.Version)
	}
	return render(bindings, rows, []column.Config{{Name: "Capability", Value: "{{ .capability }}"}, {Name: "Export", Value: "{{ .export }}"}}, outputFormat, "")
}

// PackedRelease describes the package written to a local OCI layout.
type PackedRelease struct {
	Name      string                 `json:"name" yaml:"name"`
	Version   string                 `json:"version" yaml:"version"`
	Platforms []string               `json:"platforms" yaml:"platforms"`
	Output    string                 `json:"output" yaml:"output"`
	Files     []pluginoci.BundleFile `json:"files" yaml:"files"`
}

// Packed confirms the artifact location, included platforms, and the files packaged for each.
func Packed(definition *manifest.Config, destination string, files []pluginoci.BundleFile, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Packed")()

	if files == nil {
		files = []pluginoci.BundleFile{}
	}
	result := PackedRelease{Name: definition.Metadata.Name, Version: definition.Metadata.Version, Platforms: slices.Sorted(maps.Keys(definition.Spec.Platforms)), Output: destination, Files: files}
	if outputFormat == string(format.FormatTable) {
		ui.Successf("Packed **%s** version **%s** into `%s`.", result.Name, result.Version, destination)
	}
	rows := []map[string]any{{fieldKey: "Plugin", valueKey: result.Name}, {fieldKey: "Version", valueKey: result.Version}, {fieldKey: "Platforms", valueKey: strings.Join(result.Platforms, ", ")}, {fieldKey: "Layout", valueKey: destination}}
	rows = append(rows, fileRows(files)...)
	return renderDetails(result, rows, outputFormat)
}

// fileRows lists each packaged file with its size and mode under the platform that ships it.
func fileRows(files []pluginoci.BundleFile) []map[string]any {
	rows := make([]map[string]any, 0, len(files))
	for _, file := range files {
		rows = append(rows, map[string]any{fieldKey: "Files (" + file.Platform + ")", valueKey: fmt.Sprintf("%s  %s  %s", file.Path, formatSize(file.Size), file.Mode)})
	}
	return rows
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value, suffix := float64(size), "KiB"
	value /= unit
	for _, next := range []string{"MiB", "GiB"} {
		if value < unit {
			break
		}
		value /= unit
		suffix = next
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}

// PublishedRelease adds the destination and completed publication steps to release metadata.
type PublishedRelease struct {
	*pluginoci.Release `yaml:",inline"`
	Repository         string                 `json:"repository" yaml:"repository"`
	Signed             bool                   `json:"signed" yaml:"signed"`
	Files              []pluginoci.BundleFile `json:"files" yaml:"files"`
}

// Published confirms the completed upload and optional signing.
func Published(result PublishedRelease, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Published")()

	if outputFormat == string(format.FormatTable) {
		ui.Successf("Published **%s** version **%s**.", result.Name, result.Version)
	}
	signing := "Not signed by this command"
	if result.Signed {
		signing = "Signed with Sigstore"
	}
	if result.Files == nil {
		result.Files = []pluginoci.BundleFile{}
	}
	rows := releaseRows(result.Release)
	rows = append(rows, map[string]any{fieldKey: "Repository", valueKey: result.Repository}, map[string]any{fieldKey: "Signing", valueKey: signing})
	rows = append(rows, fileRows(result.Files)...)
	return renderDetails(result, rows, outputFormat)
}

// SignedRelease identifies the release and repository where its signature was written.
type SignedRelease struct {
	*pluginoci.Release `yaml:",inline"`
	Reference          string `json:"reference" yaml:"reference"`
}

// Signed confirms the exact immutable release that received a Sigstore signature.
func Signed(release *pluginoci.Release, reference, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Signed")()

	if outputFormat == string(format.FormatTable) {
		ui.Successf("Signed **%s** version **%s** with Sigstore.", release.Name, release.Version)
	}
	rows := append(releaseRows(release), map[string]any{fieldKey: "Reference", valueKey: reference})
	return renderDetails(SignedRelease{Release: release, Reference: reference}, rows, outputFormat)
}

func releaseRows(release *pluginoci.Release) []map[string]any {
	return []map[string]any{{fieldKey: "Plugin", valueKey: release.Name}, {fieldKey: "Version", valueKey: release.Version}, {fieldKey: "Platforms", valueKey: strings.Join(slices.Sorted(maps.Keys(release.Platforms)), ", ")}, {fieldKey: "Digest", valueKey: release.Digest}}
}

func renderDetails(value any, rows []map[string]any, outputFormat string) error {
	if err := ValidateFormat(outputFormat); err != nil {
		return fmt.Errorf("plugin result: %w", err)
	}
	return render(value, rows, []column.Config{{Name: "Field", Value: "{{ .field }}"}, {Name: "Value", Value: "{{ .value }}"}}, outputFormat, "")
}
