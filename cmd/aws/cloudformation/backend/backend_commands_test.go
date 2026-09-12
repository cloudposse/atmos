package backend

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// setupTestWithMocks injects mock ConfigInitializer/Provisioner
// implementations and registers cleanup, mirroring
// cmd/terraform/backend/backend_commands_test.go's helper of the same name.
// Only the ConfigInitializer mock is returned: every RunE case below stops
// (via an injected InitConfigAndAuth error) before the Provisioner would be
// invoked, so the mock Provisioner only needs to be installed, never set
// expectations on.
func setupTestWithMocks(t *testing.T) *MockConfigInitializer {
	t.Helper()

	ctrl := gomock.NewController(t)

	mockConfigInit := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)

	SetConfigInitializer(mockConfigInit)
	SetProvisioner(mockProv)

	t.Cleanup(func() {
		ResetDependencies()
		ctrl.Finish()
	})

	return mockConfigInit
}

// setupViperForTest resets Viper and sets test values, restoring the prior
// global state afterward.
func setupViperForTest(t *testing.T, values map[string]any) {
	t.Helper()

	oldViper := viper.GetViper()
	oldKeys := make(map[string]any)
	for _, key := range oldViper.AllKeys() {
		oldKeys[key] = oldViper.Get(key)
	}

	viper.Reset()
	for k, v := range values {
		viper.Set(k, v)
	}

	t.Cleanup(func() {
		viper.Reset()
		for key, val := range oldKeys {
			viper.Set(key, val)
		}
	})
}

// backendSubcommandCase is a table entry naming one of the five backend
// verbs' cobra.Command and the args it expects, shared by the RunE-exercising
// tests below.
type backendSubcommandCase struct {
	name      string
	cmd       *cobra.Command
	args      []string
	component string
}

func backendSubcommandCases() []backendSubcommandCase {
	return []backendSubcommandCase{
		{name: "create", cmd: createCmd, args: []string{"vpc"}, component: "vpc"},
		{name: "update", cmd: updateCmd, args: []string{"vpc"}, component: "vpc"},
		{name: "delete", cmd: deleteCmd, args: []string{"vpc"}, component: "vpc"},
		{name: "describe", cmd: describeCmd, args: []string{"vpc"}, component: "vpc"},
		{name: "list", cmd: listCmd, args: []string{"vpc"}, component: "vpc"},
	}
}

// TestBackendSubcommands_BindStackFlagFromCommand exercises every verb's real
// RunE closure (flag binding, StandardParser.Parse, target/stack/identity
// resolution) up through the ConfigInitializer.InitConfigAndAuth call, then
// stops via an injected error — proving the CLI wiring itself (not just the
// executeX helper functions) reaches the config-init step with the expected
// component/stack.
func TestBackendSubcommands_BindStackFlagFromCommand(t *testing.T) {
	for _, tt := range backendSubcommandCases() {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit := setupTestWithMocks(t)
			setupViperForTest(t, map[string]any{
				"stack":    "",
				"identity": "",
				"force":    false,
				"target":   "",
				"format":   "table",
			})

			expectedErr := errors.New("stop after stack parse")
			mockConfigInit.EXPECT().
				InitConfigAndAuth(tt.component, "dev", "").
				Return(nil, nil, expectedErr)

			require.NoError(t, tt.cmd.Flags().Set("stack", "dev"))
			if tt.name == "delete" {
				require.NoError(t, tt.cmd.Flags().Set("force", "true"))
			}

			err := tt.cmd.RunE(tt.cmd, tt.args)
			require.Error(t, err)
			assert.ErrorIs(t, err, expectedErr)
			assert.NotErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
		})
	}
}

// TestBackendSubcommands_StackFromViperWhenNotSetOnCLI covers the fallback
// path where --stack (and --force, for delete) is not explicitly passed on
// the CLI but is available via Viper instead.
func TestBackendSubcommands_StackFromViperWhenNotSetOnCLI(t *testing.T) {
	for _, tt := range backendSubcommandCases() {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit := setupTestWithMocks(t)
			setupViperForTest(t, map[string]any{
				"stack":    "dev",
				"identity": "",
				"force":    true,
				"target":   "",
				"format":   "table",
			})

			// These are package-level singleton *cobra.Command values shared across test
			// functions; clear any Changed state a prior subtest may have left set so this
			// test reliably exercises the "value came from Viper, not the CLI" fallback.
			for _, name := range []string{"stack", "identity", "force", "target", "format"} {
				if f := tt.cmd.Flags().Lookup(name); f != nil {
					f.Changed = false
				}
			}

			expectedErr := errors.New("stop after stack parse")
			mockConfigInit.EXPECT().
				InitConfigAndAuth(tt.component, "dev", "").
				Return(nil, nil, expectedErr)

			err := tt.cmd.RunE(tt.cmd, tt.args)
			require.Error(t, err)
			assert.ErrorIs(t, err, expectedErr)
			assert.NotErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
		})
	}
}
