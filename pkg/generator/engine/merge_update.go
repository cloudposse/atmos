package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// This file contains the update-mode (3-way merge) parts of the Processor:
// git-based merge base storage, merge configuration, and the merge pipeline
// used when regenerating files that already exist on disk.

// SetMaxChanges sets the maximum percentage of changes allowed for 3-way merge operations.
// The thresholdPercent parameter controls how aggressive the merge behavior is:
// a lower value (e.g., 30) is more conservative, while a higher value (e.g., 80)
// allows more extensive changes during merges.
func (p *Processor) SetMaxChanges(thresholdPercent int) {
	defer perf.Track(nil, "engine.Processor.SetMaxChanges")()

	p.merger = merge.NewThreeWayMerger(thresholdPercent)
}

// SetConflictStrategy sets how a real ours/theirs divergence is resolved
// during a 3-way merge. Note: SetMaxChanges replaces p.merger wholesale, so
// call SetConflictStrategy after SetMaxChanges if both are used, or the
// strategy set here would be discarded.
func (p *Processor) SetConflictStrategy(strategy merge.ConflictStrategy) {
	defer perf.Track(nil, "engine.Processor.SetConflictStrategy")()

	p.merger.SetConflictStrategy(strategy)
}

// SetMergeDriver sets which merger handles every file during a 3-way merge.
func (p *Processor) SetMergeDriver(driver merge.Driver) {
	defer perf.Track(nil, "engine.Processor.SetMergeDriver")()

	p.merger.SetDriver(driver)
}

// SetDryRun toggles dry-run mode. When enabled, ProcessFile still renders
// templates, loads the git merge base, performs the 3-way merge, and runs
// conflict/threshold checks — every step that can fail — but skips the final
// write to disk, so a dry-run preview reports real conflicts instead of only
// listing file paths.
func (p *Processor) SetDryRun(dryRun bool) {
	defer perf.Track(nil, "engine.Processor.SetDryRun")()

	p.DryRun = dryRun
}

// SetupGitStorage initializes git-based storage for 3-way merges.
// The targetPath is used to find the git repository and resolve relative file paths.
// The baseRef specifies which git reference to use as the base for merges (e.g., "main", "v1.0.0").
//
// Returns an error if:
//   - targetPath is not in a git repository
//   - baseRef cannot be resolved
func (p *Processor) SetupGitStorage(targetPath string, baseRef string) error {
	defer perf.Track(nil, "engine.Processor.SetupGitStorage")()

	// Validate everything into locals first; only mutate p.targetPath/p.baseStorage
	// once every validation step has succeeded. A failed call must leave the
	// Processor's existing state untouched rather than half-updated.
	repo, err := git.PlainOpenWithOptions(targetPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			// Not in a git repo - this is OK, just means we can't use git-based merging.
			return nil
		}
		return errUtils.Build(errUtils.ErrThreeWayMerge).
			WithCause(err).
			WithExplanationf("Failed to open git repository at: `%s`", targetPath).
			WithHint("Check that the path is accessible and the repository is not corrupted").
			WithContext("target_path", targetPath).
			WithExitCode(2).
			Err()
	}

	// Create git storage with base ref
	gitStorage := storage.NewGitBaseStorage(repo, baseRef)

	// Validate that base ref exists
	if err := gitStorage.ValidateBaseRef(); err != nil {
		return errUtils.Build(errUtils.ErrInvalidBaseRef).
			WithExplanationf("Invalid git base reference: `%s`", baseRef).
			WithHint("Ensure the git reference exists (branch, tag, or commit hash)").
			WithHint("Run `git branch -a` to see available branches").
			WithHint("Run `git tag` to see available tags").
			WithContext("base_ref", baseRef).
			WithContext("target_path", targetPath).
			WithExitCode(2).
			Err()
	}

	p.targetPath = targetPath
	p.baseStorage = gitStorage

	return nil
}

