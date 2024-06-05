package exec

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/mapstructure"
	cp "github.com/otiai10/copy"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	localRepoIsNotGitRepoError  = errors.New("the local repo is not a Git repository. Check that it was initialized and has '.git' folder")
	remoteRepoIsNotGitRepoError = errors.New("the target remote repo is not a Git repository. Check that it was initialized and has '.git' folder")
)

// ExecuteDescribeAffectedWithTargetRefClone clones the remote reference,
// processes stack configs, and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffectedWithTargetRefClone(
	cliConfig schema.CliConfiguration,
	ref string,
	sha string,
	sshKeyPath string,
	sshKeyPassword string,
	verbose bool,
	includeSpaceliftAdminStacks bool,
) ([]schema.Affected, error) {

	if verbose {
		cliConfig.Logs.Level = u.LogLevelTrace
	}

	localPath := "."

	localRepo, err := git.PlainOpenWithOptions(localPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	// Get the Git config of the local repo
	localRepoConfig, err := localRepo.Config()
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	localRepoWorktree, err := localRepo.Worktree()
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	localRepoPath := localRepoWorktree.Filesystem.Root()

	// Get the remotes of the local repo
	keys := []string{}
	for k := range localRepoConfig.Remotes {
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		return nil, localRepoIsNotGitRepoError
	}

	// Get the origin URL of the current remoteRepo
	remoteUrls := localRepoConfig.Remotes[keys[0]].URLs
	if len(remoteUrls) == 0 {
		return nil, localRepoIsNotGitRepoError
	}

	repoUrl := remoteUrls[0]
	if repoUrl == "" {
		return nil, localRepoIsNotGitRepoError
	}

	// Clone the remote repo
	// https://git-scm.com/book/en/v2/Git-Internals-Git-References
	// https://git-scm.com/docs/git-show-ref
	// https://github.com/go-git/go-git/tree/master/_examples
	// https://stackoverflow.com/questions/56810719/how-to-checkout-a-specific-sha-in-a-git-repo-using-golang
	// https://golang.hotexamples.com/examples/gopkg.in.src-d.go-git.v4.plumbing/-/ReferenceName/golang-referencename-function-examples.html
	// https://stackoverflow.com/questions/58905690/how-to-identify-which-files-have-changed-between-git-commits
	// https://github.com/src-d/go-git/issues/604

	// Create a temp dir to clone the remote repo to
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		return nil, err
	}

	defer removeTempDir(cliConfig, tempDir)

	u.LogTrace(cliConfig, fmt.Sprintf("\nCloning repo '%s' into the temp dir '%s'", repoUrl, tempDir))

	cloneOptions := git.CloneOptions{
		URL:          repoUrl,
		NoCheckout:   false,
		SingleBranch: false,
	}

	// If `ref` flag is not provided, it will clone the HEAD of the default branch
	if ref != "" {
		cloneOptions.ReferenceName = plumbing.ReferenceName(ref)
		u.LogTrace(cliConfig, fmt.Sprintf("\nCloning Git ref '%s' ...\n", ref))
	} else {
		u.LogTrace(cliConfig, "\nCloned the HEAD of the default branch ...\n")
	}

	if verbose {
		cloneOptions.Progress = os.Stdout
	}

	// Clone private repos using SSH
	// https://gist.github.com/efontan/e8e8818dc0845d3bd7bf1343c984ae7b
	// https://github.com/src-d/go-git/issues/550
	if sshKeyPath != "" {
		sshKeyContent, err := os.ReadFile(sshKeyPath)
		if err != nil {
			return nil, err
		}

		sshPublicKey, err := ssh.NewPublicKeys("git", sshKeyContent, sshKeyPassword)
		if err != nil {
			return nil, err
		}

		// Use the SSH key to clone the repo
		cloneOptions.Auth = sshPublicKey

		// Update the repo URL to SSH format
		// https://mirrors.edge.kernel.org/pub/software/scm/git/docs/git-clone.html
		cloneOptions.URL = strings.Replace(repoUrl, "https://", "ssh://", 1)
	}

	remoteRepo, err := git.PlainClone(tempDir, false, &cloneOptions)
	if err != nil {
		return nil, err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, err
	}

	if ref != "" {
		u.LogTrace(cliConfig, fmt.Sprintf("\nCloned Git ref '%s'\n", ref))
	} else {
		u.LogTrace(cliConfig, fmt.Sprintf("\nCloned Git ref '%s'\n", remoteRepoHead.Name()))
	}

	// Check if a commit SHA was provided and checkout the repo at that commit SHA
	if sha != "" {
		u.LogTrace(cliConfig, fmt.Sprintf("\nChecking out commit SHA '%s' ...\n", sha))

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, err
		}

		checkoutOptions := git.CheckoutOptions{
			Hash:   plumbing.NewHash(sha),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			return nil, err
		}

		u.LogTrace(cliConfig, fmt.Sprintf("\nChecked out commit SHA '%s'\n", sha))
	}

	affected, err := executeDescribeAffected(cliConfig, localRepoPath, tempDir, localRepo, remoteRepo, verbose, includeSpaceliftAdminStacks)
	if err != nil {
		return nil, err
	}

	return affected, nil
}

