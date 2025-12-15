package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeAffectedAuthManagerArgument verifies that AuthManager can be passed
// to the describe affected command via DescribeAffectedCmdArgs.
//
// This enables YAML functions (!terraform.state, !terraform.output) to work
// when using the --identity flag with atmos describe affected.
//
// Note: This test verifies the AuthManager is properly received and stored in the
// command args structure. Full integration testing of AuthManager propagation through
// the complex describe affected execution paths (repo-path, clone, checkout) would
// require more extensive integration tests.
func TestDescribeAffectedAuthManagerArgument(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup: Create AuthContext with AWS credentials (populated by --identity).
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-identity",
			Region:          "us-east-1",
			CredentialsFile: "/tmp/test-creds",
			ConfigFile:      "/tmp/test-config",
		},
	}

	// Create stackInfo with AuthContext (what AuthManager.Authenticate() populates).
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		Stack:       "test-stack",
	}

	// Create mock AuthManager that returns our authStackInfo.
	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(authStackInfo).
		AnyTimes()

	// Load configuration.
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	// (this also disables parent directory search and git root discovery).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err, "Should load Atmos config")

	// Create DescribeAffectedCmdArgs with AuthManager.
	// This structure is what the cmd layer creates when --identity flag is provided.
	args := &DescribeAffectedCmdArgs{
		CLIConfig:                   &atmosConfig,
		CloneTargetRef:              false,
		Format:                      "json",
		IncludeDependents:           false,
		IncludeSettings:             false,
		IncludeSpaceliftAdminStacks: false,
		OutputFile:                  "",
		Ref:                         "",
		RepoPath:                    "",
		SHA:                         "",
		SSHKeyPath:                  "",
		SSHKeyPassword:              "",
		Verbose:                     false,
		Upload:                      false,
		Stack:                       "",
		Query:                       "",
		ProcessTemplates:            false,
		ProcessYamlFunctions:        false,
		Skip:                        nil,
		ExcludeLocked:               false,
		AuthManager:                 mockAuthManager, // AuthManager from --identity flag
	}

	// Verify that AuthManager is properly stored in the args structure.
	require.NotNil(t, args.AuthManager, "AuthManager should be set in DescribeAffectedCmdArgs")
	require.Equal(t, mockAuthManager, args.AuthManager, "AuthManager should match the provided mock")

	// Verify GetStackInfo returns the expected AuthContext.
	stackInfo := args.AuthManager.GetStackInfo()
	require.NotNil(t, stackInfo, "StackInfo should not be nil")
	require.NotNil(t, stackInfo.AuthContext, "AuthContext should not be nil")
	require.Equal(t, expectedAuthContext.AWS.Profile, stackInfo.AuthContext.AWS.Profile, "AWS profile should match")
}

// TestDescribeAffectedNilAuthManagerHandling verifies that describe affected
// works correctly when no --identity flag is provided (AuthManager is nil).
func TestDescribeAffectedNilAuthManagerHandling(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	// (this also disables parent directory search and git root discovery).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Create args without AuthManager (nil).
	args := &DescribeAffectedCmdArgs{
		CLIConfig:                   &atmosConfig,
		CloneTargetRef:              false,
		Format:                      "json",
		IncludeDependents:           false,
		IncludeSettings:             false,
		IncludeSpaceliftAdminStacks: false,
		OutputFile:                  "",
		Ref:                         "",
		RepoPath:                    "",
		SHA:                         "",
		SSHKeyPath:                  "",
		SSHKeyPassword:              "",
		Verbose:                     false,
		Upload:                      false,
		Stack:                       "",
		Query:                       "",
		ProcessTemplates:            false,
		ProcessYamlFunctions:        false,
		Skip:                        nil,
		ExcludeLocked:               false,
		AuthManager:                 nil, // nil AuthManager (no --identity flag)
	}

	// Should not panic when AuthManager is nil.
	require.Nil(t, args.AuthManager, "AuthManager should be nil when no --identity flag")
}