// SetupRenderedBaseStorage points the 3-way merge base at a pristine
// re-render of the template (UpdateStrategyRendered) instead of the target's
// own git history.
//
// Note: oldRenderRoot is the root of an already-fully-rendered copy of the
// template at the ref that produced what's currently on disk (see
// pkg/generator/ui's renderPristineBase) -- unlike SetupGitStorage, there is
// no repository to open or ref to validate here, since the caller already
// did the rendering.
//
// Note: targetPath is still required: determineBaseContent uses it (via
// p.targetPath) to compute each file's base-storage-relative path
// regardless of which base storage backs it.
func (p *Processor) SetupRenderedBaseStorage(targetPath, oldRenderRoot string) {
	defer perf.Track(nil, "engine.Processor.SetupRenderedBaseStorage")()

	p.targetPath = targetPath
	p.baseStorage = storage.NewRenderedBaseStorage(oldRenderRoot)
}

// Merge performs a 3-way merge using the internal merger.
// Parameters:
//   - base: The original template content (before any processing)
//   - ours: The user's current version (what exists on disk)
//   - theirs: The new template content (after processing)
//   - fileName: The file name for merge strategy detection
func (p *Processor) Merge(base, ours, theirs, fileName string) (*merge.MergeResult, error) {
	defer perf.Track(nil, "engine.Processor.Merge")()

	return p.merger.Merge(base, ours, theirs, fileName)
}

// mergeFile attempts a 3-way merge for existing files.
//
//nolint:revive,funlen // function-length: merge logic requires detailed error handling
func (p *Processor) mergeFile(existingPath string, file File, targetPath string) error {
	// Read existing file content (user's version - "ours")
	existingContent, err := os.ReadFile(existingPath)
	if err != nil {
		return errUtils.Build(errUtils.ErrReadFile).
			WithExplanationf("Failed to read existing file: `%s`", existingPath).
			WithHint("Check file permissions").
			WithHint("Verify the file exists").
			WithContext("file_path", existingPath).
			WithExitCode(2).
			Err()
	}

	// A file left with real conflict markers from a previous --update can't
	// be merged again as-is: re-parsing it as "ours" either fails outright
	// (YAMLMerger) or silently garbles the result (TextMerger, which has no
	// syntax requirement on its inputs). Fail fast with a specific message
	// naming the real problem, instead of surfacing whatever opaque failure
	// that produces.
	if merge.HasUnresolvedConflictMarkers(string(existingContent)) {
		return errUtils.Build(errUtils.ErrMergeConflict).
			WithExplanationf("`%s` still has unresolved conflict markers from a previous `--update`", file.Path).
			WithHint("Open the file, resolve the `<<<<<<<`/`=======`/`>>>>>>>` blocks, and remove the markers").
			WithHint("Then re-run `--update`").
			WithHint("Or drop `--update` and use `--force` alone to overwrite the file completely").
			WithContext("file_path", file.Path).
			WithContext("absolute_path", existingPath).
			WithExitCode(1).
			Err()
	}

	// Determine base content for 3-way merge
	baseContent, shouldSkip, err := p.determineBaseContent(file, existingPath)
	if err != nil {
		return err
	}
	if shouldSkip {
		return nil
	}

	// Process new template content to get "theirs" version
	newContent := file.Content
	if file.IsTemplate {
		processedContent, err := p.ProcessTemplateWithDelimiters(newContent, targetPath, nil, nil, []string{defaultLeftDelimiter, defaultRightDelimiter})
		if err != nil {
			return errUtils.Build(errUtils.ErrTemplateExecution).
				WithExplanationf("Failed to process template during merge: `%s`", file.Path).
				WithHint("Check template syntax").
				WithHint("Verify all variables are defined").
				WithContext("file_path", file.Path).
				WithExitCode(1).
				Err()
		}
		newContent = processedContent
	}

	// Perform 3-way merge
	// - base: original version from git (or template if no git)
	// - ours: user's current version (existingContent)
	// - theirs: new template version (newContent after processing)
	result, err := p.merger.Merge(baseContent, string(existingContent), newContent, file.Path)
	if err != nil {
		return errUtils.Build(errUtils.ErrThreeWayMerge).
			WithExplanationf("Failed to perform 3-way merge for file: `%s`", file.Path).
			WithHint("The changes may be too extensive for automatic merging").
			WithHint("Try `--force` to resolve every conflict to the template's version instead").
			WithHint("Or manually merge the changes").
			WithContext("file_path", file.Path).
			WithExitCode(1).
			Err()
	}

	// Check for conflicts. Manual (default) strategy still writes the merged
	// content — with real <<<<<<< / ======= / >>>>>>> conflict markers, and
	// every non-conflicting change from the template applied — so the user
	// has something to actually resolve, rather than the file being left
	// completely untouched. Dry-run never writes, same as the clean path below.
	if result.HasConflicts {
		if !p.DryRun {
			if err := writeFileSecure(existingPath, []byte(result.Content), file.Permissions, true); err != nil {
				return errUtils.Build(errUtils.ErrFileWrite).
					WithCause(err).
					WithExplanationf("Failed to write conflict markers to file: `%s`", existingPath).
					WithHint("Check directory permissions").
					WithHint("Verify sufficient disk space").
					WithContext("file_path", file.Path).
					WithContext("absolute_path", existingPath).
					WithExitCode(2).
					Err()
			}
		}

		builder := errUtils.Build(errUtils.ErrMergeConflict).
			WithExplanationf("Merge resulted in **%d conflict(s)** in file: `%s`", result.ConflictCount, file.Path)
		if result.HasMarkers {
			builder = builder.
				WithHint("Conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`) have been written to the file").
				WithHint("Open it, resolve the conflicts, and remove the markers")
		} else {
			// Some conflicts (e.g. a document the template changed that the
			// user's stream dropped) have no ours/theirs node pair to splice
			// inline markers from, so the template's version was kept as-is
			// instead -- there's nothing in the file itself to point at.
			builder = builder.WithHint("The template's version was kept for the conflicting item(s); review the file to confirm it's what you want")
		}
		builder = builder.
			WithHint("Or re-run with `--force` (or `--merge-strategy=theirs`) to resolve every conflict to the template's version").
			WithContext("file_path", file.Path).
			WithContext("conflict_count", result.ConflictCount).
			WithContext("absolute_path", existingPath)
		if len(result.ConflictPaths) > 0 {
			builder = builder.WithContext("conflict_paths", strings.Join(result.ConflictPaths, ", "))
		}
		return builder.WithExitCode(1).Err()
	}

	// Dry-run: the merge above already ran (and would have surfaced conflicts
	// or threshold errors), but skip the actual write to disk.
	if p.DryRun {
		return nil
	}

	// Write merged content atomically: the target may be open elsewhere, and
	// os.Rename (which WriteFileAtomic uses) replaces a symlink dirent rather
	// than following it.
	if err := writeFileSecure(existingPath, []byte(result.Content), file.Permissions, true); err != nil {
		return errUtils.Build(errUtils.ErrFileWrite).
			WithCause(err).
			WithExplanationf("Failed to write merged file: `%s`", existingPath).
			WithHint("Check directory permissions").
			WithHint("Verify sufficient disk space").
			WithContext("file_path", file.Path).
			WithContext("absolute_path", existingPath).
			WithExitCode(2).
			Err()
	}

	return nil
}

