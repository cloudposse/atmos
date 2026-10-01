package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	tmpl "github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// SetUpdateStrategy selects where a 3-way merge's base content comes from
// during --update (engine.UpdateStrategyTracked, the default: the target's
// own git history; engine.UpdateStrategyRendered: a pristine template
// re-render, see SetRenderedBaseSource).
func (ui *InitUI) SetUpdateStrategy(strategy engine.UpdateStrategy) {
	ui.updateStrategy = strategy
}

// SetRenderedBaseSource supplies the pristine "old ref" template
// configuration and its originally-recorded answers that
// engine.UpdateStrategyRendered re-renders as the merge base. Both must come
// from the same generation (see pkg/project/config's project-record
// Spec.Values/Spec.BaseRef) -- rendering the old config's own scaffold.yaml
// against these old values keeps base self-consistent, unlike applying this
// run's new answers to the old schema.
func (ui *InitUI) SetRenderedBaseSource(cfg *tmpl.Configuration, values map[string]interface{}) {
	ui.renderedBaseConfig = cfg
	ui.renderedBaseValues = values
}

// loadOldScaffoldConfig finds and loads oldConfig's own scaffold.yaml, so
// renderPristineBase can render it against the old ref's own schema rather
// than the current run's.
func loadOldScaffoldConfig(oldConfig *tmpl.Configuration) (*config.ScaffoldConfig, error) {
	var oldScaffoldConfigFile *tmpl.File
	for i := range oldConfig.Files {
		if oldConfig.Files[i].Path == config.ScaffoldConfigFileName {
			oldScaffoldConfigFile = &oldConfig.Files[i]
			break
		}
	}
	if oldScaffoldConfigFile == nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldConfigMissing).
			WithExplanationf("%s not found in the old ref's rendered configuration", config.ScaffoldConfigFileName).
			WithHint("--update-strategy=rendered requires the template to carry a scaffold.yaml at every ref it's updated across").
			Err()
	}

	oldScaffoldConfig, err := config.LoadScaffoldConfigFromContent(oldScaffoldConfigFile.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to load the old ref's scaffold configuration: %w", err)
	}
	return oldScaffoldConfig, nil
}

