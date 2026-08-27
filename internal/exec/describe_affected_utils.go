package exec

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// evalSymlinksBestEffort resolves symlinks in path even when the full path
// does not exist, by resolving the deepest existing ancestor and re-appending
// the non-existent remainder. The plain filepath.EvalSymlinks errors on
// missing paths, and unused helmfile/packer default dirs routinely do not exist.
func evalSymlinksBestEffort(path string) string {
	remainder := ""
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Nothing along the path resolves — return the input unchanged.
			return path
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

// rebaseOnePathOntoWorktree maps one absolute config path from the local repo
// onto the BASE worktree checkout.
//
// Both sides are symlink-normalized before computing the relative path: the
// repo root typically comes from git (symlink-resolved) while config paths
// derive from the CWD as the shell reported it (logical, unresolved), and
// mixing the two forms makes filepath.Rel produce a `..`-climbing path.
// Joined onto the worktree, that path either lands on a nonexistent dir (the
// BASE scan finds no stack manifests and EVERY component is reported as
// affected) or clamps at the filesystem root and lands back on the HEAD repo
// (BASE == HEAD and NOTHING is reported as affected). Both are silent, so a
// path that still escapes after normalization is a hard error — it cannot be
// represented inside the BASE checkout.
func rebaseOnePathOntoWorktree(localRepoAbs, currentAbs, worktreePath string) (string, error) {
	rel, err := filepath.Rel(evalSymlinksBestEffort(localRepoAbs), evalSymlinksBestEffort(currentAbs))
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(
			"%w: config path %q is outside the repository root %q and cannot be located in the BASE checkout %q",
			errUtils.ErrGitPathEscapesWorktree, currentAbs, localRepoAbs, worktreePath,
		)
	}
	return filepath.Join(worktreePath, rel), nil
}

// rebaseConfigPathsOntoWorktree points atmosConfig's absolute paths at the
// BASE worktree checkout, preserving each path's location relative to the
// repository root. The caller saves and restores the original values.
func rebaseConfigPathsOntoWorktree(atmosConfig *schema.AtmosConfiguration, localRepoAbs, worktreePath string) error {
	targets := []struct {
		current string
		set     func(string)
	}{
		{atmosConfig.BasePathAbsolute, func(p string) { atmosConfig.BasePath = p; atmosConfig.BasePathAbsolute = p }},
		{atmosConfig.StacksBaseAbsolutePath, func(p string) { atmosConfig.StacksBaseAbsolutePath = p }},
		{atmosConfig.TerraformDirAbsolutePath, func(p string) { atmosConfig.TerraformDirAbsolutePath = p }},
		{atmosConfig.HelmfileDirAbsolutePath, func(p string) { atmosConfig.HelmfileDirAbsolutePath = p }},
		{atmosConfig.PackerDirAbsolutePath, func(p string) { atmosConfig.PackerDirAbsolutePath = p }},
	}
	for _, target := range targets {
		rebased, err := rebaseOnePathOntoWorktree(localRepoAbs, target.current, worktreePath)
		if err != nil {
			return err
		}
		target.set(rebased)
	}
	return nil
}

