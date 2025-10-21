package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfg "github.com/cloudposse/atmos/pkg/config"
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

	d.executeDescribeAffectedWithTargetRepoPath = func(atmosConfig *schema.AtmosConfiguration, targetRefPath string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}

	d.executeDescribeAffectedWithTargetRefClone = func(atmosConfig *schema.AtmosConfiguration, ref, sha, sshKeyPath, sshKeyPassword string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}

	d.executeDescribeAffectedWithTargetRefCheckout = func(atmosConfig *schema.AtmosConfiguration, ref, sha string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{
			{
				Stack: "test-stack",
			},
		}, nil, nil, "", nil
	}

	d.atmosConfig = &schema.AtmosConfiguration{}
	d.addDependentsToAffected = func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings bool, processTemplates bool, processFunctions bool, skip []string, onlyInStack string) error {
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
	// Enable pager for the tests that expect it to be called
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
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/atmos-describe-affected"

	// Point to the same local repository
	// This will compare this local repository with itself as the remote target, which should result in an empty `affected` list
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
			if strings.Contains(src, "node_modules") {
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

	// Copy the local repository into a temp dir
	err = cp.Copy(pathPrefix, tempDir, copyOptions)
	require.NoError(t, err)

	// Copy the affected stacks into the `stacks` folder in the temp dir
	err = cp.Copy(filepath.Join(stacksPath, "stacks-affected"), filepath.Join(tempDir, basePath, "stacks"), copyOptions)
	require.NoError(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "./",
	// which will be wrong for the cloned repo in the temp dir.
	// Set the correct base path for the cloned remote repo
	config.BasePath = basePath

	// Point to the copy of the local repository
	repoPath = tempDir

	// OS-specific expected component path
	componentPath = filepath.Join("tests", "fixtures", "components", "terraform", "mock")

	return config, repoPath, componentPath
}

//nolint:dupl // Test scenarios have similar structures but test different conditions.
func TestDescribeAffectedWithTemplatesAndFunctions(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true` and `excludeLocked: false`
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
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
	assert.ElementsMatch(t, expected, affected)
}

//nolint:dupl // Test scenarios have similar structures but test different conditions.
func TestDescribeAffectedWithoutTemplatesAndFunctions(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: false`, `processFunctions: false` and `excludeLocked: false`
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
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
	assert.ElementsMatch(t, expected, affected)
}

func TestDescribeAffectedWithExcludeLocked(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true` and `excludeLocked: true`
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
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
	assert.ElementsMatch(t, expected, affected)
}

//nolint:dupl // Test scenarios have similar structures but test different dependents processing.
func TestDescribeAffectedWithDependents(t *testing.T) {
	atmosConfig, repoPath, componentPath := setupDescribeAffectedTest(t)

	// Test affected with `processTemplates: true`, `processFunctions: true`, `excludeLocked: false`,
	// and process dependents (with `processTemplates: true`, `processFunctions: true` for the dependents)
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
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
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
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
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
	)
	require.NoError(t, err)
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		true,
		nil,
		onlyInStack, // Filter dependents to only show those in "ue1-network" stack.
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
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
	)
	require.NoError(t, err)
	err = addDependentsToAffected(
		&atmosConfig,
		&affected,
		true,
		true,
		true,
		nil,
		onlyInStack, // Filter dependents to only show those in "uw2-network" stack.
	)
	require.NoError(t, err)
	// Order-agnostic equality on struct slices
	assert.ElementsMatch(t, expected, affected)
}
