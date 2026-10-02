package exec

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestTerraformComponentMocksResolveStateAndOutput(t *testing.T) {
	sandbox, err := testhelpers.SetupSandbox(t, "../../tests/fixtures/scenarios/terraform-component-mocks")
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(sandbox.OriginalWorkdir)
	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "app",
		ComponentType:    cfg.TerraformComponentType,
		Stack:            "dev",
		UseMocks:         true,
	}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	atmosConfig.Components.Terraform.Mocks.Mode = schema.TerraformMocksModeAlways

	stackInfo := &schema.ConfigAndStacksInfo{UseMocks: true}
	state, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc vpc_id", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, "vpc-local", state)

	output, err := processTagTerraformOutput(&atmosConfig, "!terraform.output vpc '.network.cidr'", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.0/16", output)

	list, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc private_subnet_ids", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, []any{"subnet-a", "subnet-b"}, list)

	literal, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc literal_template", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, "{{ .Environment }}", literal, "mock values must not be template-evaluated")

	nullable, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc nullable_output", "dev", stackInfo)
	require.NoError(t, err)
	assert.Nil(t, nullable)
}

func TestTerraformComponentMocksAlwaysModeFailClosed(t *testing.T) {
	sandbox, err := testhelpers.SetupSandbox(t, "../../tests/fixtures/scenarios/terraform-component-mocks")
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(sandbox.OriginalWorkdir)
	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: "dev"}, true)
	require.NoError(t, err)
	atmosConfig.Components.Terraform.Mocks.Mode = schema.TerraformMocksModeAlways

	_, err = processTagTerraformState(&atmosConfig, "!terraform.state app missing", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.ErrorIs(t, err, errUtils.ErrTerraformComponentMocksNotDeclared)

	_, err = processTagTerraformOutput(&atmosConfig, "!terraform.output vpc missing", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.ErrorIs(t, err, errUtils.ErrTerraformMockOutputNotDeclared)

	_, err = processTagTerraformOutput(&atmosConfig, "!terraform.output vpc '.network.missing'", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.ErrorIs(t, err, errUtils.ErrTerraformMockOutputNotDeclared)
}

// TestTerraformComponentMocksYqDefaultDoesNotRequireMocks verifies that a YQ `//`
// default in the caller's expression is honored even when the referenced
// component declares no `mocks` section at all, mirroring how a `//` default
// already rescues a component with no real state. Without a default, an
// undeclared `mocks` map must still hard-error (see TestTerraformComponentMocksAlwaysModeFailClosed).
func TestTerraformComponentMocksYqDefaultDoesNotRequireMocks(t *testing.T) {
	sandbox, err := testhelpers.SetupSandbox(t, "../../tests/fixtures/scenarios/terraform-component-mocks")
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(sandbox.OriginalWorkdir)
	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: "dev"}, true)
	require.NoError(t, err)
	atmosConfig.Components.Terraform.Mocks.Mode = schema.TerraformMocksModeAlways

	// "app" declares no `mocks` section at all.
	value, err := processTagTerraformState(&atmosConfig, `!terraform.state app .missing // "yq-fallback"`, "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.NoError(t, err)
	assert.Equal(t, "yq-fallback", value)
}

// Compile-time sentinels: the fallback tests reference these schema fields.
var (
	_ = schema.TerraformMocks{Mode: schema.TerraformMocksModeFallback}
	_ = schema.ConfigAndStacksInfo{UseMocks: true}
)

// newMocksFixtureConfig loads the terraform-component-mocks fixture and pins components.terraform.mocks.mode.
func newMocksFixtureConfig(t *testing.T, mode schema.TerraformMocksMode) *schema.AtmosConfiguration {
	t.Helper()

	sandbox, err := testhelpers.SetupSandbox(t, "../../tests/fixtures/scenarios/terraform-component-mocks")
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(sandbox.OriginalWorkdir)
	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: "dev"}, true)
	require.NoError(t, err)
	atmosConfig.Components.Terraform.Mocks.Mode = mode
	return &atmosConfig
}

