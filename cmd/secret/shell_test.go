package secret

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateShellArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no args, no separator", args: nil, wantErr: false},
		{name: "shell args after separator", args: []string{"--", "-lc", "env"}, wantErr: false},
		{name: "positional before separator is rejected", args: []string{"bash"}, wantErr: true},
		{name: "positional before separator with args is rejected", args: []string{"bash", "--", "-l"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			require.NoError(t, cmd.ParseFlags(tt.args))

			err := validateShellArgs(cmd)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidArguments)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestSecretShellCommand_Structure(t *testing.T) {
	assert.Equal(t, "shell [-- [shell args...]]", shellCmd.Use)
	assert.NotEmpty(t, shellCmd.Short)
	assert.NotEmpty(t, shellCmd.Long)
	assert.NotEmpty(t, shellCmd.Example)
	assert.NotNil(t, shellCmd.RunE)

	// The --shell flag must be registered.
	assert.NotNil(t, shellCmd.Flags().Lookup(shellFlagName))
}