// ExecuteDescribeAffectedWithTargetRefCheckout checks out the target reference,
// processes stack configs, and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffectedWithTargetRefCheckout(
	cliConfig schema.CliConfiguration,
	ref string,
	sha string,
	verbose bool,
	includeSpaceliftAdminStacks bool,
) ([]schema.Affected, error) {

	if verbose {
		cliConfig.Logs.Level = u.LogLevelTrace
	}

	localPath := "."

	localRepo, err := git.PlainOpenWithOptions(localPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	// Check the Git config of the local repo
	_, err = localRepo.Config()
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	localRepoWorktree, err := localRepo.Worktree()
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	localRepoPath := localRepoWorktree.Filesystem.Root()

	// Create a temp dir for the target ref
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		return nil, err
	}

	defer removeTempDir(cliConfig, tempDir)

	// Copy the local repo into the temp directory
	u.LogTrace(cliConfig, fmt.Sprintf("\nCopying the local repo into the temp directory '%s' ...", tempDir))

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		// Skip specifies which files should be skipped
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") {
				return true, nil
			}
			return false, nil
		},
	}

	if err = cp.Copy(localRepoPath, tempDir, copyOptions); err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("Copied the local repo into the temp directory '%s'\n", tempDir))

	remoteRepo, err := git.PlainOpenWithOptions(tempDir, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the target ref
	_, err = remoteRepo.Config()
	if err != nil {
		return nil, errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	if sha != "" {
		u.LogTrace(cliConfig, fmt.Sprintf("\nChecking out commit SHA '%s' ...\n", sha))

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, err
		}

		checkoutOptions := git.CheckoutOptions{
			Hash:   plumbing.NewHash(sha),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			return nil, err
		}

		u.LogTrace(cliConfig, fmt.Sprintf("Checked out commit SHA '%s'\n", sha))
	} else {
		// If `ref` is not provided, use the HEAD of the remote origin
		if ref == "" {
			ref = "refs/remotes/origin/HEAD"
		}

		u.LogTrace(cliConfig, fmt.Sprintf("\nChecking out Git ref '%s' ...", ref))

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, err
		}

		checkoutOptions := git.CheckoutOptions{
			Branch: plumbing.ReferenceName(ref),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			if strings.Contains(err.Error(), "reference not found") {
				errorMessage := fmt.Sprintf("the Git ref '%s' does not exist on the local filesystem"+
					"\nmake sure it's correct and was cloned by Git from the remote, or use the '--clone-target-ref=true' flag to clone it"+
					"\nrefer to https://atmos.tools/cli/commands/describe/affected for more details", ref)
				err = errors.New(errorMessage)
			}
			return nil, err
		}

		u.LogTrace(cliConfig, fmt.Sprintf("Checked out Git ref '%s'\n", ref))
	}

	affected, err := executeDescribeAffected(cliConfig, localRepoPath, tempDir, localRepo, remoteRepo, verbose, includeSpaceliftAdminStacks)
	if err != nil {
		return nil, err
	}

	return affected, nil
}