// determineBaseContent determines the base content for 3-way merge.
// Returns (baseContent, shouldSkip, error).
// ShouldSkip is true when the file is user-added and should not be merged.
//
// A meaningful 3-way merge requires the git base: using the (already
// rendered) template content as base would make base identical to "theirs",
// silently turning the merge into a no-op that keeps the user's file and
// drops template updates. Such cases return an error instead.
//
// Migration-aware fallback: "no history at the current rendered path" is
// genuinely ambiguous -- it means either a real user-added file, or a
// spec.files[] entry whose target: changed since the file was last
// generated (e.g. adopting a glob+.file.RelPath-based target on an entry
// that previously rendered verbatim to its own discovered path -- see
// File.OriginalSourcePath's doc comment for the full scenario). There is no
// principled way to *know* which case this is without new bookkeeping (a
// persisted rename record in the project record, keyed per spec entry) that
// is out of scope here. Instead, when OriginalSourcePath differs from the
// current path, this also tries the base lookup there -- the one concrete,
// recoverable signal already available without new bookkeeping, and exactly
// right for the common "previously no target:, verbatim passthrough" case.
// If that also finds nothing, the file is still treated as user-added (never
// silently mutated), but a warning is logged instead of staying silent, so a
// real migration doesn't look identical to an intentional user-added file.
func (p *Processor) determineBaseContent(file File, existingPath string) (string, bool, error) {
	if p.baseStorage == nil {
		// Callers guard against this, but never silently degrade.
		return "", false, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithExplanationf("Cannot determine the merge base for `%s` without a git repository", file.Path).
			WithHint("Run inside a git repository so the base version can be loaded").
			WithHint("Or drop `--update` and use `--force` alone to overwrite the file").
			WithContext("file_path", file.Path).
			WithExitCode(2).
			Err()
	}

	// Try to load base content. relativePath is relative to the merge
	// target's own root -- for GitBaseStorage that's the target's git
	// working tree, for RenderedBaseStorage it's the pristine re-render's
	// root -- both are file-tree-relative, so the same computation applies.
	relativePath, err := filepath.Rel(p.targetPath, existingPath)
	if err != nil {
		relativePath = file.Path // Fallback to template path.
	}

	base, found, err := p.baseStorage.LoadBase(relativePath)
	switch {
	case err != nil:
		return "", false, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithCause(err).
			WithExplanationf("Failed to load the merge base for `%s`", file.Path).
			// p.baseStorage can be either *storage.GitBaseStorage
			// (--update-strategy=tracked) or *storage.RenderedBaseStorage
			// (--update-strategy=rendered, see SetupRenderedBaseStorage) --
			// this hint stays strategy-neutral rather than always pointing at
			// `git show`, which cannot diagnose a rendered base's own
			// pristine-re-render failure.
			WithHint("Verify the merge base is available: for `--update-strategy=tracked`, check the base ref exists (`git show <base-ref>`); for `--update-strategy=rendered`, check the pristine re-render of the recorded ref succeeded").
			WithHint("Or drop `--update` and use `--force` alone to overwrite the file").
			WithContext("file_path", file.Path).
			WithContext("relative_path", relativePath).
			WithExitCode(2).
			Err()
	case found:
		// Use the loaded version as base.
		return base, false, nil
	default:
		return p.determineBaseContentMigrationFallback(file, relativePath)
	}
}

