package toolchain

import (
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// LockOptions configures RunLock.
type LockOptions struct {
	MaxConcurrency int
}

// lockResult is the outcome of attempting to lock a single tool.
type lockResult int

const (
	lockResultLocked lockResult = iota
	lockResultFailed
)

// lockOutcome pairs a tool's lock result with its report message. Workers return this
// instead of printing directly so RunLock can report in a stable order rather than
// interleaving concurrent workers' output.
type lockOutcome struct {
	result  lockResult
	message string
}

// RunLock resolves and downloads every tool configured in .tool-versions (or just the
// named tools, if given) to compute a real checksum and write/update its
// toolchain.lock.yaml entry -- without extracting/installing the binary into the real
// toolchain tree. This lets an already-installed project populate or refresh its lock
// file without needing to reinstall every tool with --reinstall first.
//
// Tools are locked concurrently, bounded by opts.MaxConcurrency (must be >= 1).
func RunLock(toolNames []string, opts LockOptions) error {
	defer perf.Track(nil, "toolchain.Lock")()

	if opts.MaxConcurrency < 1 {
		return fmt.Errorf("%w: max concurrency must be at least 1", errUtils.ErrInvalidFlagValue)
	}

	if config := GetAtmosConfig(); config != nil && !config.Toolchain.UseLockFile {
		ui.Warning("toolchain.use_lock_file is not set to true in atmos.yaml -- writing toolchain.lock.yaml, but `atmos toolchain install` won't verify against it until use_lock_file is enabled.")
	}

	filePath := GetToolVersionsFilePath()
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return fmt.Errorf("%w: failed to load %s: %w", errUtils.ErrToolVersionsFileOperation, filePath, err)
	}

	targets, err := resolveLockTargets(toolVersions, toolNames)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		ui.Writef("No tools found in %s\n", filePath)
		return nil
	}

	ui.Infof("Locking %d tool(s)...", len(targets))

	// runConcurrentBatchWithLiveProgress live-renders a spinner per in-flight tool plus an
	// overall N/M progress bar, exactly like `atmos toolchain install`'s batch mode -- each
	// tool's line prints via the matching ui.* style as soon as it completes, instead of every
	// result being silently buffered and dumped at once after the whole batch finishes.
	outcomes := runConcurrentBatchWithLiveProgress(
		targets,
		opts.MaxConcurrency,
		func(tool toolInfo) string { return fmt.Sprintf("%s/%s@%s", tool.owner, tool.repo, tool.version) },
		lockOneTool,
		func(outcome lockOutcome) (string, batchLineStyle) {
			if outcome.result == lockResultFailed {
				return outcome.message, batchLineError
			}
			return outcome.message, batchLineSuccess
		},
	)
	failed := tallyLockOutcomes(outcomes)
	if failed > 0 {
		return fmt.Errorf("%w: %d tool lock(s) failed", errUtils.ErrToolInstall, failed)
	}
	return nil
}

// tallyLockOutcomes counts each outcome's result and prints the one-line summary. Individual
// per-tool lines are already printed live as each tool completes by
// runConcurrentBatchWithLiveProgress, so this only tallies -- it doesn't print them again.
func tallyLockOutcomes(outcomes []lockOutcome) int {
	var locked, failed int
	for _, outcome := range outcomes {
		switch outcome.result {
		case lockResultLocked:
			locked++
		case lockResultFailed:
			failed++
		}
	}
	var segments []string
	if locked > 0 {
		segments = append(segments, fmt.Sprintf("Locked %d tool(s)", locked))
	}
	if failed > 0 {
		segments = append(segments, fmt.Sprintf("%d failed", failed))
	}
	if len(segments) == 0 {
		segments = append(segments, "Locked 0 tool(s)")
	}
	ui.Writef("%s\n", strings.Join(segments, ", "))
	return failed
}

// resolveLockTargets builds the list of tool@version entries to lock. An empty toolNames
// locks every version configured for every tool in .tool-versions, matching install's own
// per-version granularity (buildToolList expands multi-version lines into one entry per
// version) -- sorted for deterministic reporting, since buildToolList iterates a map.
// Explicit toolNames each resolve to just their default (first) version.
func resolveLockTargets(toolVersions *ToolVersions, toolNames []string) ([]toolInfo, error) {
	installer := NewInstaller()

	if len(toolNames) == 0 {
		targets := buildToolList(installer, toolVersions)
		sort.Slice(targets, func(i, j int) bool {
			if targets[i].owner != targets[j].owner {
				return targets[i].owner < targets[j].owner
			}
			if targets[i].repo != targets[j].repo {
				return targets[i].repo < targets[j].repo
			}
			return targets[i].version < targets[j].version
		})
		return targets, nil
	}

	targets := make([]toolInfo, 0, len(toolNames))
	for _, name := range toolNames {
		resolvedKey, version, found := LookupToolVersion(name, toolVersions, installer.GetResolver())
		if !found {
			return nil, fmt.Errorf("%w: tool '%s' not configured in .tool-versions", ErrToolNotFound, name)
		}
		owner, repo, err := installer.ParseToolSpec(resolvedKey)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to resolve tool '%s': %w", errUtils.ErrInvalidToolSpec, name, err)
		}
		targets = append(targets, toolInfo{version, owner, repo})
	}
	return targets, nil
}

// lockOneTool resolves tool's version (handling "latest"), fetches its registry metadata,
// and downloads+verifies the artifact to write its toolchain.lock.yaml entry. Called
// concurrently by runConcurrentBatchWithLiveProgress's workers, so it must not print directly.
func lockOneTool(tool toolInfo) lockOutcome {
	label := fmt.Sprintf("%s/%s", tool.owner, tool.repo)

	installer := NewInstaller(WithForceLockFile())

	spinner := &spinnerControl{showingSpinner: false}
	resolvedVersion, err := resolveLatestVersionWithSpinner(tool.owner, tool.repo, tool.version, tool.version == "latest", spinner)
	if err != nil {
		return lockOutcome{result: lockResultFailed, message: fmt.Sprintf("%s@%s: failed to resolve version: %v", label, tool.version, err)}
	}

	registryTool, err := installer.FindTool(tool.owner, tool.repo, resolvedVersion)
	if err != nil {
		return lockOutcome{result: lockResultFailed, message: fmt.Sprintf("%s@%s: %v", label, resolvedVersion, err)}
	}

	if err := installer.LockTool(registryTool, resolvedVersion); err != nil {
		return lockOutcome{result: lockResultFailed, message: fmt.Sprintf("%s@%s: %v", label, resolvedVersion, err)}
	}

	return lockOutcome{result: lockResultLocked, message: fmt.Sprintf("%s@%s locked", label, resolvedVersion)}
}
