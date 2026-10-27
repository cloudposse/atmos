package terraform

import (
	context "context"
	errors "errors"
	filepath "path/filepath"
	testing "testing"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	types "github.com/cloudposse/atmos/pkg/auth/types"
	component "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	schema "github.com/cloudposse/atmos/pkg/schema"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestExtractAffectedNodeIDs(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ids := extractAffectedNodeIDs(nil)
		assert.Empty(t, ids)
	})

	t.Run("single affected", func(t *testing.T) {
		affected := []schema.Affected{
			{Component: "vpc", Stack: "dev"},
		}
		ids := extractAffectedNodeIDs(affected)
		assert.Equal(t, []string{"vpc-dev"}, ids)
	})

	t.Run("multiple affected", func(t *testing.T) {
		affected := []schema.Affected{
			{Component: "vpc", Stack: "dev"},
			{Component: "rds", Stack: "prod"},
			{Component: "app", Stack: "staging"},
		}
		ids := extractAffectedNodeIDs(affected)
		assert.Equal(t, 3, len(ids))
		assert.Equal(t, "vpc-dev", ids[0])
		assert.Equal(t, "rds-prod", ids[1])
		assert.Equal(t, "app-staging", ids[2])
	})
}

// TestAffectedTerraformSelection verifies the selection carries the parsed
// --include-dependents depth from info: a zero DependentDepth means unlimited
// in dependency.Filter encoding, so omitting it silently erased a user-supplied
// depth bound on the --affected path (the closure spec keeps the most
// permissive depth when merging selection and info).
func TestAffectedTerraformSelection(t *testing.T) {
	affected := []schema.Affected{{Component: "vpc", Stack: "dev"}}

	tests := []struct {
		name                  string
		argsIncludeDependents bool
		infoIncludeDependents int
		wantInclude           bool
		wantDepth             int
	}{
		{
			name:                  "flag off",
			argsIncludeDependents: false,
			infoIncludeDependents: 0,
			wantInclude:           false,
			wantDepth:             0,
		},
		{
			name:                  "bare flag is unlimited",
			argsIncludeDependents: true,
			infoIncludeDependents: -1,
			wantInclude:           true,
			wantDepth:             0,
		},
		{
			name:                  "numeric depth is carried through, not erased to unlimited",
			argsIncludeDependents: true,
			infoIncludeDependents: 2,
			wantInclude:           true,
			wantDepth:             2,
		},
		{
			name:                  "programmatically forced dependents without flag depth stay unlimited",
			argsIncludeDependents: true,
			infoIncludeDependents: 0,
			wantInclude:           true,
			wantDepth:             0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := &DescribeAffectedCmdArgs{IncludeDependents: tc.argsIncludeDependents}
			info := &schema.ConfigAndStacksInfo{IncludeDependents: tc.infoIncludeDependents}

			selection := affectedTerraformSelection(affected, args, info)

			assert.Equal(t, []string{"vpc-dev"}, selection.NodeIDs)
			assert.Equal(t, tc.wantInclude, selection.IncludeDependents)
			assert.Equal(t, tc.wantDepth, selection.DependentDepth)
		})
	}

	t.Run("nil info leaves depth unlimited", func(t *testing.T) {
		args := &DescribeAffectedCmdArgs{IncludeDependents: true}
		selection := affectedTerraformSelection(affected, args, nil)
		assert.True(t, selection.IncludeDependents)
		assert.Equal(t, 0, selection.DependentDepth)
	})
}

func TestAffectedExecutorRejectsMissingRequest(t *testing.T) {
	executor := NewExecutor(Services{})
	require.ErrorIs(t, executor.ExecuteTerraformAffected(nil, &schema.ConfigAndStacksInfo{}), errUtils.ErrNilParam)
	require.ErrorIs(t, executor.ExecuteTerraformAffected(&component.DescribeAffectedCmdArgs{}, nil), errUtils.ErrNilParam)
}