// ExecuteDescribeAffectedWithTargetRepoPath uses `repo-path` to access the target repo, and processes stack configs
// and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffectedWithTargetRepoPath(
	cliConfig schema.CliConfiguration,
	targetRefPath string,
	verbose bool,
	includeSpaceliftAdminStacks bool,
) ([]schema.Affected, error) {

	localPath := "."

	localRepo, err := git.PlainOpenWithOptions(localPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	// Check the Git config of the local repo
	_, err = localRepo.Config()
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	localRepoWorktree, err := localRepo.Worktree()
	if err != nil {
		return nil, errors.Join(err, localRepoIsNotGitRepoError)
	}

	localRepoPath := localRepoWorktree.Filesystem.Root()

	remoteRepo, err := git.PlainOpenWithOptions(targetRefPath, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the remote target repo
	_, err = remoteRepo.Config()
	if err != nil {
		return nil, errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	affected, err := executeDescribeAffected(cliConfig, localRepoPath, targetRefPath, localRepo, remoteRepo, verbose, includeSpaceliftAdminStacks)
	if err != nil {
		return nil, err
	}

	return affected, nil
}

func executeDescribeAffected(
	cliConfig schema.CliConfiguration,
	localRepoFileSystemPath string,
	remoteRepoFileSystemPath string,
	localRepo *git.Repository,
	remoteRepo *git.Repository,
	verbose bool,
	includeSpaceliftAdminStacks bool,
) ([]schema.Affected, error) {

	if verbose {
		cliConfig.Logs.Level = u.LogLevelTrace
	}

	localRepoHead, err := localRepo.Head()
	if err != nil {
		return nil, err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("Current working repo HEAD: %s", localRepoHead))
	u.LogTrace(cliConfig, fmt.Sprintf("Remote repo HEAD: %s", remoteRepoHead))

	currentStacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	if err != nil {
		return nil, err
	}

	localRepoFileSystemPathAbs, err := filepath.Abs(localRepoFileSystemPath)
	if err != nil {
		return nil, err
	}

	basePath := cliConfig.BasePath

	// Handle `atmos` absolute base path.
	// Absolute base path can be set in the `base_path` attribute in `atmos.yaml`, or using the ENV var `ATMOS_BASE_PATH` (as it's done in `geodesic`)
	// If the `atmos` base path is absolute, find the relative path between the local repo path and the `atmos` base path.
	// This relative path (the difference) is then used below to join with the remote (cloned) repo path.
	if path.IsAbs(basePath) {
		basePath, err = filepath.Rel(localRepoFileSystemPathAbs, basePath)
		if err != nil {
			return nil, err
		}
	}

	// Update paths to point to the cloned remote repo dir
	cliConfig.StacksBaseAbsolutePath = path.Join(remoteRepoFileSystemPath, basePath, cliConfig.Stacks.BasePath)
	cliConfig.TerraformDirAbsolutePath = path.Join(remoteRepoFileSystemPath, basePath, cliConfig.Components.Terraform.BasePath)
	cliConfig.HelmfileDirAbsolutePath = path.Join(remoteRepoFileSystemPath, basePath, cliConfig.Components.Helmfile.BasePath)

	cliConfig.StackConfigFilesAbsolutePaths, err = u.JoinAbsolutePathWithPaths(
		path.Join(remoteRepoFileSystemPath, basePath, cliConfig.Stacks.BasePath),
		cliConfig.StackConfigFilesRelativePaths,
	)
	if err != nil {
		return nil, err
	}

	remoteStacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, true)
	if err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("\nGetting current working repo commit object..."))

	localCommit, err := localRepo.CommitObject(localRepoHead.Hash())
	if err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("Got current working repo commit object"))
	u.LogTrace(cliConfig, fmt.Sprintf("Getting current working repo commit tree..."))

	localTree, err := localCommit.Tree()
	if err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("Got current working repo commit tree"))
	u.LogTrace(cliConfig, fmt.Sprintf("Getting remote repo commit object..."))

	remoteCommit, err := remoteRepo.CommitObject(remoteRepoHead.Hash())
	if err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("Got remote repo commit object"))
	u.LogTrace(cliConfig, fmt.Sprintf("Getting remote repo commit tree..."))

	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return nil, err
	}

	u.LogTrace(cliConfig, fmt.Sprintf("Got remote repo commit tree"))
	u.LogTrace(cliConfig, fmt.Sprintf("Finding difference between the current working branch and remote target branch ..."))

	// Find a slice of Patch objects with all the changes between the current working and remote trees
	patch, err := localTree.Patch(remoteTree)
	if err != nil {
		return nil, err
	}

	var changedFiles []string

	if len(patch.Stats()) > 0 {
		u.LogTrace(cliConfig, fmt.Sprintf("Found difference between the current working branch and remote target branch"))
		u.LogTrace(cliConfig, "\nChanged files:\n")

		for _, fileStat := range patch.Stats() {
			u.LogTrace(cliConfig, fileStat.Name)
			changedFiles = append(changedFiles, fileStat.Name)
		}
		u.LogTrace(cliConfig, "")
	} else {
		u.LogTrace(cliConfig, fmt.Sprintf("The current working branch and remote target branch are the same"))
	}

	affected, err := findAffected(currentStacks, remoteStacks, cliConfig, changedFiles, includeSpaceliftAdminStacks)
	if err != nil {
		return nil, err
	}

	return affected, nil
}