// installMockGetters swaps the package-level state and output getters for gomock doubles.
func installMockGetters(t *testing.T) (*MockTerraformStateGetter, *MockTerraformOutputGetter) {
	t.Helper()

	ctrl := gomock.NewController(t)
	stateMock := NewMockTerraformStateGetter(ctrl)
	outputMock := NewMockTerraformOutputGetter(ctrl)
	originalState, originalOutput := stateGetter, outputGetter
	stateGetter, outputGetter = stateMock, outputMock
	t.Cleanup(func() { stateGetter, outputGetter = originalState, originalOutput })
	return stateMock, outputMock
}

var errMocksAccessDenied = errors.New("access denied")

// expectStateLookup registers one GetState call that must receive the given output expression
// and returns (value, err).
func expectStateLookup(stateMock *MockTerraformStateGetter, expr string, value any, err error) {
	stateMock.EXPECT().
		GetState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(expr), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(value, err).
		Times(1)
}

// expectOutputLookup registers one GetOutput call that must receive the given output expression
// and returns (value, exists, err).
func expectOutputLookup(outputMock *MockTerraformOutputGetter, expr string, value any, exists bool, err error) {
	outputMock.EXPECT().
		GetOutput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(expr), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(value, exists, err).
		Times(1)
}

type mocksFallbackCase struct {
	name string
	// mode is the effective components.terraform.mocks.mode.
	mode schema.TerraformMocksMode
	// useMocks mirrors --use-mocks.
	useMocks bool
	// input is the YAML function expression; stack is always dev.
	input string
	// lookupExpr is the output expression the real getter must receive. Empty means the whole
	// output map is requested (terraformAllOutputsExpression), which is what fallback mode does
	// for a component that declares mocks.
	lookupExpr string
	// lookupValue / lookupErr are what the real getter returns; skipLookup means the getter must
	// not be called at all (always mode, or mock loading failed first).
	lookupValue any
	lookupErr   error
	skipLookup  bool
	want        any
	wantErrIs   error
	wantErrText string
	notErrText  string
}