func TestAffectedExecutorPreservesServiceBoundaries(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "..", "tests", "fixtures", "scenarios", "terraform-apply-affected"))
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	for _, outcome := range []string{"lookup error", "empty after filtering", "describe error", "scheduled"} {
		t.Run(outcome, func(t *testing.T) {
			executor := testExecutor(t)
			sentinel := errors.New("host service unavailable")
			args := &component.DescribeAffectedCmdArgs{Stack: "prod", AuthDisabled: true}
			info := &schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "plan", Stack: "prod", ProcessTemplates: true, ProcessFunctions: true, UseMocks: true, Skip: []string{"terraform.state"}}
			lookedUp, described, executed := false, false, false
			executor.services.GetAffectedComponents = func(actual *component.DescribeAffectedCmdArgs) ([]schema.Affected, error) {
				lookedUp = true
				require.Same(t, args, actual)
				require.NotNil(t, actual.CLIConfig)
				require.True(t, actual.AuthDisabled)
				require.Nil(t, actual.AuthManager)
				if outcome == "lookup error" {
					return nil, sentinel
				}
				result := []schema.Affected{{Component: "chart", Stack: "prod", ComponentType: "helmfile"}, {Component: "removed", Stack: "prod", ComponentType: "terraform", Deleted: true}}
				if outcome != "empty after filtering" {
					result = append(result, schema.Affected{Component: "vpc", Stack: "prod", ComponentType: "terraform"})
				}
				return result, nil
			}
			executor.services.ExecuteDescribeStacksWithAuthDisabledAndMocks = func(ac *schema.AtmosConfiguration, stack string, components, types, sections []string, ignoreMissing, templates, functions, empty bool, skip []string, manager auth.AuthManager, disabled, mocks bool) (map[string]any, error) {
				described = true
				require.Same(t, args.CLIConfig, ac)
				require.Empty(t, stack)
				require.Nil(t, components)
				require.Equal(t, []string{"terraform"}, types)
				require.Nil(t, sections)
				require.False(t, ignoreMissing)
				require.True(t, templates)
				require.True(t, functions)
				require.False(t, empty)
				require.Equal(t, info.Skip, skip)
				require.Nil(t, manager)
				require.True(t, disabled)
				require.True(t, mocks)
				if outcome == "describe error" {
					return nil, sentinel
				}
				return map[string]any{"prod": map[string]any{"components": map[string]any{"terraform": map[string]any{"vpc": map[string]any{"vars": map[string]any{}}, "unrelated": map[string]any{"vars": map[string]any{}}}}}}, nil
			}
			executor.services.ExecuteTerraform = func(actual schema.ConfigAndStacksInfo, opts ...component.ExecutionOption) error {
				executed = true
				require.Equal(t, "vpc", actual.ComponentFromArg)
				require.Equal(t, "prod", actual.Stack)
				require.Equal(t, "plan", actual.SubCommand)
				require.True(t, actual.AuthDisabled)
				require.Equal(t, cfg.IdentityFlagDisabledValue, actual.Identity)
				require.NoError(t, component.ContextFromOptions(opts...).Err())
				return nil
			}
			err := executor.ExecuteTerraformAffectedWithContext(context.Background(), args, info)
			if outcome == "lookup error" || outcome == "describe error" {
				require.ErrorIs(t, err, sentinel)
			} else {
				require.NoError(t, err)
			}
			require.True(t, lookedUp)
			require.Equal(t, outcome == "describe error" || outcome == "scheduled", described)
			require.Equal(t, outcome == "scheduled", executed)
			require.True(t, info.AuthDisabled)
			require.Equal(t, cfg.IdentityFlagDisabledValue, info.Identity)
		})
	}
}

