package exec

import (
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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

	currentStacks, err := ExecuteDescribeStacks(
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
		nil, // AuthManager passed from describe affected command layer
	)
	if err != nil {
		return nil, nil, nil, err
	}

	localRepoFileSystemPathAbs, err := filepath.Abs(localRepoFileSystemPath)
	if err != nil {
		return nil, nil, nil, err
	}

	basePath := atmosConfig.BasePath

	// Handle `atmos` absolute base path.
	// Absolute base path can be set in the `base_path` attribute in `atmos.yaml`, or using the ENV var `ATMOS_BASE_PATH` (as it's done in `geodesic`)
	// If the `atmos` base path is absolute, find the relative path between the local repo path and the `atmos` base path.
	// This relative path (the difference) is then used below to join with the remote (cloned) target repo path.
	if filepath.IsAbs(basePath) {
		basePath, err = filepath.Rel(localRepoFileSystemPathAbs, basePath)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	// Update paths to point to the cloned remote repo dir
	currentStacksBaseAbsolutePath := atmosConfig.StacksBaseAbsolutePath
	currentStacksTerraformDirAbsolutePath := atmosConfig.TerraformDirAbsolutePath
	currentStacksHelmfileDirAbsolutePath := atmosConfig.HelmfileDirAbsolutePath
	currentStacksPackerDirAbsolutePath := atmosConfig.PackerDirAbsolutePath
	currentStacksStackConfigFilesAbsolutePaths := atmosConfig.StackConfigFilesAbsolutePaths

	atmosConfig.StacksBaseAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Stacks.BasePath)
	atmosConfig.TerraformDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Terraform.BasePath)
	atmosConfig.HelmfileDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Helmfile.BasePath)
	atmosConfig.PackerDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Packer.BasePath)
	atmosConfig.StackConfigFilesAbsolutePaths, err = u.JoinPaths(
		filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Stacks.BasePath),
		atmosConfig.StackConfigFilesRelativePaths,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	remoteStacks, err := ExecuteDescribeStacks(
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
		nil, // AuthManager passed from describe affected command layer
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// Restore atmosConfig
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
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return affected, localRepoHead, remoteRepoHead, nil
}

// findAffected returns a list of all affected components in all stacks.
// Uses parallel processing for improved performance.
func findAffected(
	currentStacks *map[string]any,
	remoteStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stackToFilter string,
	excludeLocked bool,
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
	)
}