func mocksFallbackCases(function string) []mocksFallbackCase {
	notProvisioned := fmt.Errorf("component not provisioned: %w", errUtils.ErrTerraformStateNotProvisioned)
	outputMissing := fmt.Errorf("no such output: %w", errUtils.ErrTerraformOutputNotFound)
	fb := schema.TerraformMocksModeFallback
	return []mocksFallbackCase{
		{
			name: "fallback: real value wins even when the mock is declared", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupValue: map[string]any{"vpc_id": "vpc-real"}, want: "vpc-real",
		},
		{
			name: "fallback: real value wins over the mock and the YQ default", mode: fb, useMocks: true,
			input: function + ` vpc '.vpc_id // "vpc-default"'`, lookupValue: map[string]any{"vpc_id": "vpc-real"}, want: "vpc-real",
		},
		{
			name: "fallback: real value for a complex expression wins", mode: fb, useMocks: true,
			input: function + " vpc '.network.cidr'", lookupValue: map[string]any{"network": map[string]any{"cidr": "192.168.0.0/16"}}, want: "192.168.0.0/16",
		},
		{
			name: "fallback: provisioned state missing the output resolves from the mock before the YQ default", mode: fb, useMocks: true,
			input: function + ` vpc '.vpc_id // "vpc-default"'`, lookupValue: map[string]any{"other": "x"}, want: "vpc-local",
		},
		{
			name: "fallback: provisioned state missing the output resolves from the mock for a direct name", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupValue: map[string]any{"other": "x"}, want: "vpc-local",
		},
		{
			name: "fallback: provisioned state with a nil outputs map resolves from the mock", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupValue: nil, want: "vpc-local",
		},
		{
			name: "fallback: provisioned state missing the output and the mock uses the YQ default", mode: fb, useMocks: true,
			input: function + ` vpc '.missing_id // "vpc-default"'`, lookupValue: map[string]any{"other": "x"}, want: "vpc-default",
		},
		{
			name: "fallback: provisioned state missing a list output resolves an indexed expression from the mock", mode: fb, useMocks: true,
			input: function + " vpc '.private_subnet_ids[0]'", lookupValue: map[string]any{}, want: "subnet-a",
		},
		{
			name: "fallback: real outputs overlay the mocks per top-level output", mode: fb, useMocks: true,
			input: function + " vpc '.private_subnet_ids[1]'", lookupValue: map[string]any{"vpc_id": "vpc-real", "private_subnet_ids": []any{"real-a", "real-b"}}, want: "real-b",
		},
		{
			name: "fallback: a mock fills a key missing from a real map output", mode: fb, useMocks: true,
			input: function + " vpc '.network.cidr'", lookupValue: map[string]any{"network": map[string]any{"name": "real"}}, want: "10.0.0.0/16",
		},
		{
			name: "fallback: a real nested value wins over the mocked nested value", mode: fb, useMocks: true,
			input: function + " vpc '.network.cidr'", lookupValue: map[string]any{"network": map[string]any{"cidr": "192.168.0.0/16"}}, want: "192.168.0.0/16",
		},
		{
			name: "fallback: a real map output is merged over the mocked map", mode: fb, useMocks: true,
			input: function + " vpc network", lookupValue: map[string]any{"network": map[string]any{"name": "real"}},
			want: map[string]any{"name": "real", "cidr": "10.0.0.0/16"},
		},
		{
			name: "fallback: a real list output replaces the mocked list without merging elements", mode: fb, useMocks: true,
			input: function + " vpc '.private_subnet_ids[1]'", lookupValue: map[string]any{"private_subnet_ids": []any{"real-a"}}, want: nil,
		},
		{
			name: "fallback: explicit null in the real outputs is a hit and is not replaced by the mock", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupValue: map[string]any{"vpc_id": nil}, want: nil,
		},
		{
			name: "fallback: provisioned output missing in real and mocks with no default is nil", mode: fb, useMocks: true,
			input: function + " vpc missing", lookupValue: map[string]any{"other": "x"}, want: nil,
		},
		{
			name: "fallback: not provisioned resolves from the mock", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupErr: notProvisioned, want: "vpc-local",
		},
		{
			name: "fallback: output not found resolves from the mock", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupErr: outputMissing, want: "vpc-local",
		},
		{
			name: "fallback: nested mock path resolves when not provisioned", mode: fb, useMocks: true,
			input: function + " vpc '.network.cidr'", lookupErr: notProvisioned, want: "10.0.0.0/16",
		},
		{
			name: "fallback: indexed mock path resolves when not provisioned", mode: fb, useMocks: true,
			input: function + " vpc '.private_subnet_ids[0]'", lookupErr: notProvisioned, want: "subnet-a",
		},
		{
			name: "fallback: mock beats the YQ default when not provisioned", mode: fb, useMocks: true,
			input: function + ` vpc '.vpc_id // "default"'`, lookupErr: notProvisioned, want: "vpc-local",
		},
		{
			name: "fallback: explicit null mock is a hit", mode: fb, useMocks: true,
			input: function + " vpc nullable_output", lookupErr: notProvisioned, want: nil,
		},
		{
			name: "fallback: mocks do not declare the output and a YQ default uses the default", mode: fb, useMocks: true,
			input: function + ` vpc '.missing // "yq-fallback"'`, lookupErr: notProvisioned, want: "yq-fallback",
		},
		{
			name: "fallback: mock does not declare the output and no default keeps the original error", mode: fb, useMocks: true,
			input: function + " vpc missing", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
			notErrText: "is not declared",
		},
		{
			name: "fallback: a complex expression that resolves to nothing keeps the original error", mode: fb, useMocks: true,
			input: function + " vpc '.missing[0]'", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
		},
		{
			name: "fallback: no mocks declared and a YQ default uses the default through the plain lookup", mode: fb, useMocks: true,
			input: function + ` app '.missing // "yq-fallback"'`, lookupExpr: `.missing // "yq-fallback"`, lookupErr: notProvisioned, want: "yq-fallback",
		},
		{
			name: "fallback: no mocks declared and no default keeps the original error through the plain lookup", mode: fb, useMocks: true,
			input: function + " app vpc_id", lookupExpr: "vpc_id", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
			notErrText: "does not declare `mocks`",
		},
		{
			name: "fallback: no mocks declared returns the real value through the plain lookup", mode: fb, useMocks: true,
			input: function + " app vpc_id", lookupExpr: "vpc_id", lookupValue: "vpc-real", want: "vpc-real",
		},
		{
			name: "fallback: non-recoverable error is returned and the mock is not used", mode: fb, useMocks: true,
			input: function + " vpc vpc_id", lookupErr: errMocksAccessDenied, wantErrIs: errMocksAccessDenied,
		},
		{
			name: "fallback: mock loading failure is returned before the real lookup", mode: fb, useMocks: true,
			input: function + " does-not-exist vpc_id", skipLookup: true, wantErrIs: errUtils.ErrInvalidComponent, wantErrText: "failed to load mocks",
		},
		{
			name: "mocks off: a recoverable miss never reads mocks even in fallback mode", mode: fb, useMocks: false,
			input: function + " vpc vpc_id", lookupExpr: "vpc_id", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
		},
		{
			name: "always: mock resolves without calling the real getter", mode: schema.TerraformMocksModeAlways, useMocks: true,
			input: function + " vpc vpc_id", skipLookup: true, want: "vpc-local",
		},
		{
			name: "always: missing mock is an error without calling the real getter", mode: schema.TerraformMocksModeAlways, useMocks: true,
			input: function + " vpc missing", skipLookup: true, wantErrIs: errUtils.ErrTerraformMockOutputNotDeclared,
		},
		{
			name: "always: undeclared mocks map is an error without calling the real getter", mode: schema.TerraformMocksModeAlways, useMocks: true,
			input: function + " app vpc_id", skipLookup: true, wantErrIs: errUtils.ErrTerraformComponentMocksNotDeclared,
		},
	}
}

