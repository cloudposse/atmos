package engine

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/storage"
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

// SetupGitStorage initializes git-based storage for 3-way merges.
// The targetPath is used to find the git repository and resolve relative file paths.
// The baseRef specifies which git reference to use as the base for merges (e.g., "main", "v1.0.0").
//
// Returns an error if:
//   - targetPath is not in a git repository
//   - baseRef cannot be resolved
func (p *Processor) SetupGitStorage(targetPath string, baseRef string) error {
	defer perf.Track(nil, "engine.Processor.SetupGitStorage")()

	p.targetPath = targetPath

	// Open git repository at target path
	repo, err := git.PlainOpenWithOptions(targetPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		// Not in a git repo - this is OK, just means we can't use git-based merging
		return nil
	}

	// Create git storage with base ref
	p.gitStorage = storage.NewGitBaseStorage(repo, baseRef)

	// Validate that base ref exists
	if err := p.gitStorage.ValidateBaseRef(); err != nil {
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

	return nil
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
			WithHint("Try using `--force` to overwrite instead").
			WithHint("Or manually merge the changes").
			WithContext("file_path", file.Path).
			WithExitCode(1).
			Err()
	}

	// Check for conflicts
	if result.HasConflicts {
		return errUtils.Build(errUtils.ErrMergeConflict).
			WithExplanationf("Merge resulted in **%d conflict(s)** in file: `%s`", result.ConflictCount, file.Path).
			WithHint("Open the file and look for conflict markers: `<<<<<<<`, `=======`, `>>>>>>>`").
			WithHint("Resolve conflicts manually and re-run the command").
			WithHint("Or use `--force` to overwrite the file completely").
			WithContext("file_path", file.Path).
			WithContext("conflict_count", result.ConflictCount).
			WithContext("absolute_path", existingPath).
			WithExitCode(1).
			Err()
	}

	// Write merged content
	if err := os.WriteFile(existingPath, []byte(result.Content), file.Permissions); err != nil {
		return errUtils.Build(errUtils.ErrFileWrite).
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
func (p *Processor) determineBaseContent(file File, existingPath string) (string, bool, error) {
	if p.gitStorage == nil {
		// Callers guard against this, but never silently degrade.
		return "", false, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithExplanationf("Cannot determine the merge base for `%s` without a git repository", file.Path).
			WithHint("Run inside a git repository so the base version can be loaded").
			WithHint("Or use `--force` to overwrite the file").
			WithContext("file_path", file.Path).
			WithExitCode(2).
			Err()
	}

	// Try to load base content from git.
	relativePath, err := filepath.Rel(p.targetPath, existingPath)
	if err != nil {
		relativePath = file.Path // Fallback to template path.
	}

	gitBase, found, err := p.gitStorage.LoadBase(relativePath)
	switch {
	case err != nil:
		return "", false, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithCause(err).
			WithExplanationf("Failed to load the merge base for `%s` from git", file.Path).
			WithHint("Verify the base ref exists: `git show <base-ref>`").
			WithHint("Or use `--force` to overwrite the file").
			WithContext("file_path", file.Path).
			WithContext("relative_path", relativePath).
			WithExitCode(2).
			Err()
	case found:
		// Use git version as base.
		return gitBase, false, nil
	default:
		// File doesn't exist in base ref.
		// This is a user-added file - skip merge, don't touch it.
		return "", true, nil
	}
}