// TestCreateQueryAuthManager verifies createQueryAuthManager behavior across
// different scenarios using injected auth factory to cover all branches.
func TestCreateQueryAuthManager(t *testing.T) {
	tests := []struct {
		name             string
		setupMock        func(ctrl *gomock.Controller, m *MockAuthManagerQueryFactory)
		expectErr        bool
		expectSentinel   error
		expectNilMgr     bool
		expectInfoStored bool
		expectExit       bool
	}{
		{
			name: "no auth configured returns nil manager",
			setupMock: func(_ *gomock.Controller, m *MockAuthManagerQueryFactory) {
				m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			expectNilMgr:     true,
			expectInfoStored: false,
		},
		{
			name: "nonexistent identity returns wrapped error",
			setupMock: func(_ *gomock.Controller, m *MockAuthManagerQueryFactory) {
				m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrAuthNotConfigured)
			},
			expectErr:        true,
			expectSentinel:   errUtils.ErrAuthNotConfigured,
			expectNilMgr:     true,
			expectInfoStored: false,
		},
		{
			name: "non-nil manager is stored in info",
			setupMock: func(ctrl *gomock.Controller, m *MockAuthManagerQueryFactory) {
				m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(types.NewMockAuthManager(ctrl), nil)
			},
			expectNilMgr:     false,
			expectInfoStored: true,
		},
		{
			name: "ErrUserAborted calls Exit with SIGINT code",
			setupMock: func(_ *gomock.Controller, m *MockAuthManagerQueryFactory) {
				m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrUserAborted)
			},
			expectExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inject test factory.
			ctrl := gomock.NewController(t)
			mockFactory := NewMockAuthManagerQueryFactory(ctrl)
			tt.setupMock(ctrl, mockFactory)
			original := authManagerFactory
			authManagerFactory = mockFactory
			defer func() { authManagerFactory = original }()

			// Mock os.Exit to capture exit calls.
			var exitCode int
			exitCalled := false
			originalExit := errUtils.OsExit
			errUtils.OsExit = func(code int) {
				exitCode = code
				exitCalled = true
			}
			defer func() { errUtils.OsExit = originalExit }()

			info := &schema.ConfigAndStacksInfo{}
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: t.TempDir(),
			}

			authManager, err := createQueryAuthManager(info, atmosConfig)

			if tt.expectExit {
				assert.True(t, exitCalled, "Exit should have been called")
				assert.Equal(t, errUtils.ExitCodeSIGINT, exitCode)
				return
			}

			assert.False(t, exitCalled, "Exit should not have been called")

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectSentinel != nil {
					assert.ErrorIs(t, err, tt.expectSentinel)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNilMgr {
				assert.Nil(t, authManager)
			} else {
				assert.NotNil(t, authManager)
			}

			if tt.expectInfoStored {
				assert.NotNil(t, info.AuthManager, "AuthManager should be stored in info")
			} else {
				assert.Nil(t, info.AuthManager, "AuthManager should not be stored in info")
			}
		})
	}
}

// TestExecuteTerraformQueryComponent covers testExecutor(t).executeTerraformQueryComponent
// directly — it has no test at all today (only testExecutor(t).ExecuteTerraformQuery /
// testExecutor(t).ExecuteTerraformQueryWithContext, which call it indirectly through the
// scheduler, are exercised elsewhere). A nonexistent component/stack makes
// the underlying ExecuteTerraform fail fast during stack processing, without
// needing a real terraform/tofu binary, so both CaptureOutput branches can be
// asserted directly.
func TestExecuteTerraformQueryComponent(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "..", "examples", "demo-stacks"))

	tests := []struct {
		name          string
		captureOutput bool
	}{
		{name: "without output capture", captureOutput: false},
		{name: "with output capture", captureOutput: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			execution := scheduleradapters.TerraformExecution{
				Context: context.Background(),
				Info: schema.ConfigAndStacksInfo{
					ComponentFromArg: "nonexistent",
					Stack:            "nonexistent",
					SubCommand:       "plan",
				},
				CaptureOutput: tc.captureOutput,
			}

			result, err := testExecutor(t).executeTerraformQueryComponent(execution)

			require.Error(t, err, "a nonexistent component/stack must fail during stack processing")
			assert.Empty(t, result.Stdout, "no shell command ever runs, so nothing is captured")
			assert.Empty(t, result.Stderr)
		})
	}
}