// findAffected returns a list of all affected components in all stacks
func findAffected(
	currentStacks map[string]any,
	remoteStacks map[string]any,
	cliConfig schema.CliConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
) ([]schema.Affected, error) {

	res := []schema.Affected{}
	var err error

	for stackName, stackSection := range currentStacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {

				// Terraform
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection["metadata"].(map[any]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Check `metadata` section
								if !isEqual(remoteStacks, stackName, "terraform", componentName, metadataSection, "metadata") {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.metadata",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}

							// Check the Terraform configuration of the component
							if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
								// Check if the component uses some external modules (on the local filesystem) that have changed
								changed, err := areTerraformComponentModulesChanged(component, cliConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component.module",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
									)
									if err != nil {
										return nil, err
									}
									continue
								}

								// Check if any files in the component's folder have changed
								changed, err = isComponentFolderChanged(component, "terraform", cliConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection["settings"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, settingsSection, "settings") {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}

								// Check `settings.depends_on.file` and `settings.depends_on.folder`
								// Convert the `settings` section to the `Settings` structure
								var stackComponentSettings schema.Settings
								err = mapstructure.Decode(settingsSection, &stackComponentSettings)
								if err != nil {
									return nil, err
								}

								// Skip if the stack component has an empty `settings.depends_on` section
								if reflect.ValueOf(stackComponentSettings).IsZero() ||
									reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
									continue
								}

								isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChanged(
									changedFiles,
									stackComponentSettings.DependsOn,
								)

								if err != nil {
									return nil, err
								}

								if isFolderOrFileChanged {
									changedFile := ""
									if changedType == "file" {
										changedFile = changedFileOrFolder
									}

									changedFolder := ""
									if changedType == "folder" {
										changedFolder = changedFileOrFolder
									}

									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      changedType,
										File:          changedFile,
										Folder:        changedFolder,
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
						}
					}
				}

				// Helmfile
				if helmfileSection, ok := componentsSection["helmfile"].(map[string]any); ok {
					for componentName, compSection := range helmfileSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection["metadata"].(map[any]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Check `metadata` section
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, metadataSection, "metadata") {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.metadata",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}

							// Check the Helmfile configuration of the component
							if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
								// Check if any files in the component's folder have changed
								changed, err := isComponentFolderChanged(component, "helmfile", cliConfig, changedFiles)
								if err != nil {
									return nil, err
								}

								if changed {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "component",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection["settings"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, settingsSection, "settings") {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
									)
									if err != nil {
										return nil, err
									}
									continue
								}

								// Check `settings.depends_on.file` and `settings.depends_on.folder`
								// Convert the `settings` section to the `Settings` structure
								var stackComponentSettings schema.Settings
								err = mapstructure.Decode(settingsSection, &stackComponentSettings)
								if err != nil {
									return nil, err
								}

								// Skip if the stack component has an empty `settings.depends_on` section
								if reflect.ValueOf(stackComponentSettings).IsZero() ||
									reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
									continue
								}

								isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChanged(
									changedFiles,
									stackComponentSettings.DependsOn,
								)

								if err != nil {
									return nil, err
								}

								if isFolderOrFileChanged {
									changedFile := ""
									if changedType == "file" {
										changedFile = changedFileOrFolder
									}

									changedFolder := ""
									if changedType == "folder" {
										changedFolder = changedFileOrFolder
									}

									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      changedType,
										File:          changedFile,
										Folder:        changedFolder,
									}
									res, err = appendToAffected(
										cliConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
						}
					}
				}
			}
		}
	}

	return res, nil
}

