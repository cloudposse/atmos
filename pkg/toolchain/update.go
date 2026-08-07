package toolchain

import (
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// UpdateOptions configures RunUpdate.
type UpdateOptions struct {
	DryRun         bool
	MaxConcurrency int
}

// updateResult is the outcome of attempting to update a single tool.
type updateResult int

const (
	updateResultUpdated updateResult = iota
	updateResultUpToDate
	updateResultSkipped
	updateResultFailed
)

// updateOutcome pairs a tool's result with its report message. Workers return
// this instead of printing directly so RunUpdate can report in the original
// target order rather than interleaving concurrent workers' output.
type updateOutcome struct {
	result  updateResult
	message string
}

// RunUpdate updates the given tools (or every tool in .tool-versions if
// toolNames is empty) to their newest available version, respecting each
// tool's current pin:
//   - "latest" re-resolves to the newest version and reinstalls, via the same
//     path `install --reinstall` already uses.
//   - An exact version is replaced with the newest available version from the
//     registry and reinstalled.
//   - pr:/sha:/ref: pins are skipped with an explanatory message — these are
//     immutable-by-design and updating them silently would defeat their purpose.
//
// Tools are updated concurrently, bounded by opts.MaxConcurrency (must be >=
// 1), mirroring `install`'s batch behavior.
func RunUpdate(toolNames []string, opts UpdateOptions) error {
	defer perf.Track(nil, "toolchain.Update")()

	if opts.MaxConcurrency < 1 {
		return fmt.Errorf("%w: max concurrency must be at least 1", errUtils.ErrInvalidFlagValue)
	}

	filePath := GetToolVersionsFilePath()
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("%w: failed to load .tool-versions: %w", errUtils.ErrToolVersionsFileOperation, err)
	}

	targets, err := resolveUpdateTargets(toolVersions, toolNames)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		ui.Writef("No tools found in %s\n", filePath)
		return nil
	}

	verb := "Checking"
	if !opts.DryRun {
		verb = "Updating"
	}
	ui.Infof("%s %d tool(s)...", verb, len(targets))

	// runConcurrentBatchWithLiveProgress live-renders a spinner per in-flight tool plus an
	// overall N/M progress bar, exactly like `atmos toolchain install`'s batch mode -- each
	// tool's line prints via the matching ui.* style as soon as it completes, instead of every
	// result being silently buffered and dumped at once after the whole batch finishes.
	outcomes := runConcurrentBatchWithLiveProgress(
		targets,
		opts.MaxConcurrency,
		func(tool string) string { return tool },
		func(tool string) updateOutcome { return updateOneTool(tool, opts) },
		renderUpdateOutcome,
	)
	failed := tallyUpdateOutcomes(outcomes, opts.DryRun)
	if failed > 0 {
		return fmt.Errorf("%w: %d tool update(s) failed", errUtils.ErrToolInstall, failed)
	}
	return nil
}

// renderUpdateOutcome maps an update outcome to its live-progress display line and style.
func renderUpdateOutcome(outcome updateOutcome) (string, batchLineStyle) {
	switch outcome.result {
	case updateResultSkipped:
		return outcome.message, batchLineInfo
	case updateResultFailed:
		return outcome.message, batchLineError
	case updateResultUpdated, updateResultUpToDate:
		fallthrough
	default:
		return outcome.message, batchLineSuccess
	}
}

// tallyUpdateOutcomes counts each outcome's result and prints the one-line summary. Individual
// per-tool lines are already printed live as each tool completes by
// runConcurrentBatchWithLiveProgress, so this only tallies -- it doesn't print them again.
func tallyUpdateOutcomes(outcomes []updateOutcome, dryRun bool) int {
	var updated, upToDate, skipped, failed int
	for _, outcome := range outcomes {
		switch outcome.result {
		case updateResultUpdated:
			updated++
		case updateResultUpToDate:
			upToDate++
		case updateResultSkipped:
			skipped++
		case updateResultFailed:
			failed++
		}
	}
	printUpdateSummary(updated, upToDate, skipped, failed, dryRun)
	return failed
}

// resolveUpdateTargets resolves the requested tool names (aliases included) to
// their canonical .tool-versions keys. An empty toolNames means every
// configured tool.
func resolveUpdateTargets(toolVersions *ToolVersions, toolNames []string) ([]string, error) {
	if len(toolNames) == 0 {
		targets := make([]string, 0, len(toolVersions.Tools))
		for tool := range toolVersions.Tools {
			targets = append(targets, tool)
		}
		// Map iteration order is randomized; sort so `atmos toolchain update` with no
		// arguments reports tools in the same order on every run.
		sort.Strings(targets)
		return targets, nil
	}

	installer := NewInstaller()
	targets := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		resolvedKey, _, found := LookupToolVersion(name, toolVersions, installer.GetResolver())
		if !found {
			return nil, fmt.Errorf("%w: tool '%s' not configured in .tool-versions", ErrToolNotFound, name)
		}
		targets = append(targets, resolvedKey)
	}
	return targets, nil
}

