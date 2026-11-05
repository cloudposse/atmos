package skill

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/skills/source"
	"github.com/cloudposse/atmos/pkg/ui"
)

// appendSourceListEntries keeps catalog browsing available when source state needs repair.
func appendSourceListEntries(ctx context.Context, entries []listEntry, engine *source.Engine) []listEntry {
	statuses, err := engine.Status(ctx, source.Options{})
	if err != nil {
		ui.Warningf("Declared skill sources unavailable: %v", err)
	}
	return mergeSourceListEntries(entries, statuses)
}

// mergeSourceListEntries shows one canonical row per installation, retaining client problems.
func mergeSourceListEntries(entries []listEntry, statuses []source.Status) []listEntry {
	for _, status := range statuses {
		if status.Client != "" && status.Client != "atmos" {
			continue
		}
		installed := sourceListInstalled(&status, statuses)
		status.Status = sourceListState(&status, statuses)
		if status.Name == "" {
			status.Name = status.Source
		}
		entry := listEntry{name: status.Name, displayName: status.Name}
		index := catalogEntryIndex(entries, &status)
		if index >= 0 {
			entry = entries[index]
		}
		entry.source, entry.displaySource = status.Source, status.Source
		entry.installed, entry.sourceStatus = installed, &status
		// Catalog version comparisons do not describe source-managed installations.
		entry.version, entry.updateAvailable, entry.skill = "", false, nil
		if index >= 0 {
			entries[index] = entry
		} else {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries
}

// sourceListInstalled includes surviving client copies even when the canonical copy is missing.
func sourceListInstalled(canonical *source.Status, statuses []source.Status) bool {
	for _, status := range statuses {
		if status.Name == canonical.Name && status.Source == canonical.Source && status.Scope == canonical.Scope &&
			status.Status != "missing" && (status.Status != "stale" || status.Path != "") {
			return true
		}
	}
	return false
}

// sourceListState preserves canonical health and summarizes unhealthy client destinations.
func sourceListState(canonical *source.Status, statuses []source.Status) string {
	problems := []string{}
	for _, status := range statuses {
		if status.Name == canonical.Name && status.Source == canonical.Source && status.Scope == canonical.Scope &&
			status.Client != "" && status.Client != "atmos" && status.Status != "current" {
			problems = append(problems, fmt.Sprintf("%s: %s", status.Client, status.Status))
		}
	}
	if len(problems) == 0 {
		return canonical.Status
	}
	sort.Strings(problems)
	return canonical.Status + "; " + strings.Join(problems, "; ")
}

// catalogEntryIndex merges catalog metadata but preserves installations at different paths.
func catalogEntryIndex(entries []listEntry, status *source.Status) int {
	for i := range entries {
		entry := &entries[i]
		if entry.name != status.Name || entry.sourceStatus != nil {
			continue
		}
		if !entry.installed || (entry.skill != nil && entry.skill.Path != "" && status.Path != "" &&
			filepath.Clean(entry.skill.Path) == filepath.Clean(status.Path)) {
			return i
		}
	}
	return -1
}