// appendToAffected adds an item to the affected list, and adds the Spacelift stack and Atlantis project (if configured)
func appendToAffected(
	cliConfig schema.CliConfiguration,
	componentName string,
	stackName string,
	componentSection map[string]any,
	affectedList []schema.Affected,
	affected schema.Affected,
	includeSpaceliftAdminStacks bool,
	stacks map[string]any,
) ([]schema.Affected, error) {

	// If the affected component in the stack was already added to the result, don't add it again
	for _, v := range affectedList {
		if v.Component == affected.Component && v.Stack == affected.Stack && v.ComponentType == affected.ComponentType {
			return affectedList, nil
		}
	}

	if affected.ComponentType == "terraform" {
		varSection := map[any]any{}
		settingsSection := map[any]any{}

		if i, ok2 := componentSection["vars"]; ok2 {
			varSection = i.(map[any]any)
		}

		if i, ok2 := componentSection["settings"]; ok2 {
			settingsSection = i.(map[any]any)
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:         componentName,
			Stack:                    stackName,
			ComponentVarsSection:     varSection,
			ComponentSettingsSection: settingsSection,
			ComponentSection: map[string]any{
				cfg.VarsSectionName:     varSection,
				cfg.SettingsSectionName: settingsSection,
			},
		}

		// Affected Spacelift stack
		spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(cliConfig, configAndStacksInfo)
		if err != nil {
			return nil, err
		}
		affected.SpaceliftStack = spaceliftStackName

		// Affected Atlantis project
		atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(cliConfig, configAndStacksInfo)
		if err != nil {
			return nil, err
		}
		affected.AtlantisProject = atlantisProjectName

		if includeSpaceliftAdminStacks {
			affectedList, err = addAffectedSpaceliftAdminStack(cliConfig, affectedList, settingsSection, stacks, stackName, componentName, configAndStacksInfo)
			if err != nil {
				return nil, err
			}
		}
	}

	// Check `component` section and add `ComponentPath` to the output
	affected.ComponentPath = BuildComponentPath(cliConfig, componentSection, affected.ComponentType)
	affected.StackSlug = fmt.Sprintf("%s-%s", stackName, strings.Replace(componentName, "/", "-", -1))

	return append(affectedList, affected), nil
}