// updateOneTool updates a single tool (already resolved to its canonical
// .tool-versions key) and returns its outcome. Called concurrently by
// runUpdatesConcurrently, so it must not print directly.
func updateOneTool(tool string, opts UpdateOptions) updateOutcome {
	filePath := GetToolVersionsFilePath()
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: failed to load .tool-versions: %v", tool, err)}
	}

	versions := toolVersions.Tools[tool]
	if len(versions) == 0 {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: not configured in .tool-versions", tool)}
	}
	current := versions[0]

	if pinDescription, immutable := describeImmutablePin(current); immutable {
		return updateOutcome{result: updateResultSkipped, message: fmt.Sprintf(
			"%s: pinned to %s — not eligible for automatic update, use `add` to change it explicitly", tool, pinDescription,
		)}
	}

	installer := NewInstaller()
	owner, repo, err := installer.ParseToolSpec(tool)
	if err != nil {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: failed to resolve tool: %v", tool, err)}
	}

	if current == "latest" {
		return updateLatestPinnedTool(tool, opts)
	}

	return updateExactPinnedTool(tool, owner, repo, current, opts)
}

// describeImmutablePin returns a human-readable description and true if
// version is a pr:/sha:/ref: pin — these never get bumped automatically.
func describeImmutablePin(version string) (string, bool) {
	if prNum, isPR := IsPRVersion(version); isPR {
		return fmt.Sprintf("pr:%d", prNum), true
	}
	if sha, isSHA := IsSHAVersion(version); isSHA {
		return "sha:" + sha, true
	}
	if ref, isRef := IsRefVersion(version); isRef {
		return "ref:" + ref, true
	}
	return "", false
}

// updateLatestPinnedTool re-resolves a tool pinned to "latest" and reinstalls
// it if a newer version is now available — the same resolution
// install --reinstall already performs.
func updateLatestPinnedTool(tool string, opts UpdateOptions) updateOutcome {
	if opts.DryRun {
		return updateOutcome{result: updateResultUpToDate, message: fmt.Sprintf("%s: latest (dry-run — re-resolves on install)", tool)}
	}
	if err := RunInstall(tool+"@latest", false, true, false, false); err != nil {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: %v", tool, err)}
	}
	return updateOutcome{result: updateResultUpdated, message: fmt.Sprintf("%s: latest (re-resolved)", tool)}
}

// updateExactPinnedTool finds the newest available version for a tool pinned
// to an exact version and, if newer than the current pin, replaces the
// default version and installs it.
func updateExactPinnedTool(tool, owner, repo, current string, opts UpdateOptions) updateOutcome {
	allVersions, err := fetchAllGitHubVersions(owner, repo, defaultVersionLimit)
	if err != nil {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: failed to fetch available versions: %v", tool, err)}
	}
	sorted := dedupeAndSort(allVersions)
	if len(sorted) == 0 {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: no versions found in registry", tool)}
	}
	newest := sorted[len(sorted)-1]

	// pkg/github.GetReleaseVersions strips a leading "v" from every fetched release tag, but
	// the current pin in .tool-versions may still have one (e.g. captured verbatim by an
	// earlier `add`). Comparing raw strings here falsely reported "updated" -- with a real,
	// unnecessary reinstall and .tool-versions rewrite -- for any unchanged "v"-prefixed pin.
	if normalizeVersion(newest) == normalizeVersion(current) {
		return updateOutcome{result: updateResultUpToDate, message: fmt.Sprintf("%s: up to date (%s)", tool, current)}
	}

	if opts.DryRun {
		return updateOutcome{result: updateResultUpdated, message: fmt.Sprintf("%s: %s -> %s (dry-run)", tool, current, newest)}
	}

	// Install the newest version BEFORE writing it to .tool-versions as the new default.
	// If install fails, the previously-configured (and actually-installed) version must
	// remain the default -- otherwise which/exec would report a version the user never
	// asked for as "configured but not installed".
	installer := NewInstaller()
	if err := installSingleToolWithInstaller(installer, owner, repo, newest, InstallOptions{
		ShowProgressBar:    false,
		ShowInstallDetails: false,
		ShowHint:           false,
	}); err != nil {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: failed to install %s: %v", tool, newest, err)}
	}

	filePath := GetToolVersionsFilePath()
	if err := AddToolToVersionsAsDefault(filePath, tool, newest); err != nil {
		return updateOutcome{result: updateResultFailed, message: fmt.Sprintf("%s: failed to update .tool-versions: %v", tool, err)}
	}

	return updateOutcome{result: updateResultUpdated, message: fmt.Sprintf("%s: %s -> %s", tool, current, newest)}
}

// printUpdateSummary prints a one-line summary of an update run.
func printUpdateSummary(updated, upToDate, skipped, failed int, dryRun bool) {
	verb := "Updated"
	if dryRun {
		verb = "Would update"
	}

	var segments []string
	if updated > 0 {
		segments = append(segments, fmt.Sprintf("%s %d tool(s)", verb, updated))
	}
	if upToDate > 0 {
		segments = append(segments, fmt.Sprintf("%d up to date", upToDate))
	}
	if skipped > 0 {
		segments = append(segments, fmt.Sprintf("%d skipped", skipped))
	}
	if failed > 0 {
		segments = append(segments, fmt.Sprintf("%d failed", failed))
	}
	if len(segments) == 0 {
		segments = append(segments, fmt.Sprintf("%s 0 tool(s)", verb))
	}

	ui.Writef("%s\n", strings.Join(segments, ", "))
}
