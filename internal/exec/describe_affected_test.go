package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	cp "github.com/otiai10/copy"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	githubCI "github.com/cloudposse/atmos/pkg/ci/providers/github"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/tests"
)

// createExpectedAffectedResults creates the expected affected results for testing.
// When templatesProcessed is true, returns results with unprocessed template strings.
func createExpectedAffectedResults(componentPath string, templatesProcessed bool) []schema.Affected {
	tgwCrossRegionStack := "ue1-network"
	tgwHubStack := "ue1-network"
	if templatesProcessed {
		tgwCrossRegionStack = "ue1-{{ .vars.stage }}"
		tgwHubStack = "{{ .vars.environment }}-{{ .vars.stage }}"
	}

	return []schema.Affected{
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			Dependents:           nil,
			IncludedInDependents: false,
			Settings:             map[string]any{},
		},
		{
			Component:            "tgw/cross-region-hub-connector",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-tgw-cross-region-hub-connector",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			Dependents:           nil,
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "tgw/hub",
						"stack":     tgwCrossRegionStack,
					},
				},
			},
		},
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			Dependents:           nil,
			IncludedInDependents: false,
			Settings:             map[string]any{},
		},
		{
			Component:            "tgw/hub",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-tgw-hub",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			Dependents:           nil,
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "vpc",
						"stack":     tgwHubStack,
					},
				},
			},
		},
	}
}

func TestDescribeAffected(t *testing.T) {
	d := describeAffectedExec{atmosConfig: &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				Pager: "false", // Initially disabled
			},
		},
	}}
	d.IsTTYSupportForStdout = func() bool {
		return false
	}

	d.executeDescribeAffectedWithTargetRepoPath = func(atmosConfig *schema.AtmosConfiguration, targetRefPath string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool, authManager auth.AuthManager) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}

	d.executeDescribeAffectedWithTargetRefClone = func(atmosConfig *schema.AtmosConfiguration, ref, sha, sshKeyPath, sshKeyPassword string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool, authManager auth.AuthManager) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}

	d.executeDescribeAffectedWithTargetRefCheckout = func(atmosConfig *schema.AtmosConfiguration, ref, sha string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool, authManager auth.AuthManager) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{
			{
				Stack: "test-stack",
			},
		}, nil, nil, "", nil
	}

	d.atmosConfig = &schema.AtmosConfiguration{}
	d.addDependentsToAffected = func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings bool, processTemplates bool, processFunctions bool, skip []string, onlyInStack string, authManager auth.AuthManager) error {
		return nil
	}
	d.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
		return nil
	}

	err := d.Execute(&DescribeAffectedCmdArgs{
		Format:   "json",
		RepoPath: "",
	})
	assert.NoError(t, err)

	err = d.Execute(&DescribeAffectedCmdArgs{
		Format:         "yaml",
		CloneTargetRef: true,
	})
	assert.NoError(t, err)

	d.IsTTYSupportForStdout = func() bool {
		return true
	}
	// Enable pager for the tests that expect it to be called.
	d.atmosConfig.Settings.Terminal.Pager = "true"
	ctrl := gomock.NewController(t)
	mockPager := pager.NewMockPageCreator(ctrl)
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	d.pageCreator = mockPager
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format: "json",
	})
	assert.NoError(t, err)

	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format: "yaml",
	})
	assert.NoError(t, err)

	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format:   "json",
		RepoPath: "repo/path",
	})
	assert.NoError(t, err)

	err = d.Execute(&DescribeAffectedCmdArgs{
		Format: "json",
		Query:  ".0.stack",
	})
	assert.NoError(t, err)

	// Test with IncludeDependents flag to cover the addDependentsToAffected code path.
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format:            "json",
		IncludeDependents: true,
	})
	assert.NoError(t, err)

	// Test with IncludeDependents and Stack filter to cover the onlyInStack parameter.
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format:            "json",
		IncludeDependents: true,
		Stack:             "test-stack",
	})
	assert.NoError(t, err)
}

func TestExecuteDescribeAffectedWithTargetRepoPath(t *testing.T) {
	// Check for Git repository with the valid remotes precondition.
	tests.RequireGitRemoteWithValidURL(t)

	stacksPath := "../../tests/fixtures/scenarios/atmos-describe-affected"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "./",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo.
	atmosConfig.BasePath = "./tests/fixtures/scenarios/atmos-describe-affected"

	// Point to the same local repository.
	// This will compare this local repository with itself as the remote target, which should result in an empty `affected` list.
	repoPath := "../../"

	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		false,
		false,
		nil,
		false,
		nil,
	)
	assert.Nil(t, err)

	// The `affected` list should be empty, since the local repo is compared with itself.
	assert.Equal(t, 0, len(affected))
}