// renderPristineBase renders oldConfig -- a template Configuration fetched at
// the ref that produced what's currently on disk -- into a fresh temp
// directory using oldValues (that generation's own recorded answers), with
// no hooks, no UI output, and no project-record write. It exists purely to
// give engine.UpdateStrategyRendered's 3-way merge a base to read from (see
// engine.Processor.SetupRenderedBaseStorage); the caller is responsible for
// invoking the returned cleanup once the merge that consumes it is done.
//
// It reuses ui.processFileEntry -- the same per-file loop executeWithSetup
// uses for a real generation, including matrix expansion -- with
// force=true, update=false so every file is a plain overwrite into an
// otherwise-empty directory, never touching merge/hooks itself.
func (ui *InitUI) renderPristineBase(oldConfig *tmpl.Configuration, oldValues map[string]interface{}, delimiters []string) (tempDir string, cleanup func(), err error) {
	oldScaffoldConfig, err := loadOldScaffoldConfig(oldConfig)
	if err != nil {
		return "", nil, err
	}
	mergedOldValues := config.DeepMerge(oldScaffoldConfig, oldValues)

	tempDir, err = os.MkdirTemp("", "atmos-rendered-base-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create a temp directory for the rendered base: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tempDir) }

	// Discard UI output produced by the reused per-file loop below. This
	// render is a purely internal step; its progress lines must never
	// interleave with the real run's own output buffer. strings.Builder must
	// never be copied by value once used (a copy's internal address check
	// panics on the next write), so save/restore its string content rather
	// than the Builder itself.
	savedOutput := ui.output.String()
	ui.output.Reset()
	defer func() {
		ui.output.Reset()
		_, _ = ui.output.WriteString(savedOutput)
	}()

	// This internal render must always actually write oldConfig's files to
	// tempDir, even when the outer run is a --dry-run preview:
	// engine.Processor.ProcessFile skips the actual disk write whenever
	// Processor.DryRun is set, and this render shares ui.processor with the
	// real run (see ui.processFileEntry -> ui.writeOneOutput ->
	// ui.processor.ProcessFile). Left as-is, SetupRenderedBaseStorage below
	// would be pointed at an empty tempDir and a --dry-run preview under
	// --update-strategy=rendered would have no base to diff against. Save
	// and restore rather than leaving it disabled, since the real run
	// continuing after this function returns still needs its own DryRun
	// behavior intact.
	dryRun := ui.processor.DryRun
	ui.processor.SetDryRun(false)
	defer ui.processor.SetDryRun(dryRun)

	if err := ui.renderPristineBaseFiles(oldConfig, oldScaffoldConfig, mergedOldValues, tempDir, delimiters); err != nil {
		cleanup()
		return "", nil, err
	}

	return tempDir, cleanup, nil
}

// renderPristineBaseFiles loops oldConfig's files (skipping scaffold.yaml
// and directory entries) and renders each into tempDir via
// ui.processFileEntry, joining any per-file failures into a single error.
func (ui *InitUI) renderPristineBaseFiles(oldConfig *tmpl.Configuration, oldScaffoldConfig *config.ScaffoldConfig, mergedOldValues map[string]interface{}, tempDir string, delimiters []string) error {
	activeDelimiters := ResolveDelimiters(delimiters, oldScaffoldConfig)
	fileSpecs := FileSpecByPath(oldScaffoldConfig, oldConfig.Files)
	seenRenderedPaths := make(map[string]string)
	matrixExpansions := make(map[string]matrixExpansionResult)

	var failureErrs []error
	for _, file := range oldConfig.Files {
		if file.Path == config.ScaffoldConfigFileName || file.IsDirectory {
			continue
		}

		spec := fileSpecs[file.Path]
		_, _, _, entryErr := ui.processFileEntry(file, spec, tempDir, true, false, oldScaffoldConfig, mergedOldValues, activeDelimiters, seenRenderedPaths, matrixExpansions)
		if entryErr != nil {
			failureErrs = append(failureErrs, entryErr)
		}
	}

	if len(failureErrs) > 0 {
		return errUtils.Build(errUtils.ErrScaffoldGeneration).
			WithCause(errors.Join(failureErrs...)).
			WithExplanation("Failed to render the old ref's template for the rendered update-strategy base").
			Err()
	}
	return nil
}

// templateDeletionResult bundles handleTemplateDeletions's counts (grouped
// into a struct, rather than three separate return values alongside err, to
// stay under revive's function-result-limit).
type templateDeletionResult struct {
	successCount int
	errorCount   int
	failedPaths  []string
}

// handleTemplateDeletions deletes a file the template stopped generating
// between the old and new ref. This is only possible for
// engine.UpdateStrategyRendered -- it's the only strategy with a full old-ref
// file tree to diff against (ui.renderedBaseRoot, set by setupUpdateBase from
// renderPristineBase's own full when-filtered render of the old config). By
// contrast, engine.UpdateStrategyTracked has no equivalent: there is no safe
// way to enumerate "every file the template generated at base-ref" without
// conflating unrelated files that merely happen to exist in that historical
// commit, so it's a documented limitation instead (see
// docs/prd/atmos-scaffold.md).
//
// The newPaths argument is executeWithSetup's own seenRenderedPaths map: its
// keys are exactly this run's current output paths (populated only for
// spec.When-true, actually-attempted files -- see checkDuplicateRenderedPath),
// so no separate "new file set" needs computing here. Every file under
// ui.renderedBaseRoot not in newPaths is a deletion candidate, handled by
// processDeletionCandidate. force is forwarded to it unchanged -- see that
// function's own doc comment for what it does here.
func (ui *InitUI) handleTemplateDeletions(targetPath string, newPaths map[string]string, force bool) (templateDeletionResult, error) {
	if ui.renderedBaseRoot == "" {
		return templateDeletionResult{}, nil
	}

	var result templateDeletionResult
	var failureErrs []error
	walkErr := filepath.WalkDir(ui.renderedBaseRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		relPath, relErr := filepath.Rel(ui.renderedBaseRoot, path)
		if relErr != nil {
			return relErr
		}
		relPath = filepath.Clean(relPath)
		if _, stillWanted := newPaths[relPath]; stillWanted {
			return nil
		}

		deleted, candidateErr := ui.processDeletionCandidate(targetPath, relPath, force)
		switch {
		case candidateErr != nil:
			result.errorCount++
			result.failedPaths = append(result.failedPaths, relPath)
			failureErrs = append(failureErrs, candidateErr)
		case deleted:
			result.successCount++
		}
		return nil
	})
	if walkErr != nil {
		return result, fmt.Errorf("failed to scan the rendered base for removed files: %w", walkErr)
	}
	if len(failureErrs) > 0 {
		return result, errors.Join(failureErrs...)
	}
	return result, nil
}

