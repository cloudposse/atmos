package exec

import (
	"errors"
	"fmt"
	"os"
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
	g "github.com/cloudposse/atmos/pkg/git"
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
	atmosConfig schema.AtmosConfiguration,
	ref string,
	sha string,
	sshKeyPath string,
	sshKeyPassword string,
	verbose bool,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	if verbose {
		atmosConfig.Logs.Level = u.LogLevelTrace
	}

	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
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
		return nil, nil, nil, "", err
	}

	defer removeTempDir(atmosConfig, tempDir)

	u.LogTrace(fmt.Sprintf("\nCloning repo '%s' into the temp dir '%s'", localRepoInfo.RepoUrl, tempDir))

	cloneOptions := git.CloneOptions{
		URL:          localRepoInfo.RepoUrl,
		NoCheckout:   false,
		SingleBranch: false,
	}

	// If `ref` flag is not provided, it will clone the HEAD of the default branch
	if ref != "" {
		cloneOptions.ReferenceName = plumbing.ReferenceName(ref)
		u.LogTrace(fmt.Sprintf("\nCloning Git ref '%s' ...\n", ref))
	} else {
		u.LogTrace("\nCloned the HEAD of the default branch ...\n")
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
			return nil, nil, nil, "", err
		}

		sshPublicKey, err := ssh.NewPublicKeys("git", sshKeyContent, sshKeyPassword)
		if err != nil {
			return nil, nil, nil, "", err
		}

		// Use the SSH key to clone the repo
		cloneOptions.Auth = sshPublicKey

		// Update the repo URL to SSH format
		// https://mirrors.edge.kernel.org/pub/software/scm/git/docs/git-clone.html
		cloneOptions.URL = strings.Replace(localRepoInfo.RepoUrl, "https://", "ssh://", 1)
	}

	remoteRepo, err := git.PlainClone(tempDir, false, &cloneOptions)
	if err != nil {
		return nil, nil, nil, "", err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, nil, nil, "", err
	}

	if ref != "" {
		u.LogTrace(fmt.Sprintf("\nCloned Git ref '%s'\n", ref))
	} else {
		u.LogTrace(fmt.Sprintf("\nCloned Git ref '%s'\n", remoteRepoHead.Name()))
	}

	// Check if a commit SHA was provided and checkout the repo at that commit SHA
	if sha != "" {
		u.LogTrace(fmt.Sprintf("\nChecking out commit SHA '%s' ...\n", sha))

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, nil, nil, "", err
		}

		checkoutOptions := git.CheckoutOptions{
			Hash:   plumbing.NewHash(sha),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			return nil, nil, nil, "", err
		}

		u.LogTrace(fmt.Sprintf("\nChecked out commit SHA '%s'\n", sha))
	}

	affected, localRepoHead, remoteRepoHead, err := executeDescribeAffected(
		atmosConfig,
		localRepoInfo.LocalWorktreePath,
		tempDir,
		localRepo,
		remoteRepo,
		verbose,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		processTemplates,
		processYamlFunctions,
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}

