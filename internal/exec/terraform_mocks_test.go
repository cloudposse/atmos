package exec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not declare `mocks`")

	_, err = processTagTerraformOutput(&atmosConfig, "!terraform.output vpc missing", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not declared")

	_, err = processTagTerraformOutput(&atmosConfig, "!terraform.output vpc '.network.missing'", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not declared")
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

// expectStateLookup registers one GetState call returning (value, err).
func expectStateLookup(stateMock *MockTerraformStateGetter, value any, err error) {
	stateMock.EXPECT().
		GetState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(value, err).
		Times(1)
}

// expectOutputLookup registers one GetOutput call returning (value, exists, err).
func expectOutputLookup(outputMock *MockTerraformOutputGetter, value any, exists bool, err error) {
	outputMock.EXPECT().
		GetOutput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
	// lookupValue / lookupExists / lookupErr are what the real getter returns; skipLookup means
	// the getter must not be called at all (always mode).
	lookupValue  any
	lookupExists bool
	lookupErr    error
	skipLookup   bool
	want         any
	wantErrIs    error
	wantErrText  string
	notErrText   string
}

func mocksFallbackCases(function string) []mocksFallbackCase {
	notProvisioned := fmt.Errorf("component not provisioned: %w", errUtils.ErrTerraformStateNotProvisioned)
	outputMissing := fmt.Errorf("no such output: %w", errUtils.ErrTerraformOutputNotFound)
	return []mocksFallbackCase{
		{
			name: "fallback: real value wins even when the mock is declared", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc vpc_id", lookupValue: "vpc-real", lookupExists: true, want: "vpc-real",
		},
		{
			name: "fallback: real value for a complex expression wins", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc '.network.cidr'", lookupValue: "192.168.0.0/16", lookupExists: true, want: "192.168.0.0/16",
		},
		{
			name: "fallback: not provisioned resolves from the mock", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc vpc_id", lookupErr: notProvisioned, want: "vpc-local",
		},
		{
			name: "fallback: output not found resolves from the mock", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc vpc_id", lookupErr: outputMissing, want: "vpc-local",
		},
		{
			name: "fallback: nested mock path resolves on a miss", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc '.network.cidr'", lookupErr: notProvisioned, want: "10.0.0.0/16",
		},
		{
			name: "fallback: mock beats the YQ default", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + ` vpc '.vpc_id // "default"'`, lookupErr: notProvisioned, want: "vpc-local",
		},
		{
			name: "fallback: explicit null mock is a hit", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc nullable_output", lookupErr: notProvisioned, want: nil,
		},
		{
			name: "fallback: no mock declared and a YQ default uses the default", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + ` app '.missing // "yq-fallback"'`, lookupErr: notProvisioned, want: "yq-fallback",
		},
		{
			name: "fallback: mocks do not declare the output and a YQ default uses the default", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + ` vpc '.missing // "yq-fallback"'`, lookupErr: notProvisioned, want: "yq-fallback",
		},
		{
			name: "fallback: no mock declared and no default keeps the original error", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " app vpc_id", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
			notErrText: "does not declare `mocks`",
		},
		{
			name: "fallback: mock does not declare the output and no default keeps the original error", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " vpc missing", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
			notErrText: "is not declared",
		},
		{
			name: "fallback: non-recoverable error is returned and the mock is not consulted", mode: schema.TerraformMocksModeFallback, useMocks: true,
			// A component that cannot be described proves the mock lookup never ran.
			input: function + " does-not-exist vpc_id", lookupErr: errMocksAccessDenied, wantErrIs: errMocksAccessDenied,
			notErrText: "failed to load mocks",
		},
		{
			name: "fallback: mock loading failure keeps the original sentinel", mode: schema.TerraformMocksModeFallback, useMocks: true,
			input: function + " does-not-exist vpc_id", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
			wantErrText: "failed to load mocks",
		},
		{
			name: "mocks off: a recoverable miss never reads mocks even in fallback mode", mode: schema.TerraformMocksModeFallback, useMocks: false,
			input: function + " vpc vpc_id", lookupErr: notProvisioned, wantErrIs: errUtils.ErrTerraformStateNotProvisioned,
		},
		{
			name: "always: mock resolves without calling the real getter", mode: schema.TerraformMocksModeAlways, useMocks: true,
			input: function + " vpc vpc_id", skipLookup: true, want: "vpc-local",
		},
		{
			name: "always: missing mock is an error without calling the real getter", mode: schema.TerraformMocksModeAlways, useMocks: true,
			input: function + " vpc missing", skipLookup: true, wantErrText: "is not declared",
		},
		{
			name: "always: undeclared mocks map is an error without calling the real getter", mode: schema.TerraformMocksModeAlways, useMocks: true,
			input: function + " app vpc_id", skipLookup: true, wantErrIs: errUtils.ErrTerraformComponentMocksNotDeclared,
		},
	}
}

func assertMocksFallbackResult(t *testing.T, tt *mocksFallbackCase, got any, err error) {
	t.Helper()

	if tt.wantErrIs == nil && tt.wantErrText == "" {
		require.NoError(t, err)
		assert.Equal(t, tt.want, got)
		return
	}
	require.Error(t, err)
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
				expectStateLookup(stateMock, tt.lookupValue, tt.lookupErr)
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
				expectOutputLookup(outputMock, tt.lookupValue, tt.lookupExists, tt.lookupErr)
			}

			got, err := processTagTerraformOutput(atmosConfig, tt.input, "dev", &schema.ConfigAndStacksInfo{UseMocks: tt.useMocks})

			assertMocksFallbackResult(t, &tt, got, err)
		})
	}
}

// TestTerraformOutputComponentMocksFallbackOnMissingOutput covers the non-error miss for
// !terraform.output: the getter succeeds but reports the output does not exist.
func TestTerraformOutputComponentMocksFallbackOnMissingOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		useMocks bool
		want     any
	}{
		{name: "mock resolves a missing output", input: "!terraform.output vpc vpc_id", useMocks: true, want: "vpc-local"},
		{name: "no mock for the output returns nil like before", input: "!terraform.output vpc missing", useMocks: true, want: nil},
		{name: "no mock and a YQ default uses the default", input: `!terraform.output app '.missing // "yq-fallback"'`, useMocks: true, want: "yq-fallback"},
		{name: "mocks off keeps returning nil", input: "!terraform.output vpc vpc_id", useMocks: false, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
			_, outputMock := installMockGetters(t)
			expectOutputLookup(outputMock, nil, false, nil)

			got, err := processTagTerraformOutput(atmosConfig, tt.input, "dev", &schema.ConfigAndStacksInfo{UseMocks: tt.useMocks})

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTerraformStateComponentMocksFallbackOnNilDirectOutput covers Terraform dropping a null output
// from state: a nil result for a direct output name is a recoverable miss, but a nil result for a
// complex expression is left alone.
func TestTerraformStateComponentMocksFallbackOnNilDirectOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{name: "direct output name resolves from the mock", input: "!terraform.state vpc vpc_id", want: "vpc-local"},
		{name: "complex expression keeps the nil result", input: "!terraform.state vpc '.network.cidr'", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := newMocksFixtureConfig(t, schema.TerraformMocksModeFallback)
			stateMock, _ := installMockGetters(t)
			expectStateLookup(stateMock, nil, nil)

			got, err := processTagTerraformState(atmosConfig, tt.input, "dev", &schema.ConfigAndStacksInfo{UseMocks: true})

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
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