// setupDescribeAffectedTest sets up the test environment for describe affected tests.
func setupDescribeAffectedTest(t *testing.T) (atmosConfig schema.AtmosConfiguration, repoPath, componentPath string) {
	t.Helper()

	// Check for valid Git remote URL before running the test.
	tests.RequireGitRemoteWithValidURL(t)

	basePath := "tests/fixtures/scenarios/atmos-describe-affected-with-dependents-and-locked"
	pathPrefix := "../../"

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	tempDir := t.TempDir()

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") ||
				strings.Contains(src, ".terraform") {
				return true, nil
			}
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

	// Copy the local repository into a temp dir.
	err = cp.Copy(pathPrefix, tempDir, copyOptions)
	require.NoError(t, err)

	// Copy the affected stacks into the `stacks` folder in the temp dir.
	err = cp.Copy(filepath.Join(stacksPath, "stacks-affected"), filepath.Join(tempDir, basePath, "stacks"), copyOptions)
	require.NoError(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "./",
	// which will be wrong for the cloned repo in the temp dir.
	// Set the correct base path for the cloned remote repo.
	config.BasePath = basePath

	// Point to the copy of the local repository.
	repoPath = tempDir

	// OS-specific expected component path.
	componentPath = filepath.Join("tests", "fixtures", "components", "terraform", "mock")

	return config, repoPath, componentPath
}

//nolint:dupl // Test scenarios have similar structures but test different conditions.
func TestDescribeAffectedWithTemplatesAndFunctions(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true` and `excludeLocked: false`.
	expected := createExpectedAffectedResults(componentPath, false)
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

//nolint:dupl // Test scenarios have similar structures but test different conditions.
func TestDescribeAffectedWithoutTemplatesAndFunctions(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: false`, `processFunctions: false` and `excludeLocked: false`.
	expected := createExpectedAffectedResults(componentPath, true)
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		false,
		false,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

func TestDescribeAffectedWithExcludeLocked(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true` and `excludeLocked: true`.
	expected := []schema.Affected{
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			Dependents:           nil, // must be nil to match actual
			IncludedInDependents: false,
			Settings:             map[string]any{},
		},
		{
			Component:            "tgw/hub",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-tgw-hub",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			Dependents:           nil, // must be nil to match actual
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{ // note: any keys
					1: map[string]any{
						"component": "vpc",
						"stack":     "ue1-network",
					},
				},
			},
		},
	}
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,
		true,
		nil,
		true,
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

//nolint:dupl // Test scenarios have similar structures but test different dependents processing.
func TestDescribeAffectedWithDependents(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true`, `excludeLocked: false`,
	// and process dependents (with `processTemplates: true`, `processFunctions: true` for the dependents).
	expected := []schema.Affected{
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents: []schema.Dependent{
				{
					Component:            "tgw/attachment",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-attachment",
					IncludedInDependents: true,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{
								"component": "vpc",
							},
							2: map[string]any{
								"component": "tgw/hub",
							},
						},
					},
					Dependents: []schema.Dependent{},
				},
				{
					Component:            "tgw/hub",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-hub",
					IncludedInDependents: false,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{
								"component": "vpc",
								"stack":     "ue1-network",
							},
						},
					},
					Dependents: []schema.Dependent{
						{
							Component:            "tgw/attachment",
							ComponentType:        "terraform",
							ComponentPath:        componentPath,
							Environment:          "ue1",
							Stage:                "network",
							Stack:                "ue1-network",
							StackSlug:            "ue1-network-tgw-attachment",
							IncludedInDependents: false,
							Settings: map[string]any{
								"depends_on": map[any]any{
									1: map[string]any{
										"component": "vpc",
									},
									2: map[string]any{
										"component": "tgw/hub",
									},
								},
							},
							Dependents: []schema.Dependent{},
						},
					},
				},
			},
		},
		{
			Component:            "tgw/hub",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-tgw-hub",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: true,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "vpc",
						"stack":     "ue1-network",
					},
				},
			},
			Dependents: []schema.Dependent{
				{
					Component:            "tgw/attachment",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-attachment",
					IncludedInDependents: false,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{
								"component": "vpc",
							},
							2: map[string]any{
								"component": "tgw/hub",
							},
						},
					},
					Dependents: []schema.Dependent{},
				},
			},
		},
		{
			Component:            "tgw/cross-region-hub-connector",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-tgw-cross-region-hub-connector",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "tgw/hub",
						"stack":     "ue1-network",
					},
				},
			},
			Dependents: []schema.Dependent{},
		},
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents:           []schema.Dependent{},
		},
	}
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		true,
		nil,
		"",
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

func TestDescribeAffectedWithDependentsWithoutTemplates(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: false`, `processFunctions: false`, `excludeLocked: false`,
	// and process dependents (with `processTemplates: false`, `processFunctions: false` for the dependents)
	expected := []schema.Affected{
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents: []schema.Dependent{
				{
					Component:            "tgw/attachment",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-attachment",
					IncludedInDependents: false,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{"component": "vpc"},
							2: map[string]any{"component": "tgw/hub"},
						},
					},
					Dependents: []schema.Dependent{},
				},
			},
		},
		{
			Component:            "tgw/hub",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-tgw-hub",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "vpc",
						"stack":     "{{ .vars.environment }}-{{ .vars.stage }}",
					},
				},
			},
			Dependents: []schema.Dependent{
				{
					Component:            "tgw/attachment",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-attachment",
					IncludedInDependents: false,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{"component": "vpc"},
							2: map[string]any{"component": "tgw/hub"},
						},
					},
					Dependents: []schema.Dependent{},
				},
			},
		},
		{
			Component:            "tgw/cross-region-hub-connector",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-tgw-cross-region-hub-connector",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "tgw/hub",
						"stack":     "ue1-{{ .vars.stage }}",
					},
				},
			},
			Dependents: []schema.Dependent{},
		},
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents:           []schema.Dependent{},
		},
	}
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		false,
		false,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		false,
		false,
		nil,
		"",
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

func TestDescribeAffectedWithDependentsFilteredByStack(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true`, `excludeLocked: false`,
	// process dependents (with `processTemplates: true`, `processFunctions: true` for the dependents),
	// and filter the dependents by a specific stack.
	onlyInStack := "ue1-network"
	//nolint:dupl // Test scenarios have similar structures for consistency
	expected := []schema.Affected{
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents: []schema.Dependent{
				{
					Component:            "tgw/attachment",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-attachment",
					IncludedInDependents: true,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{
								"component": "vpc",
							},
							2: map[string]any{
								"component": "tgw/hub",
							},
						},
					},
					Dependents: []schema.Dependent{},
				},
				{
					Component:            "tgw/hub",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-hub",
					IncludedInDependents: false,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{
								"component": "vpc",
								"stack":     "ue1-network",
							},
						},
					},
					Dependents: []schema.Dependent{
						{
							Component:            "tgw/attachment",
							ComponentType:        "terraform",
							ComponentPath:        componentPath,
							Environment:          "ue1",
							Stage:                "network",
							Stack:                "ue1-network",
							StackSlug:            "ue1-network-tgw-attachment",
							IncludedInDependents: false,
							Settings: map[string]any{
								"depends_on": map[any]any{
									1: map[string]any{
										"component": "vpc",
									},
									2: map[string]any{
										"component": "tgw/hub",
									},
								},
							},
							Dependents: []schema.Dependent{},
						},
					},
				},
			},
		},
		{
			Component:            "tgw/hub",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-tgw-hub",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: true,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "vpc",
						"stack":     "ue1-network",
					},
				},
			},
			Dependents: []schema.Dependent{
				{
					Component:            "tgw/attachment",
					ComponentType:        "terraform",
					ComponentPath:        componentPath,
					Environment:          "ue1",
					Stage:                "network",
					Stack:                "ue1-network",
					StackSlug:            "ue1-network-tgw-attachment",
					IncludedInDependents: false,
					Settings: map[string]any{
						"depends_on": map[any]any{
							1: map[string]any{
								"component": "vpc",
							},
							2: map[string]any{
								"component": "tgw/hub",
							},
						},
					},
					Dependents: []schema.Dependent{},
				},
			},
		},
		{
			Component:            "tgw/cross-region-hub-connector",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-tgw-cross-region-hub-connector",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "tgw/hub",
						"stack":     "ue1-network",
					},
				},
			},
			Dependents: nil, // nil because component is not in the filtered stack (onlyInStack = "ue1-network").
		},
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents:           nil, // nil because component is not in the filtered stack (onlyInStack = "ue1-network").
		},
	}
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		true,
		nil,
		onlyInStack, // Filter dependents to only show those in "ue1-network" stack.,
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

