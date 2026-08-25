package exec

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	auth "github.com/cloudposse/atmos/pkg/auth"
	mockTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubPackerAuthSeams replaces the auth-config and auth-manager creation seams so
// ExecutePacker builds a fake, non-nil AuthManager without touching a real cloud.
// The returned manager only needs GetStackInfo (called by setupTerraformAuth); an
// explicit info.Identity short-circuits the chain/default-identity lookups.
func stubPackerAuthSeams(t *testing.T, manager auth.AuthManager) {
	t.Helper()

	origGetter := defaultMergedAuthConfigGetter
	origCreator := defaultAuthManagerCreator
	t.Cleanup(func() {
		defaultMergedAuthConfigGetter = origGetter
		defaultAuthManagerCreator = origCreator
	})

	defaultMergedAuthConfigGetter = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
		return &schema.AuthConfig{}, nil
	}
	defaultAuthManagerCreator = func(identity string, authConfig *schema.AuthConfig, selectValue string, atmosConfig *schema.AtmosConfiguration, stack string) (auth.AuthManager, error) {
		return manager, nil
	}
}

// TestExecutePacker_PassesAuthManagerToProcessStacks is the primary regression test
// for the packer auth bug: `atmos packer` historically called ProcessStacks with a
// nil AuthManager, so the `auth:` section was never evaluated and no credentials were
// injected into the packer subprocess ("No valid credential sources found").
//
// The test intercepts ProcessStacks via the processStacksForPacker seam and asserts
// that ExecutePacker passes a non-nil AuthManager, matching terraform and helmfile.
func TestExecutePacker_PassesAuthManagerToProcessStacks(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)

	ctrl := gomock.NewController(t)
	fakeManager := mockTypes.NewMockAuthManager(ctrl)
	// setupTerraformAuth calls GetStackInfo; return nil so it is a no-op.
	fakeManager.EXPECT().GetStackInfo().Return(nil).AnyTimes()
	stubPackerAuthSeams(t, fakeManager)

	origProcess := processStacksForPacker
	t.Cleanup(func() { processStacksForPacker = origProcess })

	var captured auth.AuthManager
	var called bool
	processStacksForPacker = func(
		atmosConfig *schema.AtmosConfiguration,
		info schema.ConfigAndStacksInfo,
		checkStack, processTemplates, processFunctions bool,
		skip []string,
		authManager auth.AuthManager,
	) (schema.ConfigAndStacksInfo, error) {
		captured = authManager
		called = true
		// Short-circuit ExecutePacker before component-path resolution: keep the
		// stack set (so the missing-stack guard passes) but mark it disabled so
		// ExecutePacker returns early.
		info.ComponentIsEnabled = false
		return info, nil
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		Stack:            "nonprod",
		SubCommand:       "build",
		Identity:         "test-identity",
	}

	err := ExecutePacker(info, &PackerFlags{})
	require.NoError(t, err)
	require.True(t, called, "processStacksForPacker should have been invoked")
	require.NotNil(t, captured,
		"ExecutePacker must pass a non-nil AuthManager to ProcessStacks so Atmos Auth "+
			"credentials are evaluated and injected into the packer subprocess "+
			"(regression: packer passed nil, causing 'No valid credential sources found')")
}

// TestExecutePacker_InjectsAuthCredentialsIntoSubprocessEnv confirms that the
// resolved Atmos Auth credentials actually reach the packer subprocess environment.
// A fake AuthManager injects a sentinel AWS credential via PrepareShellEnvironment;
// the packer shell invocation is intercepted so no real packer binary runs.
func TestExecutePacker_InjectsAuthCredentialsIntoSubprocessEnv(t *testing.T) {
	// No RequirePacker guard: executePackerShellCommand is replaced below, so no real
	// Packer binary is launched, and the fixture declares no packer tool-dependencies,
	// so the pre-exec pipeline (path resolution, validation, varfile) needs no binary.
	const sentinel = "AWS_ACCESS_KEY_ID=ATMOS_AUTH_SENTINEL"

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)

	ctrl := gomock.NewController(t)
	fakeManager := mockTypes.NewMockAuthManager(ctrl)
	fakeManager.EXPECT().GetStackInfo().Return(nil).AnyTimes()
	// prepareComponentAuthEnvironment calls PrepareShellEnvironment with the resolved
	// identity; return the incoming env with the sentinel credential appended.
	fakeManager.EXPECT().
		PrepareShellEnvironment(gomock.Any(), "test-identity", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, env []string) ([]string, error) {
			return append(env, sentinel), nil
		}).
		AnyTimes()
	stubPackerAuthSeams(t, fakeManager)

	origShell := executePackerShellCommand
	t.Cleanup(func() { executePackerShellCommand = origShell })

	var capturedEnv []string
	var shellCalled bool
	executePackerShellCommand = func(
		atmosConfig schema.AtmosConfiguration,
		command string,
		args []string,
		dir string,
		env []string,
		dryRun bool,
		redirectStdError string,
		opts ...ShellCommandOption,
	) error {
		capturedEnv = env
		shellCalled = true
		return nil
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		Stack:            "nonprod",
		SubCommand:       "validate",
		ProcessTemplates: true,
		ProcessFunctions: true,
		Identity:         "test-identity",
	}

	err := ExecutePacker(info, &PackerFlags{})
	require.NoError(t, err)
	require.True(t, shellCalled, "packer shell command should have been invoked")
	require.Contains(t, strings.Join(capturedEnv, "\n"), sentinel,
		"Atmos Auth credentials must be injected into the packer subprocess environment")
}
