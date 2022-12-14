package exec

import (
	"fmt"
	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/tcnksm/go-gitconfig"
	"os"
	"path"
	"reflect"
	"strconv"
	"time"
)

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	// Process flags
	flags := cmd.Flags()

	ref, err := flags.GetString("ref")
	if err != nil {
		return err
	}

	sha, err := flags.GetString("sha")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'json' (default) and 'yaml'", format)
	}

	if format == "" {
		format = "json"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	verbose, err := flags.GetBool("verbose")
	if err != nil {
		return err
	}

	affected, err := ExecuteDescribeAffected(cliConfig, ref, sha, verbose)
	if err != nil {
		return err
	}

	err = printOrWriteToFile(format, file, affected)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeAffected processes stack configs and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffected(
	cliConfig cfg.CliConfiguration,
	ref string,
	sha string,
	verbose bool,
) ([]cfg.Affected, error) {

	// Get the origin URL of the current repo
	repoUrl, err := gitconfig.OriginURL()
	if err != nil {
		return nil, err
	}

	if repoUrl == "" {
		return nil, errors.New("the current repo is not a Git repository. Check that it was initialized and has '.git' folder")
	}

	// Create a temp dir to clone the remote repo to
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		return nil, err
	}

	defer removeTempDir(tempDir)

	// Clone the remote repo
	// https://git-scm.com/book/en/v2/Git-Internals-Git-References
	// https://git-scm.com/docs/git-show-ref
	// https://github.com/go-git/go-git/tree/master/_examples
	// https://stackoverflow.com/questions/56810719/how-to-checkout-a-specific-sha-in-a-git-repo-using-golang
	// https://golang.hotexamples.com/examples/gopkg.in.src-d.go-git.v4.plumbing/-/ReferenceName/golang-referencename-function-examples.html

	u.PrintInfoVerbose(verbose, fmt.Sprintf("\nCloning repo '%s' into the temp dir '%s'", repoUrl, tempDir))

	cloneOptions := git.CloneOptions{
		URL:          repoUrl,
		NoCheckout:   false,
		SingleBranch: false,
	}

	// If `ref` flag is not provided, it will clone the HEAD of the default branch
	if ref != "" {
		cloneOptions.ReferenceName = plumbing.ReferenceName(ref)
		u.PrintInfoVerbose(verbose, fmt.Sprintf("\nChecking out Git ref: %s\n", ref))
	} else {
		u.PrintInfoVerbose(verbose, "\nChecking out the HEAD of the default branch\n")
	}

	if verbose {
		cloneOptions.Progress = os.Stdout
	}

	repo, err := git.PlainClone(tempDir, false, &cloneOptions)
	if err != nil {
		return nil, err
	}

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	if ref != "" {
		u.PrintInfoVerbose(verbose, fmt.Sprintf("\nChecked out Git ref: %s\n", ref))
	} else {
		u.PrintInfoVerbose(verbose, fmt.Sprintf("\nChecked out Git ref: %s\n", head.Name()))
	}

	// Check if a commit SHA was provided and checkout the repo at that commit SHA
	if sha != "" {
		u.PrintInfoVerbose(verbose, fmt.Sprintf("\nChecking out commit SHA: %s\n", sha))

		w, err := repo.Worktree()
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

		u.PrintInfoVerbose(verbose, fmt.Sprintf("\nChecked out commit SHA: %s\n", sha))
	}

	currentStacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil)
	if err != nil {
		return nil, err
	}

	// Update paths to point to the temp dir
	cliConfig.StacksBaseAbsolutePath = path.Join(tempDir, cliConfig.BasePath, cliConfig.Stacks.BasePath)
	cliConfig.TerraformDirAbsolutePath = path.Join(tempDir, cliConfig.BasePath, cliConfig.Components.Terraform.BasePath)
	cliConfig.HelmfileDirAbsolutePath = path.Join(tempDir, cliConfig.BasePath, cliConfig.Components.Helmfile.BasePath)

	cliConfig.StackConfigFilesAbsolutePaths, err = u.JoinAbsolutePathWithPaths(
		path.Join(tempDir, cliConfig.BasePath, cliConfig.Stacks.BasePath),
		cliConfig.StackConfigFilesRelativePaths,
	)
	if err != nil {
		return nil, err
	}

	remoteStacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil)
	if err != nil {
		return nil, err
	}

	affected := findAffected(currentStacks, remoteStacks)
	return affected, nil
}

func findAffected(currentStacks map[string]any, remoteStacks map[string]any) []cfg.Affected {
	res := []cfg.Affected{}

	for stackName, stackSection := range currentStacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {
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
									affected := cfg.Affected{
										ComponentType:   "terraform",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "metadata",
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, varSection, "vars") {
									affected := cfg.Affected{
										ComponentType:   "terraform",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "vars",
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, envSection, "env") {
									affected := cfg.Affected{
										ComponentType:   "terraform",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "env",
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection["settings"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, settingsSection, "settings") {
									affected := cfg.Affected{
										ComponentType:   "terraform",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "settings",
									}
									res = append(res, affected)
									continue
								}
							}
						}
					}
				}
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
									affected := cfg.Affected{
										ComponentType:   "helmfile",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "metadata",
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, varSection, "vars") {
									affected := cfg.Affected{
										ComponentType:   "helmfile",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "vars",
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, envSection, "env") {
									affected := cfg.Affected{
										ComponentType:   "helmfile",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "env",
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `settings` section
							if settingsSection, ok := componentSection["settings"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, settingsSection, "settings") {
									affected := cfg.Affected{
										ComponentType:   "helmfile",
										Component:       componentName,
										Stack:           stackName,
										AffectedSection: "settings",
									}
									res = append(res, affected)
									continue
								}
							}
						}
					}
				}
			}
		}
	}

	return res
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
