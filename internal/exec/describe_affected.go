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

	if ref == "" && sha == "" {
		return fmt.Errorf("invalid flag. Either '--ref' or '--sha' is required. Both can be specified")
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
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

	if format == "yaml" {
		if file == "" {
			err = u.PrintAsYAML(affected)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsYAML(file, affected, 0644)
			if err != nil {
				return err
			}
		}
	} else if format == "json" {
		if file == "" {
			err = u.PrintAsJSON(affected)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsJSON(file, affected, 0644)
			if err != nil {
				return err
			}
		}
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

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			u.PrintError(err)
		}
	}(tempDir)

	// Clone the remote repo
	// https://git-scm.com/book/en/v2/Git-Internals-Git-References
	// https://github.com/go-git/go-git/tree/master/_examples
	// https://stackoverflow.com/questions/56810719/how-to-checkout-a-specific-sha-in-a-git-repo-using-golang
	// https://golang.hotexamples.com/examples/gopkg.in.src-d.go-git.v4.plumbing/-/ReferenceName/golang-referencename-function-examples.html

	u.PrintInfoVerbose(verbose, fmt.Sprintf("\nCloning repo '%s' into a temp dir '%s'", repoUrl, tempDir))

	cloneOptions := git.CloneOptions{
		URL:          repoUrl,
		NoCheckout:   false,
		SingleBranch: false,
	}

	if ref != "" {
		cloneOptions.ReferenceName = plumbing.ReferenceName(ref)
		u.PrintInfoVerbose(verbose, fmt.Sprintf("Git ref: %s", ref))
	}
	if verbose {
		cloneOptions.Progress = os.Stdout
	}

	repo, err := git.PlainClone(tempDir, false, &cloneOptions)
	if err != nil {
		return nil, err
	}

	// Check if a commit SHA was provided and checkout the repo at that commit SHA
	if sha != "" {
		u.PrintInfoVerbose(verbose, fmt.Sprintf("\nChecking out SHA: %s\n", sha))

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
							// Skip abstract components
							if metadataSection, ok := componentSection["metadata"].(map[any]any); ok {
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, varSection, "vars") {
									affected := cfg.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "terraform", componentName, envSection, "env") {
									affected := cfg.Affected{
										ComponentType: "terraform",
										Component:     componentName,
										Stack:         stackName,
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
							// Skip abstract components
							if metadataSection, ok := componentSection["metadata"].(map[any]any); ok {
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
							}
							// Check `vars` section
							if varSection, ok := componentSection["vars"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, varSection, "vars") {
									affected := cfg.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
									}
									res = append(res, affected)
									continue
								}
							}
							// Check `env` section
							if envSection, ok := componentSection["env"].(map[any]any); ok {
								if !isEqual(remoteStacks, stackName, "helmfile", componentName, envSection, "env") {
									affected := cfg.Affected{
										ComponentType: "helmfile",
										Component:     componentName,
										Stack:         stackName,
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
	sectionName string) bool {

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