func (c *mocksFallbackCase) expectedLookupExpr() string {
	if c.lookupExpr == "" {
		return terraformAllOutputsExpression
	}
	return c.lookupExpr
}

func assertMocksFallbackResult(t *testing.T, tt *mocksFallbackCase, got any, err error) {
	t.Helper()

	if tt.wantErrIs == nil && tt.wantErrText == "" {
		require.NoError(t, err)
		assert.Equal(t, tt.want, got)
		return
	}
	require.Error(t, err)
	assert.Nil(t, got)
	if tt.wantErrIs != nil {
		assert.ErrorIs(t, err, tt.wantErrIs)
	}
	if tt.wantErrText != "" {
		assert.Contains(t, err.Error(), tt.wantErrText)
	}
	if tt.notErrText != "" {
		assert.NotContains(t, err.Error(), tt.notErrText)
	}
}

func TestTerraformStateComponentMocksModes(t *testing.T) {
	for _, tt := range mocksFallbackCases("!terraform.state") {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := newMocksFixtureConfig(t, tt.mode)
			stateMock, _ := installMockGetters(t)
			if !tt.skipLookup {
				expectStateLookup(stateMock, tt.expectedLookupExpr(), tt.lookupValue, tt.lookupErr)
			}

			got, err := processTagTerraformState(atmosConfig, tt.input, "dev", &schema.ConfigAndStacksInfo{UseMocks: tt.useMocks})

			assertMocksFallbackResult(t, &tt, got, err)
		})
	}
}

