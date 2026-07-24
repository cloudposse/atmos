package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listAuthManagerSentinel is only compared by pointer in these policy tests; no
// AuthManager methods are expected to run.
type listAuthManagerSentinel struct{ auth.AuthManager }

func TestCreateAuthManagerForList_EvaluationPolicy(t *testing.T) {
	originalFactory := createAuthManagerWithStackScan
	t.Cleanup(func() { createAuthManagerWithStackScan = originalFactory })

	sentinel := &listAuthManagerSentinel{}
	var calls int
	var gotIdentity string
	createAuthManagerWithStackScan = func(
		identity string,
		_ *schema.AuthConfig,
		_ string,
		_ *schema.AtmosConfiguration,
	) (auth.AuthManager, error) {
		calls++
		gotIdentity = identity
		return sentinel, nil
	}

	newCommand := func(identity string) *cobra.Command {
		cmd := &cobra.Command{Use: "list-test"}
		cmd.PersistentFlags().String(cfg.IdentityFlagName, "", "identity")
		// Set the empty value too: this isolates the no-identity cases from Viper
		// state established by unrelated command tests in the same package.
		require.NoError(t, cmd.PersistentFlags().Set(cfg.IdentityFlagName, identity))
		return cmd
	}

	tests := []struct {
		name                 string
		identity             string
		processTemplates     bool
		processYamlFunctions bool
		wantCalls            int
		wantIdentity         string
	}{
		{
			name:             "templates enable default identity discovery",
			processTemplates: true,
			wantCalls:        1,
		},
		{
			name:                 "yaml functions enable default identity discovery",
			processYamlFunctions: true,
			wantCalls:            1,
		},
		{
			name:      "no evaluation remains credential free",
			wantCalls: 0,
		},
		{
			name:         "explicit identity authenticates without evaluation",
			identity:     "operator",
			wantCalls:    1,
			wantIdentity: "operator",
		},
		{
			name:             "explicit disable overrides template evaluation",
			identity:         "false",
			processTemplates: true,
			wantCalls:        0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls = 0
			gotIdentity = ""

			got, err := createAuthManagerForList(
				newCommand(tc.identity),
				&schema.AtmosConfiguration{},
				tc.processTemplates,
				tc.processYamlFunctions,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCalls, calls)
			assert.Equal(t, tc.wantIdentity, gotIdentity)
			if tc.wantCalls == 0 {
				assert.Nil(t, got)
			} else {
				assert.Same(t, sentinel, got)
			}
		})
	}
}