// processDeletionCandidate handles one path the template no longer generates
// (see handleTemplateDeletions): deletes it when the on-disk copy still
// matches the old pristine render exactly, and silently no-ops when the file
// is already gone.
//
// When local edits survive, force decides the outcome: false (default)
// reports an unresolved merge conflict (reusing errUtils.ErrMergeConflict and
// the same "✗ path (error: ...)" shape a real content conflict already uses
// -- see merge_update.go's mergeFile -- rather than a new reporting shape);
// true deletes it anyway. This mirrors --force's existing meaning elsewhere
// in this subsystem ("on conflict, the template's choice wins" -- see
// merge.ResolveConflictStrategy) -- here the template's choice is deletion,
// so --force deletes through the local edits instead of leaving them as an
// unresolved conflict.
func (ui *InitUI) processDeletionCandidate(targetPath, relPath string, force bool) (deleted bool, err error) {
	if !fileExistsAt(targetPath, relPath) {
		return false, nil
	}
	targetFullPath := filepath.Join(targetPath, relPath)

	// Refuse to read through or remove a symlink at the deletion-candidate
	// path, mirroring engine.validateWriteTarget's write-side protection: a
	// symlink here could otherwise make the byte-comparison below read
	// content from outside the target directory entirely.
	if info, lstatErr := os.Lstat(targetFullPath); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
		symlinkErr := errUtils.Build(errUtils.ErrSymlinkWrite).
			WithExplanationf("Refusing to inspect `%s` for deletion: it's a symlink", relPath).
			WithHint("Remove the symlink manually if it's no longer needed, or replace it with a real file").
			WithContext("file_path", relPath).
			WithExitCode(2).
			Err()
		ui.writeOutput(fileStatusFormat,
			ui.errorStyle.Render(ui.xMark),
			relPath,
			ui.grayStyle.Render(fmt.Sprintf("(error: %v)", symlinkErr)))
		return false, symlinkErr
	}

	oldContent, err := os.ReadFile(filepath.Join(ui.renderedBaseRoot, relPath))
	if err != nil {
		return false, fmt.Errorf("failed to read the old rendered base for `%s`: %w", relPath, err)
	}
	currentContent, err := os.ReadFile(targetFullPath)
	if err != nil {
		return false, fmt.Errorf("failed to read `%s`: %w", relPath, err)
	}

	locallyModified := !bytes.Equal(oldContent, currentContent)
	if locallyModified && !force {
		conflictErr := errUtils.Build(errUtils.ErrMergeConflict).
			WithExplanationf("`%s` was removed from the template but has local modifications", relPath).
			WithHint("Resolve manually: delete the file if it's no longer needed, or keep it -- future updates won't touch it again since the template no longer generates it").
			WithHint("Or re-run with `--force` to delete it anyway").
			WithContext("file_path", relPath).
			WithExitCode(1).
			Err()
		ui.writeOutput(fileStatusFormat,
			ui.errorStyle.Render(ui.xMark),
			relPath,
			ui.grayStyle.Render(fmt.Sprintf("(error: %v)", conflictErr)))
		return false, conflictErr
	}

	status := deletedStatus
	switch {
	case locallyModified && ui.processor.DryRun:
		status = dryRunForcedDeleteStatus
	case locallyModified:
		status = forcedDeletedStatus
	case ui.processor.DryRun:
		status = dryRunDeleteStatus
	}
	if !ui.processor.DryRun {
		if removeErr := os.Remove(targetFullPath); removeErr != nil {
			return false, fmt.Errorf("failed to delete `%s`: %w", relPath, removeErr)
		}
	}
	ui.writeOutput(fileStatusFormat,
		ui.successStyle.Render(ui.checkmark),
		relPath,
		ui.grayStyle.Render(status))
	return true, nil
}