// isEqual compares a section of a component from the remote stacks with a section of a local component
func isEqual(
	remoteStacks map[string]any,
	localStackName string,
	componentType string,
	localComponentName string,
	localSection map[any]any,
	sectionName string,
) bool {

	if remoteStackSection, ok := remoteStacks[localStackName].(map[string]any); ok {
		if remoteComponentsSection, ok := remoteStackSection["components"].(map[string]any); ok {
			if remoteComponentTypeSection, ok := remoteComponentsSection[componentType].(map[string]any); ok {
				if remoteComponentSection, ok := remoteComponentTypeSection[localComponentName].(map[string]any); ok {
					if remoteSection, ok := remoteComponentSection[sectionName].(map[any]any); ok {
						if reflect.DeepEqual(localSection, remoteSection) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// isComponentDependentFolderOrFileChanged checks if a folder or file that the component depends on has changed
func isComponentDependentFolderOrFileChanged(
	changedFiles []string,
	deps schema.DependsOn,
) (bool, string, string, error) {

	hasDependencies := false
	isChanged := false
	changedType := ""
	changedFileOrFolder := ""
	pathPatternSuffix := ""

	for _, dep := range deps {
		if isChanged {
			break
		}

		if dep.File != "" {
			changedType = "file"
			changedFileOrFolder = dep.File
			pathPatternSuffix = ""
			hasDependencies = true
		} else if dep.Folder != "" {
			changedType = "folder"
			changedFileOrFolder = dep.Folder
			pathPatternSuffix = "/**"
			hasDependencies = true
		}

		if hasDependencies {
			changedFileOrFolderAbs, err := filepath.Abs(changedFileOrFolder)
			if err != nil {
				return false, "", "", err
			}

			pathPattern := changedFileOrFolderAbs + pathPatternSuffix

			for _, changedFile := range changedFiles {
				changedFileAbs, err := filepath.Abs(changedFile)
				if err != nil {
					return false, "", "", err
				}

				match, err := u.PathMatch(pathPattern, changedFileAbs)
				if err != nil {
					return false, "", "", err
				}

				if match {
					isChanged = true
					break
				}
			}
		}
	}

	return isChanged, changedType, changedFileOrFolder, nil
}

// isComponentFolderChanged checks if the component folder changed (has changed files in the folder or its sub-folders)
func isComponentFolderChanged(
	component string,
	componentType string,
	cliConfig schema.CliConfiguration,
	changedFiles []string,
) (bool, error) {

	var componentPath string

	switch componentType {
	case "terraform":
		componentPath = path.Join(cliConfig.BasePath, cliConfig.Components.Terraform.BasePath, component)
	case "helmfile":
		componentPath = path.Join(cliConfig.BasePath, cliConfig.Components.Helmfile.BasePath, component)
	}

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return false, err
	}

	componentPathPattern := componentPathAbs + "/**"

	for _, changedFile := range changedFiles {
		changedFileAbs, err := filepath.Abs(changedFile)
		if err != nil {
			return false, err
		}

		match, err := u.PathMatch(componentPathPattern, changedFileAbs)
		if err != nil {
			return false, err
		}

		if match {
			return true, nil
		}
	}

	return false, nil
}

// areTerraformComponentModulesChanged checks if any of the external Terraform modules (but on the local filesystem) that the component uses have changed
func areTerraformComponentModulesChanged(
	component string,
	cliConfig schema.CliConfiguration,
	changedFiles []string,
) (bool, error) {

	componentPath := path.Join(cliConfig.BasePath, cliConfig.Components.Terraform.BasePath, component)

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return false, err
	}

	terraformConfiguration, _ := tfconfig.LoadModule(componentPathAbs)

	for _, changedFile := range changedFiles {
		changedFileAbs, err := filepath.Abs(changedFile)
		if err != nil {
			return false, err
		}

		for _, moduleConfig := range terraformConfiguration.ModuleCalls {
			// We are processing the local modules only (not from terraform registry), they will have `Version` as an empty string
			if moduleConfig.Version != "" {
				continue
			}

			modulePath := path.Join(path.Dir(moduleConfig.Pos.Filename), moduleConfig.Source)

			modulePathAbs, err := filepath.Abs(modulePath)
			if err != nil {
				return false, err
			}

			modulePathPattern := modulePathAbs + "/**"

			match, err := u.PathMatch(modulePathPattern, changedFileAbs)
			if err != nil {
				return false, err
			}

			if match {
				return true, nil
			}
		}
	}

	return false, nil
}

// addAffectedSpaceliftAdminStack adds the affected Spacelift admin stack that manages the affected child stack
func addAffectedSpaceliftAdminStack(
	cliConfig schema.CliConfiguration,
	affectedList []schema.Affected,
	settingsSection map[any]any,
	stacks map[string]any,
	currentStackName string,
	currentComponentName string,
	configAndStacksInfo schema.ConfigAndStacksInfo,
) ([]schema.Affected, error) {

	// Convert the `settings` section to the `Settings` structure
	var componentSettings schema.Settings
	err := mapstructure.Decode(settingsSection, &componentSettings)
	if err != nil {
		return nil, err
	}

	// Skip if the component has an empty `settings.spacelift` section
	if reflect.ValueOf(componentSettings).IsZero() ||
		reflect.ValueOf(componentSettings.Spacelift).IsZero() {
		return affectedList, nil
	}

	// Find and process `settings.spacelift.admin_stack_config` section
	var adminStackContextSection any
	var adminStackContext schema.Context
	var ok bool

	if adminStackContextSection, ok = componentSettings.Spacelift["admin_stack_selector"]; !ok {
		return affectedList, nil
	}

	err = mapstructure.Decode(adminStackContextSection, &adminStackContext)
	if err != nil {
		return nil, err
	}

	// Skip if the component has an empty `settings.spacelift.admin_stack_selector` section
	if reflect.ValueOf(adminStackContext).IsZero() {
		return affectedList, nil
	}

	var adminStackContextPrefix string

	if cliConfig.Stacks.NameTemplate != "" {
		adminStackContextPrefix, err = u.ProcessTmpl("spacelift-admin-stack-name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return nil, err
		}
	} else {
		adminStackContextPrefix, err = cfg.GetContextPrefix(currentStackName, adminStackContext, GetStackNamePattern(cliConfig), currentStackName)
		if err != nil {
			return nil, err
		}
	}

	var componentVarsSection map[any]any
	var componentSettingsSection map[any]any
	var componentSettingsSpaceliftSection map[any]any

	// Find the Spacelift admin stack that manages the current stack
	for stackName, stackSection := range stacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {

							if componentVarsSection, ok = componentSection["vars"].(map[any]any); !ok {
								return affectedList, nil
							}

							var context schema.Context
							err = mapstructure.Decode(componentVarsSection, &context)
							if err != nil {
								return nil, err
							}

							var contextPrefix string

							if cliConfig.Stacks.NameTemplate != "" {
								contextPrefix, err = u.ProcessTmpl("spacelift-stack-name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
								if err != nil {
									return nil, err
								}
							} else {
								contextPrefix, err = cfg.GetContextPrefix(stackName, context, GetStackNamePattern(cliConfig), stackName)
								if err != nil {
									return nil, err
								}
							}

							if adminStackContext.Component == componentName && adminStackContextPrefix == contextPrefix {
								if componentSettingsSection, ok = componentSection["settings"].(map[any]any); !ok {
									return affectedList, nil
								}

								if componentSettingsSpaceliftSection, ok = componentSettingsSection["spacelift"].(map[any]any); !ok {
									return affectedList, nil
								}

								if spaceliftWorkspaceEnabled, ok := componentSettingsSpaceliftSection["workspace_enabled"].(bool); !ok || !spaceliftWorkspaceEnabled {
									return nil, errors.New(fmt.Sprintf(
										"component '%s' in the stack '%s' has the section 'settings.spacelift.admin_stack_selector' "+
											"to point to the Spacelift admin component '%s' in the stack '%s', "+
											"but that component has Spacelift workspace disabled "+
											"in the 'settings.spacelift.workspace_enabled' section "+
											"and can't be added to the affected stacks",
										currentComponentName,
										currentStackName,
										componentName,
										stackName,
									))
								}

								affectedSpaceliftAdminStack := schema.Affected{
									ComponentType: "terraform",
									Component:     componentName,
									Stack:         stackName,
									Affected:      "stack.settings.spacelift.admin_stack_selector",
								}

								affectedList, err = appendToAffected(
									cliConfig,
									componentName,
									stackName,
									componentSection,
									affectedList,
									affectedSpaceliftAdminStack,
									false,
									nil,
								)
								if err != nil {
									return nil, err
								}
							}
						}
					}
				}
			}
		}
	}

	return affectedList, nil
}

