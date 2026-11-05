// Package output presents plugin discovery and installation data using the shared list UI.
package output

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/discovery"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	fieldKey   = "field"
	valueKey   = "value"
	emptyValue = "-"
)

// SearchResult associates discovery metadata with its configured registry alias.
type SearchResult = discovery.Result

// ValidateFormat checks the formats supported by plugin lists before accessing packages.
//
//nolint:lintroller // Pure validation with no I/O.
func ValidateFormat(value string) error {
	return ValidateFormatAmong(value, string(format.FormatTable), string(format.FormatJSON), string(format.FormatYAML), string(format.FormatCSV), string(format.FormatTSV))
}

// ValidateFormatAmong checks a format against the subset a command supports. Commands that
// support a narrower or different set than plugin lists, such as `plugin test`, share the
// same error type, message shape, and hint as ValidateFormat.
//
//nolint:lintroller // Pure validation with no I/O.
func ValidateFormatAmong(value string, supported ...string) error {
	if slices.Contains(supported, value) {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: unsupported format %q (supported: %s)", errUtils.ErrInvalidFlagValue, value, strings.Join(supported, ", "))).
		WithHintf("Use `--format=%s`.", supported[0]).Err()
}

// Search displays catalog entries without fetching or executing their plugin packages.
func Search(results []SearchResult, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Search")()

	if results == nil {
		results = []SearchResult{}
	}
	rows := make([]map[string]any, 0, len(results))
	for index := range results {
		result := &results[index]
		rows = append(rows, map[string]any{
			"plugin": result.Name, "capabilities": strings.Join(result.Capabilities, ", "),
			"registry": result.Registry, "description": result.Description,
		})
	}
	return render(results, rows, []column.Config{
		{Name: "Plugin", Value: "{{ .plugin }}"},
		{Name: "Capabilities", Value: "{{ .capabilities }}"},
		{Name: "Registry", Value: "{{ .registry }}"},
		{Name: "Description", Value: "{{ .description }}"},
	}, outputFormat, "No plugins found matching the search filters.")
}

// List displays selected plugin versions and their local installation state.
func List(statuses []manage.Status, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.List")()

	if statuses == nil {
		statuses = []manage.Status{}
	}
	interactive := isInteractiveTable(outputFormat)
	rows := make([]map[string]any, 0, len(statuses))
	for index := range statuses {
		status := &statuses[index]
		name, version := "-", "-"
		// A path package replaces a locked release with different code. Label it with the plugin and
		// version of its local manifest, never with the inactive release lock.
		switch {
		case status.Source == "path":
			name, version = emptyAsDash(status.Plugin), emptyAsDash(status.Version)
		case status.Release != nil:
			name, version = status.Release.Plugin, status.Release.Version
		}
		row := map[string]any{
			"alias": status.Alias, "plugin": name, "version": version, "status": status.State,
		}
		if interactive {
			row["indicator"] = statusIndicator(status.State)
		}
		rows = append(rows, row)
	}
	columns := []column.Config{
		{Name: "Alias", Value: "{{ .alias }}"},
		{Name: "Plugin", Value: "{{ .plugin }}"},
		{Name: "Version", Value: "{{ .version }}"},
		{Name: "Status", Value: "{{ .status }}"},
	}
	if interactive {
		// A space gives the indicator the blank header used by auth lists.
		columns = append([]column.Config{{Name: " ", Value: "{{ .indicator }}"}}, columns...)
	}
	if err := render(statuses, rows, columns, outputFormat, "No plugins configured."); err != nil {
		return err
	}
	reportHints(statuses)
	return nil
}

func emptyAsDash(value string) string {
	if value == "" {
		return emptyValue
	}
	return value
}

// Info displays a selected plugin's static metadata and local installation details.
func Info(status *manage.Status, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Info")()

	if status == nil {
		return errUtils.ErrPluginNotInstalled
	}
	rows := []map[string]any{
		{fieldKey: "Alias", valueKey: status.Alias},
		{fieldKey: "Status", valueKey: status.State},
		{fieldKey: "Path", valueKey: status.Path},
	}
	if status.Source == "path" {
		rows = append(rows, map[string]any{fieldKey: "Origin", valueKey: "plugins.packages." + status.Alias + ".path (local build; lock, signature and version checks are bypassed)"})
	}
	if status.Hint != "" {
		rows = append(rows, map[string]any{fieldKey: "Hint", valueKey: status.Hint})
	}
	if status.Manifest != nil {
		rows = append(
			rows,
			map[string]any{fieldKey: "Plugin", valueKey: status.Manifest.Metadata.Name},
			map[string]any{fieldKey: "Version", valueKey: status.Manifest.Metadata.Version},
			map[string]any{fieldKey: "Description", valueKey: status.Manifest.Metadata.Description},
		)
	}
	if status.Release != nil {
		release := status.Release
		rows = append(
			rows,
			map[string]any{fieldKey: "Locked plugin", valueKey: release.Plugin},
			map[string]any{fieldKey: "Locked version", valueKey: release.Version},
			map[string]any{fieldKey: "Source", valueKey: release.Source},
			map[string]any{fieldKey: "Constraint", valueKey: release.Constraint},
			map[string]any{fieldKey: "Index digest", valueKey: release.IndexDigest},
			map[string]any{fieldKey: "Signature", valueKey: release.Verification.Signature},
		)
	}
	columns := []column.Config{
		{Name: "Field", Value: "{{ .field }}"},
		{Name: "Value", Value: "{{ .value }}"},
	}
	if isInteractiveTable(outputFormat) {
		for _, row := range rows {
			row["indicator"] = ""
			if row[fieldKey] == "Status" {
				row["indicator"] = statusIndicator(status.State)
			}
		}
		columns = append([]column.Config{{Name: " ", Value: "{{ .indicator }}"}}, columns...)
	}
	return render(status, rows, columns, outputFormat, "")
}

// Registries displays configured hosts and discovery sources in deterministic alias order.
func Registries(registries map[string]schema.PluginRegistry, outputFormat string) error {
	defer perf.Track(nil, "pluginoutput.Registries")()

	if registries == nil {
		registries = map[string]schema.PluginRegistry{}
	}
	aliases := make([]string, 0, len(registries))
	for alias := range registries {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	rows := make([]map[string]any, 0, len(aliases))
	for _, alias := range aliases {
		identity := registries[alias].Identity
		if identity == "" {
			identity = emptyValue
		}
		registry := registries[alias]
		host, err := pluginconfig.RegistryHost(&registry)
		if err != nil {
			return err
		}
		catalog := registries[alias].Catalog
		if catalog == "" {
			catalog = emptyValue
		}
		repositories := strings.Join(registries[alias].Repositories, "\n")
		if repositories == "" {
			repositories = emptyValue
		}
		rows = append(rows, map[string]any{"name": alias, "host": host, "catalog": catalog, "repositories": repositories, "identity": identity})
	}
	return render(registries, rows, []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Host", Value: "{{ .host }}"},
		{Name: "Catalog", Value: "{{ .catalog }}"},
		{Name: "Repositories", Value: "{{ .repositories }}"},
		{Name: "Identity", Value: "{{ .identity }}"},
	}, outputFormat, "No plugin registries configured.")
}

func render(value any, rows []map[string]any, columns []column.Config, outputFormat, emptyMessage string) error {
	if err := ValidateFormat(outputFormat); err != nil {
		return err
	}
	// Structured output preserves nested metadata and its original field types.
	// The list renderer's column projection is for tabular formats only.
	switch format.Format(outputFormat) {
	case format.FormatJSON:
		return data.WriteJSON(value)
	case format.FormatYAML:
		return data.WriteYAML(value)
	}
	if len(rows) == 0 && outputFormat == string(format.FormatTable) {
		ui.Info(emptyMessage)
		return nil
	}
	selector, err := column.NewSelector(columns, nil)
	if err != nil {
		return err
	}
	return renderer.New(nil, selector, nil, format.Format(outputFormat), "",
		renderer.WithTableOptions(format.TableOptions{SemanticCellStyling: false})).Render(rows)
}
