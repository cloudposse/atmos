package secret

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestSecretCommandsCarryCommandPath verifies that `secret shell` and `secret exec`
// scope their component describes with their own command path, so component
// admission sees `shell`/`exec` instead of an empty command.
func TestSecretCommandsCarryCommandPath(t *testing.T) {
	for _, tc := range []struct {
		sub  string
		args []string
	}{
		{sub: "shell"},
		{sub: "exec", args: []string{"--", "true"}},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			setupIO(t)
			svc := newFakeSecretService()
			svc.declarations = []secrets.Declaration{{Name: "API_KEY"}}
			svc.getValues["API_KEY"] = "s3cr3t"
			var got secretScope
			orig := loadServiceAndConfigFn
			loadServiceAndConfigFn = func(scope secretScope) (secretService, *schema.AtmosConfiguration, error) {
				got = scope
				return svc, &schema.AtmosConfiguration{}, nil
			}
			t.Cleanup(func() { loadServiceAndConfigFn = orig })
			overrideStartShell(t, nil)
			overrideRunCommand(t, nil)

			args := append([]string{tc.sub, "--stack", "prod", "--component", "api"}, tc.args...)
			require.NoError(t, runSecretSubcommand(t, args...))
			// The harness runs secretCmd as its own root, so the path is relative to it;
			// under `atmos` it is ["secret", tc.sub] (see TestPolicyCommandPath).
			require.NotEmpty(t, got.CommandPath)
			require.Equal(t, tc.sub, got.CommandPath[len(got.CommandPath)-1])
		})
	}
}