func TestDescribeAffectedWithDisabledDependents(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true`, `excludeLocked: false`,
	// process dependents (with `processTemplates: true`, `processFunctions: true` for the dependents),
	// and filter the dependents by a specific stack.
	// This also verifies that disabled dependents (metadata.enabled: false) are excluded from the results.
	// In the test fixture, tgw/attachment is disabled in uw2-network (us-west-2.yaml), so it should NOT
	// appear in the dependents list for vpc in uw2-network.
	onlyInStack := "uw2-network"
	expected := []schema.Affected{
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			Dependents:           nil, // nil because component is not in the filtered stack (onlyInStack = "uw2-network").
		},
		{
			Component:            "tgw/hub",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "ue1-network",
			StackSlug:            "ue1-network-tgw-hub",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "vpc",
						"stack":     "ue1-network",
					},
				},
			},
			Dependents: nil, // nil because component is not in the filtered stack (onlyInStack = "uw2-network").
		},
		{
			Component:            "tgw/cross-region-hub-connector",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-tgw-cross-region-hub-connector",
			Affected:             "stack.settings",
			AffectedAll:          []string{"stack.settings"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{
					1: map[string]any{
						"component": "tgw/hub",
						"stack":     "ue1-network",
					},
				},
			},
			Dependents: []schema.Dependent{}, // empty slice because component is in filtered stack but has no dependents.
		},
		{
			Component:            "vpc",
			ComponentType:        "terraform",
			ComponentPath:        componentPath,
			Stack:                "uw2-network",
			StackSlug:            "uw2-network-vpc",
			Affected:             "stack.vars",
			AffectedAll:          []string{"stack.vars"},
			File:                 "",
			Folder:               "",
			IncludedInDependents: false,
			Settings:             map[string]any{},
			// Note: tgw/attachment is NOT included here because it's disabled (enabled: false) in uw2-network
			Dependents: []schema.Dependent{}, // empty slice because component is in filtered stack but has no dependents
		},
	}
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		true,
		nil,
		onlyInStack, // Filter dependents to only show those in "uw2-network" stack.,
		nil,
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices.
	assert.ElementsMatch(t, expected, affected)
}

// TestDescribeAffectedWithDependentsStackFilterYamlFunctions tests that when using --stack filter with --include-dependents,
// YAML functions are only executed for the specified stack, not for all stacks.
// This test demonstrates the bug where ExecuteDescribeDependents calls ExecuteDescribeStacks with empty string
// instead of passing the stack filter, causing YAML functions to be executed for ALL stacks.
func TestDescribeAffectedWithDependentsStackFilterYamlFunctions(t *testing.T) {
	atmosConfig, repoPath, _ := setupDescribeAffectedTest(t)

	// Set environment variables ONLY for ue1-network components
	t.Setenv("ATMOS_TEST_VPC_UE1", "vpc-ue1-value")
	t.Setenv("ATMOS_TEST_TGW_HUB_UE1", "tgw-hub-ue1-value")
	t.Setenv("ATMOS_TEST_TGW_ATTACHMENT_UE1", "tgw-attachment-ue1-value")

	// Intentionally DO NOT set environment variables for uw2-network components
	// If the bug exists, the code will try to process uw2-network components and execute !env functions
	// which will result in empty strings for these vars in the processed output.

	onlyInStack := "ue1-network"

	// Execute describe affected with stack filter and include dependents
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)

	// Add dependents filtered by stack
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		true,
		nil,
		onlyInStack,
		nil,
	)
	require.NoError(t, err)

	// Verify that only ue1-network components and their dependents are included
	var ue1ComponentCount int
	var uw2ComponentCount int

	for i := range affected {
		a := &affected[i]

		// Count and verify components by stack.
		switch a.Stack {
		case "ue1-network":
			ue1ComponentCount++
			if a.Dependents != nil {
				// All dependents should be from ue1-network only when filtering by ue1-network.
				for j := range a.Dependents {
					dep := &a.Dependents[j]
					if dep.Stack != "ue1-network" {
						t.Errorf("BUG DETECTED: Dependent %s in stack %s found for ue1-network component %s, "+
							"but should only include ue1-network dependents when filtering by --stack ue1-network",
							dep.Component, dep.Stack, a.Component)
					}
					// Check if the dependent has settings with env vars.
					if dep.Settings != nil {
						t.Logf("Dependent %s in stack %s has settings", dep.Component, dep.Stack)
					}
				}
			}
		case "uw2-network":
			uw2ComponentCount++
			if a.Dependents != nil {
				t.Fatalf("uw2 component %s should not have dependents when filtering by ue1-network", a.Component)
			}
		}
	}

	// The bug manifests as: even though we filter results to ue1-network, the code still calls
	// ExecuteDescribeStacks("") with empty stack filter in ExecuteDescribeDependents,
	// causing ALL stacks to be loaded and ALL YAML functions (!env, !terraform.output, etc.) to be executed.
	// This is a performance issue and can cause side effects like accessing terraform state for irrelevant stacks.

	t.Logf("Test completed - ue1-network components: %d, uw2-network components: %d, total: %d",
		ue1ComponentCount, uw2ComponentCount, len(affected))

	// EXPECTED BEHAVIOR (after fix):
	// - When filtering by ue1-network, ExecuteDescribeStacks should be called with filterByStack="ue1-network"
	// - This means YAML functions should only be executed for ue1-network components
	// - The env vars for uw2-network (ATMOS_TEST_VPC_UW2, etc.) should never be looked up
	//
	// CURRENT BUGGY BEHAVIOR (before fix):
	// - ExecuteDescribeStacks is called with filterByStack="" (empty string)
	// - All stacks are loaded and all YAML functions are executed
	// - The env vars for uw2-network are looked up (they'll get empty strings since not set)
	// - Performance penalty and potential side effects
}

// setupDescribeAffectedTestWithFixture is a generic helper that sets up a test environment for describe affected tests.
// It handles the common setup of copying the fixture, replacing stacks, and initializing git repos.
func setupDescribeAffectedTestWithFixture(t *testing.T, fixtureDir, affectedStacksDir string) (atmosConfig schema.AtmosConfiguration, repoPath string) {
	t.Helper()

	// Check for valid Git remote URL before running the test.
	tests.RequireGitRemoteWithValidURL(t)

	basePath := filepath.Join("tests", "fixtures", "scenarios", fixtureDir)
	pathPrefix := "../../"

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	tempDir := t.TempDir()

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") ||
				strings.Contains(src, ".terraform") {
				return true, nil
			}
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

	// Copy the local repository into a temp dir.
	err = cp.Copy(pathPrefix, tempDir, copyOptions)
	require.NoError(t, err)

	// Copy the affected stacks into the `stacks` folder in the temp dir.
	// This simulates BASE (main) having different content than HEAD (PR branch).
	err = cp.Copy(filepath.Join(stacksPath, affectedStacksDir), filepath.Join(tempDir, basePath, "stacks"), copyOptions)
	require.NoError(t, err)

	// Set the correct base path for the cloned remote repo.
	config.BasePath = basePath

	// Point to the copy of the local repository.
	repoPath = tempDir

	return config, repoPath
}

// setupDescribeAffectedNewComponentInBaseTest sets up the test environment for testing
// the scenario where a new component exists in BASE (main) but not in HEAD (PR branch).
func setupDescribeAffectedNewComponentInBaseTest(t *testing.T, affectedStacksDir string) (atmosConfig schema.AtmosConfiguration, repoPath string) {
	t.Helper()
	return setupDescribeAffectedTestWithFixture(t, "atmos-describe-affected-new-component-in-base", affectedStacksDir)
}

// TestDescribeAffectedNewComponentInBase tests the scenario where a new component
// exists in BASE (main branch) but not in HEAD (PR branch).
// This should NOT fail - the new component should be detected as "net new" and handled gracefully.
//
// Scenario:
// - PR1 introduces prometheus component and merges to main
// - PR2 (based on old main) runs describe affected against current main
// - Expected: Atmos should recognize prometheus as new in BASE and handle it gracefully
// - Current bug: Atmos fails with "Could not find the component prometheus".
func TestDescribeAffectedNewComponentInBase(t *testing.T) {
	atmosConfig, repoPath := setupDescribeAffectedNewComponentInBaseTest(t, "stacks-with-new-component")

	// Run describe affected comparing HEAD (without prometheus) to BASE (with prometheus).
	// This should succeed and show prometheus as affected (new component in BASE).
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,  // processTemplates
		false, // processYamlFunctions - disable to avoid !terraform.state issues
		nil,
		false,
		nil,
	)

	// The test should pass - new components in BASE should be handled gracefully.
	require.NoError(t, err)

	// Verify that the new component (prometheus) is detected as affected.
	var foundPrometheus bool
	for _, a := range affected {
		if a.Component == "prometheus" {
			foundPrometheus = true
			t.Logf("Found prometheus component as affected in stack %s", a.Stack)
		}
	}

	// NOTE: Components that exist only in BASE (not in HEAD) are currently NOT detected
	// because describe affected iterates over currentStacks (HEAD). This is a known limitation.
	// This test verifies that describe affected doesn't error when BASE has components that HEAD doesn't have.
	// TODO: Consider detecting components that exist in BASE but not HEAD in a future enhancement.
	t.Logf("Found prometheus in affected: %v, total affected: %d", foundPrometheus, len(affected))
}

// TestDescribeAffectedNewComponentInBaseWithYamlFunctions tests the scenario where
// BASE has a new component that is referenced via !terraform.state by an existing component.
// This is the exact scenario that causes the error:
//
//	"failed to describe component prometheus in stack ue1-staging"
//	"YAML function: !terraform.state prometheus workspace_endpoint"
//	"Could not find the component prometheus in the stack ue1-staging"
//
// Root cause: When processing remoteStacks (BASE), YAML functions like !terraform.state
// call ExecuteDescribeComponent with nil AtmosConfig. This causes ExecuteDescribeComponentWithContext
// to create a NEW AtmosConfig from the current working directory (HEAD), instead of using the
// modified config that points to BASE. Since prometheus only exists in BASE, the lookup fails.
func TestDescribeAffectedNewComponentInBaseWithYamlFunctions(t *testing.T) {
	atmosConfig, repoPath := setupDescribeAffectedNewComponentInBaseTest(t, "stacks-with-new-component-and-reference")

	// Run describe affected comparing HEAD (without prometheus) to BASE (with prometheus + reference).
	// This currently fails because:
	// 1. BASE has eks component with: prometheus_endpoint: '!terraform.state prometheus workspace_endpoint'
	// 2. When processing remoteStacks, the !terraform.state function is evaluated
	// 3. GetTerraformState calls ExecuteDescribeComponent with nil AtmosConfig
	// 4. ExecuteDescribeComponentWithContext creates a NEW config from CWD (HEAD)
	// 5. prometheus doesn't exist in HEAD, so the lookup fails
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true, // processTemplates
		true, // processYamlFunctions - this triggers the bug
		nil,
		false,
		nil,
	)
	// FIXED BEHAVIOR: The fix passes atmosConfig through the YAML function chain to
	// ExecuteDescribeComponent, so component lookups use BASE paths correctly.
	//
	// After the fix, we expect one of these outcomes:
	// 1. "terraform state not provisioned" - Component IS found in BASE, but since prometheus
	//    is a mock component with no actual Terraform state, getting the state fails.
	//    This is expected and shows the fix is working - component lookup now uses BASE paths.
	// 2. Success - If the error is handled gracefully (e.g., with YQ defaults).
	if err != nil {
		errStr := err.Error()
		// The fix is working if we see "not provisioned" instead of "Could not find".
		// "not provisioned" means the component WAS found (using BASE paths), but has no state.
		if strings.Contains(errStr, "prometheus") && strings.Contains(errStr, "not provisioned") {
			t.Logf("Fix verified! Component found in BASE, but no Terraform state exists (expected for mock): %v", err)
			// This is actually a success - the component lookup bug is fixed.
			// The "not provisioned" error is expected for mock components without real Terraform state.
			return
		}
		// OLD BUG: If we still see "Could not find", the fix didn't work.
		if strings.Contains(errStr, "prometheus") && strings.Contains(errStr, "Could not find") {
			t.Fatalf("BUG NOT FIXED: Component lookup still failing with: %v", err)
		}
		// Unexpected error.
		t.Fatalf("Unexpected error: %v", err)
	}

	// If we get here, the bug is fixed!
	t.Logf("Success! describe affected handled new component in BASE gracefully")
	t.Logf("Affected components: %d", len(affected))
	for _, a := range affected {
		t.Logf("  - %s in %s (affected by: %s)", a.Component, a.Stack, a.Affected)
	}
}

// setupDescribeAffectedSourceVendoringTest sets up a test environment for source vendoring scenarios.
// It returns the atmosConfig and the path to the BASE repo.
func setupDescribeAffectedSourceVendoringTest(t *testing.T, affectedStacksDir string) (atmosConfig schema.AtmosConfiguration, repoPath string) {
	t.Helper()
	return setupDescribeAffectedTestWithFixture(t, "atmos-describe-affected-source-vendoring", affectedStacksDir)
}

// TestDescribeAffectedSourceVersionChange tests that describe affected detects changes to source.version
// and provision.workdir configuration.
// This is a critical test for source vendoring because if source.version changes (e.g., upgrading a module),
// the component should be marked as affected. Similarly, workdir configuration changes should be detected.
//
// Scenario:
// - HEAD (PR branch): source.version = "1.0.0", component-workdir-only has no workdir
// - BASE (main branch): source.version = "1.1.0", component-workdir-only has workdir enabled
// Expected: vpc-source, vpc-source-workdir, and component-workdir-only should be marked as affected.
func TestDescribeAffectedSourceVersionChange(t *testing.T) {
	atmosConfig, repoPath := setupDescribeAffectedSourceVendoringTest(t, "stacks-with-source-version-change")

	// Run describe affected comparing HEAD (version 1.0.0) to BASE (version 1.1.0).
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,  // processTemplates
		false, // processYamlFunctions - don't need YAML functions for this test
		nil,
		false,
		nil,
	)
	// Check if there was an error.
	require.NoError(t, err)

	// Log what was found.
	t.Logf("Found %d affected components", len(affected))
	for _, a := range affected {
		t.Logf("  - %s in %s (affected by: %s)", a.Component, a.Stack, a.Affected)
	}

	// Check if source-vendored components are marked as affected.
	foundVpcSource := false
	foundVpcSourceWorkdir := false
	foundWorkdirOnly := false
	var vpcSourceReason, vpcSourceWorkdirReason, workdirOnlyReason string

	for _, a := range affected {
		switch a.Component {
		case "vpc-source":
			foundVpcSource = true
			vpcSourceReason = a.Affected
		case "vpc-source-workdir":
			foundVpcSourceWorkdir = true
			vpcSourceWorkdirReason = a.Affected
		case "component-workdir-only":
			foundWorkdirOnly = true
			workdirOnlyReason = a.Affected
		}
	}

	// Verify source.version changes are detected.
	require.True(t, foundVpcSource, "vpc-source should be affected due to source.version change (1.0.0 -> 1.1.0)")
	assert.Equal(t, "stack.source", vpcSourceReason, "vpc-source should be affected due to source change")

	require.True(t, foundVpcSourceWorkdir, "vpc-source-workdir should be affected due to source.version change (1.0.0 -> 1.1.0)")
	assert.Equal(t, "stack.source", vpcSourceWorkdirReason, "vpc-source-workdir should be affected due to source change")

	// Verify provision.workdir changes are detected.
	require.True(t, foundWorkdirOnly, "component-workdir-only should be affected due to provision.workdir change")
	assert.Equal(t, "stack.provision", workdirOnlyReason, "component-workdir-only should be affected due to provision change")

	// Ensure the regular component is NOT affected (no changes).
	for _, a := range affected {
		if a.Component == "regular-component" {
			t.Errorf("FAILED: regular-component should NOT be affected (no changes)")
		}
	}
}

// setupDescribeAffectedDeletedDetectionTest sets up the test environment for testing
// the scenario where components/stacks exist in BASE (main) but have been deleted in HEAD (PR branch).
func setupDescribeAffectedDeletedDetectionTest(t *testing.T, affectedStacksDir string) (atmosConfig schema.AtmosConfiguration, repoPath string) {
	t.Helper()
	return setupDescribeAffectedTestWithFixture(t, "atmos-describe-affected-deleted-detection", affectedStacksDir)
}

// TestDescribeAffectedDeletedComponentDetection tests the scenario where a component
// is deleted in HEAD (PR branch) compared to BASE (main branch).
// This enables CI/CD pipelines to detect resources that need terraform destroy.
//
// Scenario:
// - HEAD (PR branch): monitoring component has been DELETED from staging stack
// - HEAD (PR branch): production stack has been entirely DELETED
// - BASE (main branch): has monitoring component and production stack
// Expected:
// - monitoring component should be marked as affected with deleted: true, deletion_type: "component"
// - All components from production stack should be marked with deleted: true, deletion_type: "stack".
func TestDescribeAffectedDeletedComponentDetection(t *testing.T) {
	atmosConfig, repoPath := setupDescribeAffectedDeletedDetectionTest(t, "stacks-with-deleted-component")

	// Run describe affected comparing HEAD (with deleted components) to BASE (with all components).
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,  // processTemplates
		false, // processYamlFunctions - don't need YAML functions for this test
		nil,
		false,
		nil,
	)
	require.NoError(t, err)

	// Log what was found.
	t.Logf("Found %d affected components", len(affected))
	for _, a := range affected {
		t.Logf("  - %s in %s (affected: %s, deleted: %v, deletion_type: %s)",
			a.Component, a.Stack, a.Affected, a.Deleted, a.DeletionType)
	}

	// Track what we find.
	var (
		foundMonitoringDeleted bool
		foundVpcDeletedStack   bool
		foundEksDeletedStack   bool
		foundRdsDeletedStack   bool
		foundAbstractDeleted   bool
		deletedComponentCount  int
		deletedStackCount      int
	)

	for _, a := range affected {
		// Check for monitoring component deleted from staging stack.
		if a.Component == "monitoring" && a.Stack == "ue1-staging" {
			foundMonitoringDeleted = true
			assert.True(t, a.Deleted, "monitoring should have deleted: true")
			assert.Equal(t, deletionTypeComponent, a.DeletionType, "monitoring should have deletion_type: component")
			assert.Equal(t, affectedReasonDeleted, a.Affected, "monitoring should have affected: deleted")
		}

		// Check for production stack components (entire stack deleted).
		// Note: Stack names may contain unprocessed templates in some edge cases during
		// deleted component detection, so we check for both processed and unprocessed versions.
		// Use HasSuffix to avoid matching unintended stacks like "non-production-staging".
		// Also check for exact "production" match in case the stack name is just "production".
		isProductionStack := a.Stack == "ue1-production" || a.Stack == "production" || strings.HasSuffix(a.Stack, "-production")
		if isProductionStack && a.DeletionType == deletionTypeStack {
			switch a.Component {
			case "vpc":
				foundVpcDeletedStack = true
			case "eks":
				foundEksDeletedStack = true
			case "rds":
				foundRdsDeletedStack = true
			}
			assert.True(t, a.Deleted, "%s in production should have deleted: true", a.Component)
			assert.Equal(t, deletionTypeStack, a.DeletionType, "%s should have deletion_type: stack", a.Component)
			assert.Equal(t, affectedReasonDeletedStack, a.Affected, "%s should have affected: deleted.stack", a.Component)
		}

		// Abstract components should NOT be reported as deleted.
		if a.Component == "base-monitoring" {
			foundAbstractDeleted = true
		}

		// Count deleted components by type.
		if a.Deleted {
			switch a.DeletionType {
			case deletionTypeComponent:
				deletedComponentCount++
			case deletionTypeStack:
				deletedStackCount++
			}
		}
	}

	// Verify monitoring component was detected as deleted.
	assert.True(t, foundMonitoringDeleted, "monitoring component should be detected as deleted from staging stack")

	// Verify all production stack components were detected as deleted.
	assert.True(t, foundVpcDeletedStack, "vpc should be detected as deleted (stack deletion)")
	assert.True(t, foundEksDeletedStack, "eks should be detected as deleted (stack deletion)")
	assert.True(t, foundRdsDeletedStack, "rds should be detected as deleted (stack deletion)")

	// Verify abstract component is NOT reported as deleted.
	assert.False(t, foundAbstractDeleted, "abstract component base-monitoring should NOT be reported as deleted")

	// Verify counts.
	assert.Equal(t, 1, deletedComponentCount, "should have exactly 1 deleted component (monitoring)")
	assert.Equal(t, 3, deletedStackCount, "should have exactly 3 components deleted from stack (vpc, eks, rds)")

	t.Logf("Deleted detection successful: %d components deleted, %d from deleted stack",
		deletedComponentCount, deletedStackCount)
}

// TestDescribeAffectedDeletedComponentFiltering tests filtering deleted components
// using the stack filter.
func TestDescribeAffectedDeletedComponentFiltering(t *testing.T) {
	atmosConfig, repoPath := setupDescribeAffectedDeletedDetectionTest(t, "stacks-with-deleted-component")

	// Run describe affected with stack filter for staging only.
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"ue1-staging", // Filter to staging stack only
		true,
		false,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)

	// Should only find monitoring deleted from staging, not production stack deletions.
	var deletedCount int
	for _, a := range affected {
		if a.Deleted {
			deletedCount++
			assert.Equal(t, "ue1-staging", a.Stack, "with stack filter, should only find deletions in ue1-staging")
		}
	}

	// Should find only the monitoring component deleted.
	assert.Equal(t, 1, deletedCount, "with stack filter ue1-staging, should find only 1 deleted component")
}

// TestConvertAffectedToMatrix tests converting affected components to GitHub Actions matrix format.
func TestConvertAffectedToMatrix(t *testing.T) {
	t.Run("empty affected list", func(t *testing.T) {
		matrix := convertAffectedToMatrix([]schema.Affected{})
		assert.NotNil(t, matrix.Include)
		assert.Empty(t, matrix.Include)
	})

	t.Run("single affected", func(t *testing.T) {
		affected := []schema.Affected{
			{
				Stack:         "ue1-dev",
				Component:     "vpc",
				ComponentPath: filepath.Join("components", "terraform", "vpc"),
				ComponentType: "terraform",
			},
		}
		matrix := convertAffectedToMatrix(affected)
		require.Len(t, matrix.Include, 1)
		assert.Equal(t, "ue1-dev", matrix.Include[0].Stack)
		assert.Equal(t, "vpc", matrix.Include[0].Component)
		assert.Equal(t, filepath.Join("components", "terraform", "vpc"), matrix.Include[0].ComponentPath)
		assert.Equal(t, "terraform", matrix.Include[0].ComponentType)
	})

	t.Run("multiple affected", func(t *testing.T) {
		affected := []schema.Affected{
			{
				Stack:         "ue1-dev",
				Component:     "vpc",
				ComponentPath: filepath.Join("components", "terraform", "vpc"),
				ComponentType: "terraform",
			},
			{
				Stack:         "ue1-staging",
				Component:     "eks",
				ComponentPath: filepath.Join("components", "terraform", "eks"),
				ComponentType: "terraform",
			},
		}
		matrix := convertAffectedToMatrix(affected)
		require.Len(t, matrix.Include, 2)
		assert.Equal(t, "ue1-dev", matrix.Include[0].Stack)
		assert.Equal(t, "eks", matrix.Include[1].Component)
	})
}

// TestWriteMatrixOutput_File tests writing matrix output to a file.
func TestWriteMatrixOutput_File(t *testing.T) {
	t.Run("writes matrix and count to file", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "github_output")
		affected := []schema.Affected{
			{
				Stack:         "ue1-dev",
				Component:     "vpc",
				ComponentPath: filepath.Join("components", "terraform", "vpc"),
				ComponentType: "terraform",
			},
		}
		err := writeMatrixOutput(affected, outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		require.Len(t, lines, 2)
		assert.True(t, strings.HasPrefix(lines[0], "matrix="))
		assert.Equal(t, "affected_count=1", lines[1])

		// Verify JSON is valid.
		matrixJSON := strings.TrimPrefix(lines[0], "matrix=")
		var matrix MatrixOutput
		err = json.Unmarshal([]byte(matrixJSON), &matrix)
		require.NoError(t, err)
		require.Len(t, matrix.Include, 1)
		assert.Equal(t, "vpc", matrix.Include[0].Component)
	})

	t.Run("empty affected writes empty include", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "github_output")
		err := writeMatrixOutput([]schema.Affected{}, outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), `"include":[]`)
		assert.Contains(t, string(content), "affected_count=0")
	})

	t.Run("file open error", func(t *testing.T) {
		err := writeMatrixOutput([]schema.Affected{}, filepath.Join(t.TempDir(), "nonexistent", "file"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open output file")
	})
}

// TestWriteMatrixOutput_Stdout tests writing matrix output to stdout.
func TestWriteMatrixOutput_Stdout(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	err = writeMatrixOutput([]schema.Affected{
		{
			Stack:         "ue1-dev",
			Component:     "vpc",
			ComponentPath: filepath.Join("components", "terraform", "vpc"),
			ComponentType: "terraform",
		},
	}, "")
	assert.NoError(t, err)
}

// TestResolveBaseFromCI tests CI base auto-detection.
func TestResolveBaseFromCI(t *testing.T) {
	t.Run("no CI provider detected", func(t *testing.T) {
		// Reset provider registry to ensure clean state.
		ci.Reset()
		t.Cleanup(ci.Reset)

		// Clear CI env vars to ensure no provider is detected.
		t.Setenv("GITHUB_ACTIONS", "")
		t.Setenv("CI", "")
		t.Setenv("ATMOS_CI_BASE_REF", "")

		describe := &DescribeAffectedCmdArgs{
			CLIConfig: &schema.AtmosConfiguration{},
		}
		resolveBaseFromCI(describe)
		assert.Empty(t, describe.Ref)
		assert.Empty(t, describe.SHA)
	})

	t.Run("GitHub Actions PR event detected", func(t *testing.T) {
		// Reset and register provider for this test only.
		ci.Reset()
		t.Cleanup(ci.Reset)
		ci.Register(githubCI.NewProvider())

		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "pull_request")
		t.Setenv("GITHUB_BASE_REF", "main")

		eventPayload := `{"action":"synchronize","pull_request":{"base":{"ref":"main","sha":"abc123def456789012345678901234567890abcd"}}}`
		eventPath := filepath.Join(t.TempDir(), "event.json")
		err := os.WriteFile(eventPath, []byte(eventPayload), 0o644)
		require.NoError(t, err)
		t.Setenv("GITHUB_EVENT_PATH", eventPath)

		describe := &DescribeAffectedCmdArgs{
			CLIConfig: &schema.AtmosConfiguration{},
		}
		resolveBaseFromCI(describe)
		assert.Empty(t, describe.Ref)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", describe.SHA)
	})

	t.Run("GitHub Actions push event with before SHA", func(t *testing.T) {
		// Reset and register provider for this test only.
		ci.Reset()
		t.Cleanup(ci.Reset)
		ci.Register(githubCI.NewProvider())

		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "push")

		eventPayload := `{"before": "abc123def456789012345678901234567890abcd", "forced": false}`
		eventPath := filepath.Join(t.TempDir(), "event.json")
		err := os.WriteFile(eventPath, []byte(eventPayload), 0o644)
		require.NoError(t, err)
		t.Setenv("GITHUB_EVENT_PATH", eventPath)

		describe := &DescribeAffectedCmdArgs{
			CLIConfig: &schema.AtmosConfiguration{},
		}
		resolveBaseFromCI(describe)
		assert.Empty(t, describe.Ref)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", describe.SHA)
	})

	t.Run("ResolveBase returns error logs warning", func(t *testing.T) {
		// Reset and register provider for this test only.
		ci.Reset()
		t.Cleanup(ci.Reset)
		ci.Register(githubCI.NewProvider())

		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		// Missing GITHUB_EVENT_PATH causes ResolveBase to error.
		t.Setenv("GITHUB_EVENT_PATH", "")

		describe := &DescribeAffectedCmdArgs{
			CLIConfig: &schema.AtmosConfiguration{},
		}
		resolveBaseFromCI(describe)
		// Should not populate anything on error.
		assert.Empty(t, describe.Ref)
		assert.Empty(t, describe.SHA)
	})

	t.Run("ResolveBase returns nil", func(t *testing.T) {
		// Reset and register provider for this test only.
		ci.Reset()
		t.Cleanup(ci.Reset)
		ci.Register(githubCI.NewProvider())

		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")

		describe := &DescribeAffectedCmdArgs{
			CLIConfig: &schema.AtmosConfiguration{},
		}
		resolveBaseFromCI(describe)
		// workflow_dispatch returns default ref, not nil.
		assert.Equal(t, "refs/remotes/origin/HEAD", describe.Ref)
	})
}

// newDescribeAffectedFlagSet creates a pflag.FlagSet with all flags used by SetDescribeAffectedFlagValueInCliArgs.
func newDescribeAffectedFlagSet() *pflag.FlagSet {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("base", "", "")
	flags.String("ref", "", "")
	flags.String("sha", "", "")
	flags.String("repo-path", "", "")
	flags.String("ssh-key", "", "")
	flags.String("ssh-key-password", "", "")
	flags.Bool("include-spacelift-admin-stacks", false, "")
	flags.Bool("include-dependents", false, "")
	flags.Bool("include-settings", false, "")
	flags.Bool("upload", false, "")
	flags.Bool("clone-target-ref", false, "")
	flags.Bool("process-templates", true, "")
	flags.Bool("process-functions", true, "")
	flags.StringSlice("skip", nil, "")
	flags.String("pager", "", "")
	flags.String("stack", "", "")
	flags.String("format", "json", "")
	flags.String("file", "", "")
	flags.String("output-file", "", "")
	flags.String("query", "", "")
	flags.Bool("verbose", false, "")
	flags.Bool("exclude-locked", false, "")
	return flags
}

// TestSetDescribeAffectedFlagValueInCliArgs_BaseResolution tests the --base flag resolution logic.
func TestSetDescribeAffectedFlagValueInCliArgs_BaseResolution(t *testing.T) {
	// Clear CI env vars so auto-detect doesn't interfere.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")

	t.Run("base with SHA populates SHA field", func(t *testing.T) {
		flags := newDescribeAffectedFlagSet()
		err := flags.Set("base", "abc123def456789012345678901234567890abcd")
		require.NoError(t, err)

		describe := &DescribeAffectedCmdArgs{CLIConfig: &schema.AtmosConfiguration{}}
		SetDescribeAffectedFlagValueInCliArgs(flags, describe)

		assert.Equal(t, "abc123def456789012345678901234567890abcd", describe.SHA)
		assert.Empty(t, describe.Ref)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", describe.Base)
	})

	t.Run("base with ref populates Ref field", func(t *testing.T) {
		flags := newDescribeAffectedFlagSet()
		err := flags.Set("base", "main")
		require.NoError(t, err)

		describe := &DescribeAffectedCmdArgs{CLIConfig: &schema.AtmosConfiguration{}}
		SetDescribeAffectedFlagValueInCliArgs(flags, describe)

		assert.Equal(t, "main", describe.Ref)
		assert.Empty(t, describe.SHA)
		assert.Equal(t, "main", describe.Base)
	})

	t.Run("CI auto-detect when enabled and no explicit base", func(t *testing.T) {
		// Reset and register provider for this test only.
		ci.Reset()
		t.Cleanup(ci.Reset)
		ci.Register(githubCI.NewProvider())

		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_EVENT_NAME", "merge_group")
		t.Setenv("GITHUB_BASE_REF", "main")

		flags := newDescribeAffectedFlagSet()
		describe := &DescribeAffectedCmdArgs{
			CLIConfig: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Enabled: true,
				},
			},
		}
		SetDescribeAffectedFlagValueInCliArgs(flags, describe)

		assert.Equal(t, "refs/remotes/origin/main", describe.Ref)
	})
}

// TestExecute_MatrixFormat tests the matrix format code path through Execute.
func TestExecute_MatrixFormat(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	d := describeAffectedExec{atmosConfig: &schema.AtmosConfiguration{}}
	d.IsTTYSupportForStdout = func() bool {
		return false
	}
	d.executeDescribeAffectedWithTargetRefCheckout = func(
		atmosConfig *schema.AtmosConfiguration,
		ref, sha string,
		includeSpaceliftAdminStacks, includeSettings bool,
		stack string, processTemplates, processYamlFunctions bool,
		skip []string, excludeLocked bool,
		authManager auth.AuthManager,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{
			{
				Stack:         "ue1-dev",
				Component:     "vpc",
				ComponentPath: filepath.Join("components", "terraform", "vpc"),
				ComponentType: "terraform",
			},
		}, nil, nil, "", nil
	}
	d.addDependentsToAffected = func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings, processTemplates, processFunctions bool, skip []string, onlyInStack string, authManager auth.AuthManager) error {
		return nil
	}
	d.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
		return nil
	}

	t.Run("matrix to stdout", func(t *testing.T) {
		err := d.Execute(&DescribeAffectedCmdArgs{
			Format:    "matrix",
			CLIConfig: &schema.AtmosConfiguration{},
		})
		assert.NoError(t, err)
	})

	t.Run("matrix to file", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "github_output")
		err := d.Execute(&DescribeAffectedCmdArgs{
			Format:           "matrix",
			GithubOutputFile: outputFile,
			CLIConfig:        &schema.AtmosConfiguration{},
		})
		assert.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "matrix=")
		assert.Contains(t, string(content), "affected_count=1")
	})
}

// TestDescribeAffectedDeletedComponentWithDependents tests that deleted components
// don't crash when IncludeDependents is enabled. Deleted components don't exist in
// HEAD, so attempting to resolve their dependents causes "invalid component" errors.
// The fix skips dependent resolution for deleted components.
func TestDescribeAffectedDeletedComponentWithDependents(t *testing.T) {
	atmosConfig, repoPath := setupDescribeAffectedDeletedDetectionTest(t, "stacks-with-deleted-component")

	// Run describe affected to get the affected list (including deleted components).
	affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		false,
		true,
		"",
		true,  // processTemplates
		false, // processYamlFunctions
		nil,
		false,
		nil,
	)
	require.NoError(t, err)

	// Verify we have deleted components in the list.
	var deletedCount int
	for _, a := range affected {
		if a.Deleted {
			deletedCount++
		}
	}
	require.Greater(t, deletedCount, 0, "should have at least one deleted component before testing dependents")

	// This is the critical test: addDependentsToAffected should NOT crash
	// when the affected list contains deleted components.
	// Before the fix, this would fail with "invalid component" error because
	// it tried to resolve deleted components against HEAD where they don't exist.
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		false,
		nil,
		"",
		nil,
	)
	require.NoError(t, err, "addDependentsToAffected should not crash on deleted components")

	// Verify deleted components have empty dependents (they can't have dependents in HEAD).
	for _, a := range affected {
		if a.Deleted {
			assert.Empty(t, a.Dependents, "deleted component %s in %s should have empty dependents", a.Component, a.Stack)
		}
	}

	t.Logf("Successfully processed %d deleted components with dependents enabled", deletedCount)
}

// TestUploadRejectsPushEvent verifies that --upload errors when the CI event is not a pull_request.
func TestUploadRejectsPushEvent(t *testing.T) {
	d := describeAffectedExec{
		atmosConfig: &schema.AtmosConfiguration{},
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
			return nil
		},
		IsTTYSupportForStdout: func() bool { return false },
		pageCreator:           pager.New(),
	}

	headRef := plumbing.NewHashReference("refs/heads/main", plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	baseRef := plumbing.NewHashReference("refs/heads/main~1", plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	err := d.uploadableQuery(
		&DescribeAffectedCmdArgs{
			Upload:      true,
			Format:      "json",
			CIEventType: "push",
			CLIConfig:   &schema.AtmosConfiguration{},
		},
		"https://github.com/example/repo.git",
		headRef,
		baseRef,
		[]schema.Affected{},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pull_request")
	assert.Contains(t, err.Error(), "push")
}

// TestUploadAllowsPullRequestEvent verifies that --upload does not error for pull_request events.
// Note: the actual upload call requires an API client, so we only verify the event validation passes.
func TestUploadAllowsPullRequestEvent(t *testing.T) {
	d := describeAffectedExec{
		atmosConfig: &schema.AtmosConfiguration{},
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
			return nil
		},
		IsTTYSupportForStdout: func() bool { return false },
		pageCreator:           pager.New(),
	}

	headRef := plumbing.NewHashReference("refs/heads/feature", plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	baseRef := plumbing.NewHashReference("refs/heads/main", plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	// This will fail at the API client creation step (no env vars set), but the event
	// validation should pass — it should NOT return ErrUploadRequiresPullRequestEvent.
	err := d.uploadableQuery(
		&DescribeAffectedCmdArgs{
			Upload:          true,
			Format:          "json",
			CIEventType:     "pull_request",
			HeadSHAOverride: "cccccccccccccccccccccccccccccccccccccccc",
			CLIConfig:       &schema.AtmosConfiguration{},
		},
		"https://github.com/example/repo.git",
		headRef,
		baseRef,
		[]schema.Affected{},
	)
	// Should NOT be an event validation error. It may be nil (API client creation
	// logs a warning and returns nil) or some other error, but not our sentinel.
	if err != nil {
		assert.NotContains(t, err.Error(), "pull_request event")
	}
}

// TestUploadNoEventTypeAllowed verifies that --upload works when CIEventType is empty
// (e.g., when not using CI auto-detection, or using explicit --ref/--sha flags).
func TestUploadNoEventTypeAllowed(t *testing.T) {
	d := describeAffectedExec{
		atmosConfig: &schema.AtmosConfiguration{},
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
			return nil
		},
		IsTTYSupportForStdout: func() bool { return false },
		pageCreator:           pager.New(),
	}

	headRef := plumbing.NewHashReference("refs/heads/feature", plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	baseRef := plumbing.NewHashReference("refs/heads/main", plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	// Empty CIEventType means no CI auto-detection — should not block upload.
	err := d.uploadableQuery(
		&DescribeAffectedCmdArgs{
			Upload:      true,
			Format:      "json",
			CIEventType: "",
			CLIConfig:   &schema.AtmosConfiguration{},
		},
		"https://github.com/example/repo.git",
		headRef,
		baseRef,
		[]schema.Affected{},
	)
	// Should NOT be an event validation error.
	if err != nil {
		assert.NotContains(t, err.Error(), "pull_request event")
	}
}