func TestTerraformOutputComponentMocksModes(t *testing.T) {
	for _, tt := range mocksFallbackCases("!terraform.output") {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := newMocksFixtureConfig(t, tt.mode)
			_, outputMock := installMockGetters(t)
			if !tt.skipLookup {
				expectOutputLookup(outputMock, tt.expectedLookupExpr(), tt.lookupValue, tt.lookupErr == nil && tt.lookupValue != nil, tt.lookupErr)
			}

			got, err := processTagTerraformOutput(atmosConfig, tt.input, "dev", &schema.ConfigAndStacksInfo{UseMocks: tt.useMocks})

			assertMocksFallbackResult(t, &tt, got, err)
		})
	}
}

// TestTerraformComponentMocksFallbackHintsAtAlwaysMode verifies that a non-recoverable lookup error
// in fallback mode keeps its sentinel and gains a hint pointing at --use-mocks=always, the
// mocks-only mode that needs no Terraform, credentials, or backend.
func TestTerraformComponentMocksFallbackHintsAtAlwaysMode(t *testing.T) {
	t.Run("terraform.state", func(t *testing.T) {
		atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
		stateMock, _ := installMockGetters(t)
		expectStateLookup(stateMock, terraformAllOutputsExpression, nil, errMocksAccessDenied)

		_, err := processTagTerraformState(atmosConfig, "!terraform.state vpc vpc_id", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})

		require.ErrorIs(t, err, errMocksAccessDenied)
		assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "--use-mocks=always")
	})

	t.Run("terraform.output", func(t *testing.T) {
		atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
		_, outputMock := installMockGetters(t)
		expectOutputLookup(outputMock, terraformAllOutputsExpression, nil, false, errMocksAccessDenied)

		_, err := processTagTerraformOutput(atmosConfig, "!terraform.output vpc vpc_id", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})

		require.ErrorIs(t, err, errMocksAccessDenied)
		assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "--use-mocks=always")
	})
}

// TestTerraformOutputComponentMocksFallbackOnMissingOutput covers the non-error miss for
// !terraform.output: the getter succeeds but reports that no output map exists.
func TestTerraformOutputComponentMocksFallbackOnMissingOutput(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		useMocks   bool
		lookupExpr string
		want       any
	}{
		{name: "mock resolves a missing output map", input: "!terraform.output vpc vpc_id", useMocks: true, lookupExpr: terraformAllOutputsExpression, want: "vpc-local"},
		{name: "no mock for the output returns nil like before", input: "!terraform.output vpc missing", useMocks: true, lookupExpr: terraformAllOutputsExpression, want: nil},
		{name: "no mocks declared and a YQ default uses the default", input: `!terraform.output app '.missing // "yq-fallback"'`, useMocks: true, lookupExpr: `.missing // "yq-fallback"`, want: "yq-fallback"},
		{name: "mocks off keeps returning nil", input: "!terraform.output vpc vpc_id", useMocks: false, lookupExpr: "vpc_id", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
			_, outputMock := installMockGetters(t)
			expectOutputLookup(outputMock, tt.lookupExpr, nil, false, nil)

			got, err := processTagTerraformOutput(atmosConfig, tt.input, "dev", &schema.ConfigAndStacksInfo{UseMocks: tt.useMocks})

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTerraformOutputComponentMocksWrapsRecoverableError verifies that, when nothing resolves, the
// original recoverable terraform.output error keeps its component, stack, and output context.
func TestTerraformOutputComponentMocksWrapsRecoverableError(t *testing.T) {
	atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
	_, outputMock := installMockGetters(t)
	expectOutputLookup(outputMock, terraformAllOutputsExpression, nil, false, fmt.Errorf("not yet: %w", errUtils.ErrTerraformStateNotProvisioned))

	got, err := processTagTerraformOutput(atmosConfig, "!terraform.output vpc missing", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})

	require.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to get terraform output for component vpc in stack dev, output missing")
}

// TestTerraformStateComponentMocksDoesNotMutateInputs verifies the mocks and the (possibly cached)
// real output map handed back by the getter are left untouched by the merge, in both directions.
func TestTerraformStateComponentMocksDoesNotMutateInputs(t *testing.T) {
	t.Run("through the state getter", func(t *testing.T) {
		atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
		stateMock, _ := installMockGetters(t)
		realOutputs := map[string]any{"vpc_id": "vpc-real", "extra": map[string]any{"k": "v"}}
		expectStateLookup(stateMock, terraformAllOutputsExpression, realOutputs, nil)

		got, err := processTagTerraformState(atmosConfig, "!terraform.state vpc private_subnet_ids", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})

		require.NoError(t, err)
		assert.Equal(t, []any{"subnet-a", "subnet-b"}, got)
		assert.Equal(t, map[string]any{"vpc_id": "vpc-real", "extra": map[string]any{"k": "v"}}, realOutputs, "real map must not gain the mocked keys")
	})

	t.Run("merge helper isolates the result from both sources", func(t *testing.T) {
		mocks := map[string]any{"vpc_id": "vpc-local", "mock_only": "m"}
		realOutputs := map[string]any{"vpc_id": "vpc-real", "real_only": "r"}
		lookup := &terraformStateLookup{stack: "dev", component: "vpc", output: "vpc_id"}

		got, err := resolveTerraformOutputWithMocks(&schema.AtmosConfiguration{}, lookup, mocks, realOutputs, nil)

		require.NoError(t, err)
		assert.Equal(t, "vpc-real", got)
		assert.Equal(t, map[string]any{"vpc_id": "vpc-local", "mock_only": "m"}, mocks, "mocks must not receive real outputs")
		assert.Equal(t, map[string]any{"vpc_id": "vpc-real", "real_only": "r"}, realOutputs, "real map must not receive mocks")

		// Mutating the sources afterwards must not matter to a later evaluation either.
		mocks["vpc_id"] = "changed"
		realOutputs["vpc_id"] = "changed"
		got, err = resolveTerraformOutputWithMocks(&schema.AtmosConfiguration{}, lookup, map[string]any{"vpc_id": "vpc-local"}, map[string]any{}, nil)
		require.NoError(t, err)
		assert.Equal(t, "vpc-local", got)
	})
}

