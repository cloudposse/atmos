package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/golang/mock/gomock"
	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeAffected(t *testing.T) {
	d := describeAffectedExec{atmosConfig: &schema.AtmosConfiguration{}}
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
	d.addDependentsToAffected = func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings bool, processTemplates bool, processFunctions bool, skip []string) error {
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
}

func TestExecuteDescribeAffectedWithTargetRepoPath(t *testing.T) {
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

func TestDescribeAffectedExecute(t *testing.T) {
	basePath := "tests/fixtures/scenarios/atmos-describe-affected-with-dependents-and-locked"
	pathPrefix := "../../"

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	defer removeTempDir(tempDir)

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
	atmosConfig.BasePath = basePath

	// Point to the copy of the local repository
	repoPath := tempDir

	// OS-specific expected component path
	componentPath := filepath.Join("tests", "fixtures", "components", "terraform", "mock")

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
			Dependents:           nil, // must be nil to match actual
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
			Dependents:           nil, // must be nil to match actual
			IncludedInDependents: false,
			Settings: map[string]any{
				"depends_on": map[any]any{ // note: any keys
					1: map[string]any{
						"component": "tgw/hub",
						"stack":     "ue1-network",
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
			Dependents:           nil, // must be nil to match actual
			IncludedInDependents: false,
			Settings:             map[string]any{},
		},
	}

	// Test affected with `excludeLocked: false`
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