// determineBaseContentMigrationFallback is determineBaseContent's "not found
// at the current rendered path" branch, split out to stay within this repo's
// function-length limit. See determineBaseContent's own doc comment for the
// migration-aware fallback this implements.
func (p *Processor) determineBaseContentMigrationFallback(file File, relativePath string) (string, bool, error) {
	if file.OriginalSourcePath == "" || file.OriginalSourcePath == relativePath {
		// No original-source-path signal to fall back to (a caller that
		// never populates it, e.g. direct engine tests), or it's identical
		// to the current path (target: never changed this file's output
		// path in the first place) -- either way, there's no alternate
		// candidate to try, so this is today's plain "user-added" case.
		return "", true, nil
	}

	migratedBase, migratedFound, migErr := p.baseStorage.LoadBase(file.OriginalSourcePath)
	switch {
	case migErr != nil:
		// The fallback lookup itself failed (e.g. a real git/storage read
		// error) -- this is not "no base exists at the original path", so it
		// must not be folded into the "treat as user-added" case below.
		// Propagate a proper contextual error, mirroring how the primary
		// current-path lookup above handles its own LoadBase error.
		return "", false, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithCause(migErr).
			WithExplanationf("Failed to load the merge base for `%s` at its original path", file.Path).
			WithHint("Verify the merge base is available: for `--update-strategy=tracked`, check the base ref exists (`git show <base-ref>`); for `--update-strategy=rendered`, check the pristine re-render of the recorded ref succeeded").
			WithHint("Or drop `--update` and use `--force` alone to overwrite the file").
			WithContext("file_path", file.Path).
			WithContext("relative_path", relativePath).
			WithContext("original_path", file.OriginalSourcePath).
			WithExitCode(2).
			Err()
	case migratedFound:
		log.Warn(
			"scaffold --update: recovered merge base from the file's original path; target: appears to have changed since this file was last generated",
			"current_path", relativePath,
			"original_path", file.OriginalSourcePath,
		)
		return migratedBase, false, nil
	default:
		// Nothing found under either the current or the original path. Still
		// treated as user-added (never silently mutated), but this is now an
		// ambiguous case -- possibly a genuine migration with no git history
		// under either path yet (e.g. the first `--update` after target:
		// changed hasn't been committed) -- so warn instead of staying silent.
		log.Warn(
			"scaffold --update: no merge base found at this file's current or original path; treating it as user-added and leaving it untouched -- if target: changed recently, future template updates will not be applied to this file automatically",
			"current_path", relativePath,
			"original_path", file.OriginalSourcePath,
		)
		return "", true, nil
	}
}