func executeDescribeAffected(
	atmosConfig *schema.AtmosConfiguration,
	localRepoFileSystemPath string,
	remoteRepoFileSystemPath string,
	localRepo *git.Repository,
	remoteRepo *git.Repository,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	excludeLocked bool,
	authManager auth.AuthManager,
	authDisabled bool,
	errOptions DescribeStacksErrorOptions,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, error) {
	localRepoHead, err := localRepo.Head()
	if err != nil {
		return nil, nil, nil, err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Current", "HEAD", localRepoHead)
	log.Debug("Current", "BASE", remoteRepoHead)

	currentStacks, err := ExecuteDescribeStacksWithOptions(
		atmosConfig,
		stack,
		nil,
		nil,
		nil,
		false,
		processTemplates,
		processYamlFunctions,
		false,
		skip,
		authManager,
		authDisabled,
		errOptions,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// Clear base component cache between current and remote stack processing
	// to prevent cache contamination (cache keys don't include path information).
	ClearBaseComponentConfigCache()

	localRepoFileSystemPathAbs, err := filepath.Abs(localRepoFileSystemPath)
	if err != nil {
		return nil, nil, nil, err
	}

	// Save current paths before modification.
	currentBasePath := atmosConfig.BasePath
	currentBasePathAbsolute := atmosConfig.BasePathAbsolute
	currentStacksBaseAbsolutePath := atmosConfig.StacksBaseAbsolutePath
	currentStacksTerraformDirAbsolutePath := atmosConfig.TerraformDirAbsolutePath
	currentStacksHelmfileDirAbsolutePath := atmosConfig.HelmfileDirAbsolutePath
	currentStacksPackerDirAbsolutePath := atmosConfig.PackerDirAbsolutePath
	currentStacksStackConfigFilesAbsolutePaths := atmosConfig.StackConfigFilesAbsolutePaths

	// Re-base the config's absolute paths onto the BASE checkout, preserving
	// each path's location relative to the repo root. This handles the case
	// where atmos is run from a subdirectory (e.g., -C examples/demo-stacks).
	// BasePath and BasePathAbsolute must be updated so that !include can
	// resolve files relative to the base path in the remote repo (fix for
	// GitHub issue #2090).
	if err := rebaseConfigPathsOntoWorktree(atmosConfig, localRepoFileSystemPathAbs, remoteRepoFileSystemPath); err != nil {
		return nil, nil, nil, err
	}

	// Re-scan the BASE (remote) directory for stack config files.
	// This is necessary to detect deleted stacks - files that exist in BASE but not in HEAD.
	// We cannot simply convert HEAD's file paths to BASE paths, as that would miss files
	// that only exist in BASE.
	remoteIncludeStackAbsPaths, err := u.JoinPaths(atmosConfig.StacksBaseAbsolutePath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return nil, nil, nil, err
	}
	remoteExcludeStackAbsPaths, err := u.JoinPaths(atmosConfig.StacksBaseAbsolutePath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return nil, nil, nil, err
	}
	remoteStackConfigFilesAbsolutePaths, _, _, err := cfg.FindAllStackConfigsInPathsForStack(
		*atmosConfig,
		stack, // Apply the same stack filter if provided.
		remoteIncludeStackAbsPaths,
		remoteExcludeStackAbsPaths,
	)
	if err != nil {
		// Propagate unexpected errors (permission issues, invalid paths, etc.) to avoid silently
		// producing incorrect results.
		if !errors.Is(err, errUtils.ErrNoStackManifestsFound) && !errors.Is(err, errUtils.ErrFailedToFindImport) {
			return nil, nil, nil, err
		}
		// No stack manifests found in BASE (e.g. greenfield branch introducing Atmos for the first time,
		// or BASE branch uses a different stack structure). Treat BASE as empty: all HEAD stacks are new.
		log.Warn(
			"No Atmos stack manifests found in BASE; treating BASE as empty (all HEAD components will be reported as affected)",
			"hint", "This is expected for greenfield branches or when the base branch does not yet use Atmos",
			"error", err,
		)
		remoteStackConfigFilesAbsolutePaths = []string{}
	}
	atmosConfig.StackConfigFilesAbsolutePaths = remoteStackConfigFilesAbsolutePaths

	remoteStacks, err := ExecuteDescribeStacksWithOptions(
		atmosConfig,
		stack,
		nil,
		nil,
		nil,
		true,
		processTemplates,
		processYamlFunctions,
		false,
		skip,
		authManager,
		authDisabled,
		errOptions,
	)
	if err != nil {
		// If the BASE cannot be processed (e.g. greenfield: no atmos.yaml or stack configs in BASE,
		// or BASE uses a different/incompatible stack structure), treat it as empty so that all HEAD
		// components are reported as affected.  This is correct: everything is "new" relative to BASE.
		if errors.Is(err, errUtils.ErrFailedToFindImport) || errors.Is(err, errUtils.ErrNoStackManifestsFound) {
			log.Warn(
				"Could not process BASE stack configuration; treating BASE as empty (all HEAD components will be reported as affected)",
				"hint", "This is expected for greenfield branches or when the base branch does not yet use Atmos",
				"error", err,
			)
			remoteStacks = map[string]any{}
		} else {
			return nil, nil, nil, err
		}
	}

	// Restore atmosConfig.
	atmosConfig.BasePath = currentBasePath
	atmosConfig.BasePathAbsolute = currentBasePathAbsolute
	atmosConfig.StacksBaseAbsolutePath = currentStacksBaseAbsolutePath
	atmosConfig.TerraformDirAbsolutePath = currentStacksTerraformDirAbsolutePath
	atmosConfig.HelmfileDirAbsolutePath = currentStacksHelmfileDirAbsolutePath
	atmosConfig.PackerDirAbsolutePath = currentStacksPackerDirAbsolutePath
	atmosConfig.StackConfigFilesAbsolutePaths = currentStacksStackConfigFilesAbsolutePaths

	log.Debug("Getting current working repo commit object")

	localCommit, err := localRepo.CommitObject(localRepoHead.Hash())
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got current working repo commit object")
	log.Debug("Getting current working repo commit tree")

	localTree, err := localCommit.Tree()
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got current working repo commit tree")
	log.Debug("Getting remote repo commit object")

	remoteCommit, err := remoteRepo.CommitObject(remoteRepoHead.Hash())
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got remote repo commit object")
	log.Debug("Getting remote repo commit tree")

	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debug("Got remote repo commit tree")
	log.Debug("Finding difference between the current working branch and remote target branch")

	// Find a slice of Patch objects with all the changes between the current working and remote trees
	patch, err := localTree.Patch(remoteTree)
	if err != nil {
		return nil, nil, nil, err
	}

	var changedFiles []string

	if len(patch.Stats()) > 0 {
		log.Debug("Found difference between the current working branch and remote target branch")
		log.Debug("Changed", "files", patch.Stats())

		for _, fileStat := range patch.Stats() {
			changedFiles = append(changedFiles, fileStat.Name)
		}
	} else {
		log.Debug("The current working branch and remote target branch are the same")
	}

	affected, err := findAffected(
		&currentStacks,
		&remoteStacks,
		atmosConfig,
		changedFiles,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		excludeLocked,
		localRepoFileSystemPathAbs,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return affected, localRepoHead, remoteRepoHead, nil
}

// findAffected returns a list of all affected components in all stacks.
// Uses parallel processing for improved performance.
// The gitRepoRoot parameter is the absolute path to the git repository root, used to resolve
// relative file paths from git diff.
func findAffected(
	currentStacks *map[string]any,
	remoteStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stackToFilter string,
	excludeLocked bool,
	gitRepoRoot string,
) ([]schema.Affected, error) {
	// Use parallel implementation for significant performance improvement (40-60% faster).
	return findAffectedParallel(
		currentStacks,
		remoteStacks,
		atmosConfig,
		changedFiles,
		includeSpaceliftAdminStacks,
		includeSettings,
		stackToFilter,
		excludeLocked,
		gitRepoRoot,
	)
}