// TestTerraformAllOutputsExpression verifies the expression used to fetch the whole output map
// returns the full map and is reported as an existing output by the output layer, unlike a bare `.`.
func TestTerraformAllOutputsExpression(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	outputs := map[string]any{
		"vpc_id":             "vpc-real",
		"private_subnet_ids": []any{"subnet-a", "subnet-b"},
		"network":            map[string]any{"cidr": "10.0.0.0/16"},
	}

	t.Run("backend variable returns the whole map", func(t *testing.T) {
		got, err := tb.GetTerraformBackendVariable(atmosConfig, outputs, terraformAllOutputsExpression)
		require.NoError(t, err)
		assert.Equal(t, outputs, got)
	})

	t.Run("output layer reports the whole map as existing", func(t *testing.T) {
		got, exists, err := tfoutput.GetStaticRemoteStateOutput(atmosConfig, "vpc", "dev", outputs, terraformAllOutputsExpression)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, outputs, got)
	})

	t.Run("an empty map still reports as existing", func(t *testing.T) {
		got, exists, err := tfoutput.GetStaticRemoteStateOutput(atmosConfig, "vpc", "dev", map[string]any{}, terraformAllOutputsExpression)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Empty(t, got)
	})
}

func TestMergeRealOverMocks(t *testing.T) {
	mocks := map[string]any{
		"vpc_id":  "vpc-mock",
		"network": map[string]any{"cidr": "10.0.0.0/16", "tags": map[string]any{"env": "mock", "team": "mock"}},
		"subnets": []any{"mock-a", "mock-b"},
		"only":    "mock-only",
	}
	real := map[string]any{
		"vpc_id":  "vpc-real",
		"network": map[string]any{"name": "real", "tags": map[string]any{"env": "real"}},
		"subnets": []any{"real-a"},
		"nulled":  nil,
	}

	merged := mergeRealOverMocks(mocks, real)

	assert.Equal(t, map[string]any{
		"vpc_id":  "vpc-real",
		"network": map[string]any{"name": "real", "cidr": "10.0.0.0/16", "tags": map[string]any{"env": "real", "team": "mock"}},
		"subnets": []any{"real-a"},
		"only":    "mock-only",
		"nulled":  nil,
	}, merged)

	// Neither input is mutated by the merge, and the result does not alias their nested maps.
	assert.Equal(t, map[string]any{"cidr": "10.0.0.0/16", "tags": map[string]any{"env": "mock", "team": "mock"}}, mocks["network"])
	assert.Equal(t, map[string]any{"name": "real", "tags": map[string]any{"env": "real"}}, real["network"])
	merged["network"].(map[string]any)["cidr"] = "changed"
	assert.Equal(t, "10.0.0.0/16", mocks["network"].(map[string]any)["cidr"])
}