// ExecuteDescribeAffectedWithTargetRefCheckout checks out the target reference,
// processes stack configs, and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffectedWithTargetRefCheckout(
	atmosConfig schema.AtmosConfiguration,
	ref string,
	sha string,
	verbose bool,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	if verbose {
		atmosConfig.Logs.Level = u.LogLevelTrace
	}

	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	// Create a temp dir for the target ref
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		return nil, nil, nil, "", err
	}

	defer removeTempDir(atmosConfig, tempDir)

	// Copy the local repo into the temp directory
	u.LogTrace(fmt.Sprintf("\nCopying the local repo into the temp directory '%s' ...", tempDir))

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		// Skip specifies which files should be skipped
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") {
				return true, nil
			}

			// Check if the file is a socket and skip it
			isSocket, err := u.IsSocket(src)
			if err != nil {
				return true, err
			}
			if isSocket {
				return true, nil
			}

			return false, nil
		},
	}

	if err = cp.Copy(localRepoInfo.LocalWorktreePath, tempDir, copyOptions); err != nil {
		return nil, nil, nil, "", err
	}

	u.LogTrace(fmt.Sprintf("Copied the local repo into the temp directory '%s'\n", tempDir))

	remoteRepo, err := git.PlainOpenWithOptions(tempDir, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the target ref
	_, err = g.GetRepoConfig(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	if sha != "" {
		u.LogTrace(fmt.Sprintf("\nChecking out commit SHA '%s' ...\n", sha))

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, nil, nil, "", err
		}

		checkoutOptions := git.CheckoutOptions{
			Hash:   plumbing.NewHash(sha),
			Create: false,
			Force:  true,
			Keep:   false,
		}

		err = w.Checkout(&checkoutOptions)
		if err != nil {
			return nil, nil, nil, "", err
		}

		u.LogTrace(fmt.Sprintf("Checked out commit SHA '%s'\n", sha))
	} else {
		// If `ref` is not provided, use the HEAD of the remote origin
		if ref == "" {
			ref = "refs/remotes/origin/HEAD"
		}

		u.LogTrace(fmt.Sprintf("\nChecking out Git ref '%s' ...", ref))

		w, err := remoteRepo.Worktree()
		if err != nil {
			return nil, nil, nil, "", err
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
			return nil, nil, nil, "", err
		}

		u.LogTrace(fmt.Sprintf("Checked out Git ref '%s'\n", ref))
	}

	affected, localRepoHead, remoteRepoHead, err := executeDescribeAffected(
		atmosConfig,
		localRepoInfo.LocalWorktreePath,
		tempDir,
		localRepo,
		remoteRepo,
		verbose,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		processTemplates,
		processYamlFunctions,
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}

// ExecuteDescribeAffectedWithTargetRepoPath uses `repo-path` to access the target repo, and processes stack configs
// and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffectedWithTargetRepoPath(
	atmosConfig schema.AtmosConfiguration,
	targetRefPath string,
	verbose bool,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
	localRepo, err := g.GetLocalRepo()
	if err != nil {
		return nil, nil, nil, "", err
	}

	localRepoInfo, err := g.GetRepoInfo(localRepo)
	if err != nil {
		return nil, nil, nil, "", err
	}

	remoteRepo, err := git.PlainOpenWithOptions(targetRefPath, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	// Check the Git config of the remote target repo
	_, err = g.GetRepoConfig(remoteRepo)
	if err != nil {
		return nil, nil, nil, "", errors.Join(err, remoteRepoIsNotGitRepoError)
	}

	affected, localRepoHead, remoteRepoHead, err := executeDescribeAffected(
		atmosConfig,
		localRepoInfo.LocalWorktreePath,
		targetRefPath,
		localRepo,
		remoteRepo,
		verbose,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
		processTemplates,
		processYamlFunctions,
	)
	if err != nil {
		return nil, nil, nil, "", err
	}

	return affected, localRepoHead, remoteRepoHead, localRepoInfo.RepoUrl, nil
}

func executeDescribeAffected(
	atmosConfig schema.AtmosConfiguration,
	localRepoFileSystemPath string,
	remoteRepoFileSystemPath string,
	localRepo *git.Repository,
	remoteRepo *git.Repository,
	verbose bool,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, error) {
	if verbose {
		atmosConfig.Logs.Level = u.LogLevelTrace
	}

	localRepoHead, err := localRepo.Head()
	if err != nil {
		return nil, nil, nil, err
	}

	remoteRepoHead, err := remoteRepo.Head()
	if err != nil {
		return nil, nil, nil, err
	}

	u.LogTrace(fmt.Sprintf("Current HEAD: %s", localRepoHead))
	u.LogTrace(fmt.Sprintf("BASE: %s", remoteRepoHead))

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
	atmosConfig.StacksBaseAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Stacks.BasePath)
	atmosConfig.TerraformDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Terraform.BasePath)
	atmosConfig.HelmfileDirAbsolutePath = filepath.Join(remoteRepoFileSystemPath, basePath, atmosConfig.Components.Helmfile.BasePath)

	atmosConfig.StackConfigFilesAbsolutePaths, err = u.JoinAbsolutePathWithPaths(
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
	)
	if err != nil {
		return nil, nil, nil, err
	}

	u.LogTrace(fmt.Sprintf("\nGetting current working repo commit object..."))

	localCommit, err := localRepo.CommitObject(localRepoHead.Hash())
	if err != nil {
		return nil, nil, nil, err
	}

	u.LogTrace(fmt.Sprintf("Got current working repo commit object"))
	u.LogTrace(fmt.Sprintf("Getting current working repo commit tree..."))

	localTree, err := localCommit.Tree()
	if err != nil {
		return nil, nil, nil, err
	}

	u.LogTrace(fmt.Sprintf("Got current working repo commit tree"))
	u.LogTrace(fmt.Sprintf("Getting remote repo commit object..."))

	remoteCommit, err := remoteRepo.CommitObject(remoteRepoHead.Hash())
	if err != nil {
		return nil, nil, nil, err
	}

	u.LogTrace(fmt.Sprintf("Got remote repo commit object"))
	u.LogTrace(fmt.Sprintf("Getting remote repo commit tree..."))

	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return nil, nil, nil, err
	}

	u.LogTrace(fmt.Sprintf("Got remote repo commit tree"))
	u.LogTrace(fmt.Sprintf("Finding difference between the current working branch and remote target branch ..."))

	// Find a slice of Patch objects with all the changes between the current working and remote trees
	patch, err := localTree.Patch(remoteTree)
	if err != nil {
		return nil, nil, nil, err
	}

	var changedFiles []string

	if len(patch.Stats()) > 0 {
		u.LogTrace(fmt.Sprintf("Found difference between the current working branch and remote target branch"))
		u.LogTrace("\nChanged files:\n")

		for _, fileStat := range patch.Stats() {
			u.LogTrace(fileStat.Name)
			changedFiles = append(changedFiles, fileStat.Name)
		}
		u.LogTrace("")
	} else {
		u.LogTrace(fmt.Sprintf("The current working branch and remote target branch are the same"))
	}

	affected, err := findAffected(
		currentStacks,
		remoteStacks,
		atmosConfig,
		changedFiles,
		includeSpaceliftAdminStacks,
		includeSettings,
		stack,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return affected, localRepoHead, remoteRepoHead, nil
}