// addDependentsToAffected adds dependent components and stacks to each affected component
func addDependentsToAffected(cliConfig schema.CliConfiguration, affected *[]schema.Affected) error {
	for i := 0; i < len(*affected); i++ {
		a := &(*affected)[i]

		deps, err := ExecuteDescribeDependents(cliConfig, a.Component, a.Stack)
		if err != nil {
			return err
		}

		if len(deps) > 0 {
			a.Dependents = deps
			err = addDependentsToDependents(cliConfig, &deps)
			if err != nil {
				return err
			}
		}
	}

	processIncludedInDependencies(affected)
	return nil
}

// addDependentsToDependents recursively adds dependent components and stacks to each dependent component
func addDependentsToDependents(cliConfig schema.CliConfiguration, dependents *[]schema.Dependent) error {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]

		deps, err := ExecuteDescribeDependents(cliConfig, d.Component, d.Stack)
		if err != nil {
			return err
		}

		if len(deps) > 0 {
			d.Dependents = deps
			err = addDependentsToDependents(cliConfig, &deps)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func processIncludedInDependencies(affected *[]schema.Affected) bool {
	for i := 0; i < len(*affected); i++ {
		a := &(*affected)[i]
		a.IncludedInDependents = processIncludedInDependenciesForAffected(affected, a.StackSlug, i)
	}
	return false
}

func processIncludedInDependenciesForAffected(affected *[]schema.Affected, stackSlug string, affectedIndex int) bool {
	for i := 0; i < len(*affected); i++ {
		if i == affectedIndex {
			continue
		}

		a := &(*affected)[i]

		if len(a.Dependents) > 0 {
			return processIncludedInDependenciesForDependents(&a.Dependents, stackSlug)
		}
	}
	return false
}

func processIncludedInDependenciesForDependents(dependents *[]schema.Dependent, stackSlug string) bool {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]

		if d.StackSlug == stackSlug {
			return true
		}

		if len(d.Dependents) > 0 {
			return processIncludedInDependenciesForDependents(&d.Dependents, stackSlug)
		}
	}
	return false
}