func TestResolvedFromMocks(t *testing.T) {
	mocks := map[string]any{"vpc_id": "vpc-mock", "network": map[string]any{"cidr": "10.0.0.0/16"}}
	real := map[string]any{"vpc_id": "vpc-real", "network": map[string]any{"name": "real"}}
	tests := []struct {
		output string
		want   bool
	}{
		{output: "vpc_id", want: false},
		{output: ".network.cidr", want: true},
		{output: ".network.name", want: false},
		{output: "missing", want: false},
		{output: `.network.cidr // "x"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			assert.Equal(t, tt.want, resolvedFromMocks(mocks, real, tt.output))
		})
	}
}

func TestMockPathExists(t *testing.T) {
	mocks := map[string]any{
		"vpc_id":   "vpc-1",
		"nullable": nil,
		"network":  map[string]any{"cidr": "10.0.0.0/16"},
	}
	tests := []struct {
		output         string
		wantExists     bool
		wantDecidable  bool
		wantOutputFlag bool
	}{
		{output: "vpc_id", wantExists: true, wantDecidable: true, wantOutputFlag: true},
		{output: "nullable", wantExists: true, wantDecidable: true, wantOutputFlag: true},
		{output: "missing", wantExists: false, wantDecidable: true, wantOutputFlag: false},
		{output: ".network.cidr", wantExists: true, wantDecidable: true, wantOutputFlag: true},
		{output: ".network.missing", wantExists: false, wantDecidable: true, wantOutputFlag: false},
		{output: ".vpc_id.deeper", wantExists: false, wantDecidable: true, wantOutputFlag: false},
		{output: `.missing // "x"`, wantExists: false, wantDecidable: false, wantOutputFlag: true},
	}
	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			exists, decidable := mockPathExists(mocks, tt.output)
			assert.Equal(t, tt.wantExists, exists)
			assert.Equal(t, tt.wantDecidable, decidable)
			assert.Equal(t, tt.wantOutputFlag, mockOutputExists(mocks, tt.output))
		})
	}
}

// TestResolveTerraformMockOutputInvalidMode verifies that an unknown
// components.terraform.mocks.mode (only reachable through atmos.yaml, since the
// env var and flag paths validate it) fails loudly under --use-mocks instead of
// silently matching neither mode, and is ignored when mocks are off.
func TestResolveTerraformMockOutputInvalidMode(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Mocks.Mode = schema.TerraformMocksMode("bogus")

	t.Run("use-mocks on errors", func(t *testing.T) {
		value, handled, err := resolveTerraformMockOutput(atmosConfig, &schema.ConfigAndStacksInfo{UseMocks: true}, "dev", "vpc", "vpc_id")
		require.ErrorIs(t, err, errUtils.ErrInvalidMocksMode)
		assert.True(t, handled)
		assert.Nil(t, value)
		assert.Contains(t, err.Error(), `"bogus"`)
	})

	t.Run("use-mocks off ignores the mode", func(t *testing.T) {
		value, handled, err := resolveTerraformMockOutput(atmosConfig, &schema.ConfigAndStacksInfo{UseMocks: false}, "dev", "vpc", "vpc_id")
		require.NoError(t, err)
		assert.False(t, handled)
		assert.Nil(t, value)
	})
}