// findAffected returns a list of all affected components in all stacks
func findAffected(
	currentStacks map[string]any,
	remoteStacks map[string]any,
	atmosConfig schema.AtmosConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stackToFilter string,
) ([]schema.Affected, error) {
	res := []schema.Affected{}
	var err error

	for stackName, stackSection := range currentStacks {
		// If `--stack` is provided on the command line, processes only components in that stack
		if stackToFilter != "" && stackToFilter != stackName {
			continue
		}

		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {

				// Terraform
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection["metadata"].(map[string]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Use helper function to skip disabled components
								if !isComponentEnabled(metadataSection, componentName, atmosConfig) {
									continue
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
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
								changed, err := areTerraformComponentModulesChanged(component, atmosConfig, changedFiles)
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}

								// Check if any files in the component's folder have changed
								changed, err = isComponentFolderChanged(component, "terraform", atmosConfig, changedFiles)
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									res, err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									res, err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, settingsSection, cfg.SettingsSectionName) {
									affected := schema.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									res, err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
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
							if metadataSection, ok := componentSection["metadata"].(map[string]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Use helper function to skip disabled components
								if !isComponentEnabled(metadataSection, componentName, atmosConfig) {
									continue
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
										includeSettings,
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
								changed, err := isComponentFolderChanged(component, "helmfile", atmosConfig, changedFiles)
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, varSection, "vars") {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.vars",
									}
									res, err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, envSection, "env") {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.env",
									}
									res, err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
										includeSettings,
									)
									if err != nil {
										return nil, err
									}
									continue
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, settingsSection, cfg.SettingsSectionName) {
									affected := schema.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
										Affected:      "stack.settings",
									}
									res, err = appendToAffected(
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										false,
										nil,
										includeSettings,
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
										atmosConfig,
										componentName,
										stackName,
										componentSection,
										res,
										affected,
										includeSpaceliftAdminStacks,
										currentStacks,
										includeSettings,
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
	atmosConfig schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentSection map[string]any,
	affectedList []schema.Affected,
	affected schema.Affected,
	includeSpaceliftAdminStacks bool,
	stacks map[string]any,
	includeSettings bool,
) ([]schema.Affected, error) {
	// If the affected component in the stack was already added to the result, don't add it again
	for _, v := range affectedList {
		if v.Component == affected.Component && v.Stack == affected.Stack && v.ComponentType == affected.ComponentType {
			return affectedList, nil
		}
	}

	settingsSection := map[string]any{}

	if i, ok2 := componentSection[cfg.SettingsSectionName]; ok2 {
		settingsSection = i.(map[string]any)

		if includeSettings {
			affected.Settings = settingsSection
		}
	}

	if affected.ComponentType == "terraform" {
		varSection := map[string]any{}

		if i, ok2 := componentSection[cfg.VarsSectionName]; ok2 {
			varSection = i.(map[string]any)
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
		spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
		if err != nil {
			return nil, err
		}
		affected.SpaceliftStack = spaceliftStackName

		// Affected Atlantis project
		atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
		if err != nil {
			return nil, err
		}
		affected.AtlantisProject = atlantisProjectName

		if includeSpaceliftAdminStacks {
			affectedList, err = addAffectedSpaceliftAdminStack(
				atmosConfig,
				affectedList,
				settingsSection,
				stacks,
				stackName,
				componentName,
				configAndStacksInfo,
				includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	// Check `component` section and add `ComponentPath` to the output
	affected.ComponentPath = BuildComponentPath(atmosConfig, componentSection, affected.ComponentType)
	affected.StackSlug = fmt.Sprintf("%s-%s", stackName, strings.Replace(componentName, "/", "-", -1))

	return append(affectedList, affected), nil
}

// isEqual compares a section of a component from the remote stacks with a section of a local component
func isEqual(
	remoteStacks map[string]any,
	localStackName string,
	componentType string,
	localComponentName string,
	localSection map[string]any,
	sectionName string,
) bool {
	if remoteStackSection, ok := remoteStacks[localStackName].(map[string]any); ok {
		if remoteComponentsSection, ok := remoteStackSection["components"].(map[string]any); ok {
			if remoteComponentTypeSection, ok := remoteComponentsSection[componentType].(map[string]any); ok {
				if remoteComponentSection, ok := remoteComponentTypeSection[localComponentName].(map[string]any); ok {
					if remoteSection, ok := remoteComponentSection[sectionName].(map[string]any); ok {
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
	atmosConfig schema.AtmosConfiguration,
	changedFiles []string,
) (bool, error) {
	var componentPath string

	switch componentType {
	case "terraform":
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)
	case "helmfile":
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, component)
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
	atmosConfig schema.AtmosConfiguration,
	changedFiles []string,
) (bool, error) {
	componentPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)

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

			modulePath := filepath.Join(filepath.Dir(moduleConfig.Pos.Filename), moduleConfig.Source)

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
	atmosConfig schema.AtmosConfiguration,
	affectedList []schema.Affected,
	settingsSection map[string]any,
	stacks map[string]any,
	currentStackName string,
	currentComponentName string,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	includeSettings bool,
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

	if atmosConfig.Stacks.NameTemplate != "" {
		adminStackContextPrefix, err = ProcessTmpl("spacelift-admin-stack-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return nil, err
		}
	} else {
		adminStackContextPrefix, err = cfg.GetContextPrefix(currentStackName, adminStackContext, GetStackNamePattern(atmosConfig), currentStackName)
		if err != nil {
			return nil, err
		}
	}

	var componentVarsSection map[string]any
	var componentSettingsSection map[string]any
	var componentSettingsSpaceliftSection map[string]any

	// Find the Spacelift admin stack that manages the current stack
	for stackName, stackSection := range stacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {

							if componentVarsSection, ok = componentSection["vars"].(map[string]any); !ok {
								return affectedList, nil
							}

							var context schema.Context
							err = mapstructure.Decode(componentVarsSection, &context)
							if err != nil {
								return nil, err
							}

							var contextPrefix string

							if atmosConfig.Stacks.NameTemplate != "" {
								contextPrefix, err = ProcessTmpl("spacelift-stack-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
								if err != nil {
									return nil, err
								}
							} else {
								contextPrefix, err = cfg.GetContextPrefix(stackName, context, GetStackNamePattern(atmosConfig), stackName)
								if err != nil {
									return nil, err
								}
							}

							if adminStackContext.Component == componentName && adminStackContextPrefix == contextPrefix {
								if componentSettingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
									return affectedList, nil
								}

								if componentSettingsSpaceliftSection, ok = componentSettingsSection["spacelift"].(map[string]any); !ok {
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
									atmosConfig,
									componentName,
									stackName,
									componentSection,
									affectedList,
									affectedSpaceliftAdminStack,
									false,
									nil,
									includeSettings,
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
func addDependentsToAffected(
	atmosConfig schema.AtmosConfiguration,
	affected *[]schema.Affected,
	includeSettings bool,
) error {
	for i := 0; i < len(*affected); i++ {
		a := &(*affected)[i]

		deps, err := ExecuteDescribeDependents(atmosConfig, a.Component, a.Stack, includeSettings)
		if err != nil {
			return err
		}

		if len(deps) > 0 {
			a.Dependents = deps
			err = addDependentsToDependents(atmosConfig, &deps, includeSettings)
			if err != nil {
				return err
			}
		} else {
			a.Dependents = []schema.Dependent{}
		}
	}

	processIncludedInDependencies(affected)
	return nil
}

// addDependentsToDependents recursively adds dependent components and stacks to each dependent component
func addDependentsToDependents(
	atmosConfig schema.AtmosConfiguration,
	dependents *[]schema.Dependent,
	includeSettings bool,
) error {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]

		deps, err := ExecuteDescribeDependents(atmosConfig, d.Component, d.Stack, includeSettings)
		if err != nil {
			return err
		}

		if len(deps) > 0 {
			d.Dependents = deps
			err = addDependentsToDependents(atmosConfig, &deps, includeSettings)
			if err != nil {
				return err
			}
		} else {
			d.Dependents = []schema.Dependent{}
		}
	}

	return nil
}

func processIncludedInDependencies(affected *[]schema.Affected) bool {
	for i := 0; i < len(*affected); i++ {
		a := &((*affected)[i])
		a.IncludedInDependents = processIncludedInDependenciesForAffected(affected, a.StackSlug, i)
	}
	return false
}

func processIncludedInDependenciesForAffected(affected *[]schema.Affected, stackSlug string, affectedIndex int) bool {
	for i := 0; i < len(*affected); i++ {
		if i == affectedIndex {
			continue
		}

		a := &((*affected)[i])

		if len(a.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForDependents(&a.Dependents, stackSlug)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

func processIncludedInDependenciesForDependents(dependents *[]schema.Dependent, stackSlug string) bool {
	for i := 0; i < len(*dependents); i++ {
		d := &((*dependents)[i])

		if d.StackSlug == stackSlug {
			return true
		}

		if len(d.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForDependents(&d.Dependents, stackSlug)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

// isComponentEnabled checks if a component is enabled based on its metadata
func isComponentEnabled(metadataSection map[string]any, componentName string, atmosConfig schema.AtmosConfiguration) bool {
	if enabled, ok := metadataSection["enabled"].(bool); ok {
		if !enabled {
			u.LogTrace(fmt.Sprintf("Skipping disabled component %s", componentName))
			return false
		}
	}
	return true
}
