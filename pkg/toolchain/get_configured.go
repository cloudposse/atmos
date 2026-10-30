package toolchain

import (
	"fmt"
	"slices"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/ui"
)

// listConfiguredToolVersions lists the file's tools in deterministic name order.
// Aggregate output retains configured names; named-tool output remains canonical.
func listConfiguredToolVersions(showAll bool, limit int, format string) error {
	filePath := GetToolVersionsFilePath()
	configured, err := LoadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}
	if len(configured.Tools) == 0 {
		return fmt.Errorf("%w: no tools configured in %s", ErrNoVersionsFound, filePath)
	}
	names := make([]string, 0, len(configured.Tools))
	for name := range configured.Tools {
		names = append(names, name)
	}
	slices.Sort(names)
	installer := NewInstaller()
	lists := make([]ToolVersionList, 0, len(names))
	for _, name := range names {
		list, err := configuredToolVersionList(installer, name, filePath, showAll, limit)
		if err != nil {
			return err
		}
		lists = append(lists, list)
	}
	return printConfiguredToolVersions(lists, showAll, format)
}

func configuredToolVersionList(installer *Installer, name, filePath string, showAll bool, limit int) (ToolVersionList, error) {
	owner, repo, err := installer.ParseToolSpec(name)
	if err != nil {
		return ToolVersionList{}, fmt.Errorf("invalid tool name %q: %w", name, err)
	}
	// Each configured entry owns its versions, even when another entry resolves to the same tool.
	versions, defaultVersion, err := getVersions(&versionOptions{showAll, limit, owner, repo, name, name, filePath})
	if err != nil {
		return ToolVersionList{}, err
	}
	versions = dedupeAndSort(versions)
	installed := markInstalled(installer, owner, repo, versions)
	entries := make([]ToolVersionListEntry, 0, len(versions))
	for _, version := range versions {
		entries = append(entries, ToolVersionListEntry{Version: version, Installed: installed[version], Default: version == defaultVersion})
	}
	return ToolVersionList{Tool: name, DefaultVersion: defaultVersion, Versions: entries}, nil
}

func printConfiguredToolVersions(lists []ToolVersionList, showAll bool, format string) error {
	if format == "json" && showAll {
		return data.WriteJSON(lists)
	}
	defaults := configuredDefaultVersions(lists)
	if format == "json" {
		return data.WriteJSON(defaults)
	}
	for i, list := range lists {
		if format == "plain" {
			if err := data.Writeln(list.Tool + " " + defaults[i].Version); err != nil {
				return err
			}
			continue
		}
		ui.Writef("%s:\n", list.Tool)
		versions := make([]string, 0, len(list.Versions))
		installed := make(map[string]bool, len(list.Versions))
		for _, entry := range list.Versions {
			versions = append(versions, entry.Version)
			installed[entry.Version] = entry.Installed
		}
		installedStyle, notInstalledStyle := selectStyles()
		for _, version := range versions {
			printVersionsTable([]string{version}, list.DefaultVersion, installed, &installedStyle, &notInstalledStyle)
			ui.Writeln("")
		}
	}
	return nil
}

func configuredDefaultVersions(lists []ToolVersionList) []ToolVersionInfo {
	defaults := make([]ToolVersionInfo, 0, len(lists))
	for _, list := range lists {
		item := ToolVersionInfo{Tool: list.Tool, Version: list.DefaultVersion}
		for _, version := range list.Versions {
			if version.Default {
				item.Installed = version.Installed
			}
		}
		defaults = append(defaults, item)
	}
	return defaults
}
