package execution

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCommandNameMatchesTheCommandNotItsSubcommand(t *testing.T) {
	for _, test := range []struct {
		command string
		flags   []string
		want    string
	}{
		{"plan", nil, "plan"},
		{"plan", []string{"-destroy"}, "plan"},
		{"apply", []string{"-destroy"}, "destroy"},
		{"destroy", nil, "destroy"},
		{"state list", nil, "state"},
		{"state rm", []string{"x"}, "state"},
		{"workspace", nil, "workspace"},
		{"", nil, ""},
	} {
		require.Equal(t, test.want, CommandName(test.command, test.flags), test.command)
	}
}

func TestRequiredVarFileErrorsSayWhereARelativeNameWasLooked(t *testing.T) {
	directory := t.TempDir()
	items := []schema.ComponentExtraArguments{{Commands: []string{"plan"}, RequiredVarFiles: []string{"vars.tfvars"}}}

	_, err := ApplyExtraArguments(items, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1, WorkingDirectory: directory})
	require.ErrorIs(t, err, errUtils.ErrComponentVarFileMissing)
	require.Contains(t, err.Error(), "vars.tfvars")
	require.Contains(t, err.Error(), directory, "the directory the name was resolved against must be named")
	require.Contains(t, err.Error(), "get_terragrunt_dir()")

	absolute := filepath.Join(directory, "absent.tfvars")
	items[0].RequiredVarFiles = []string{absolute}
	_, err = ApplyExtraArguments(items, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1, WorkingDirectory: directory})
	require.ErrorIs(t, err, errUtils.ErrComponentVarFileMissing)
	require.Contains(t, err.Error(), absolute)
	require.NotContains(t, err.Error(), "get_terragrunt_dir()")

	require.NoError(t, os.WriteFile(absolute, nil, 0o600))
	_, err = ApplyExtraArguments(items, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1, WorkingDirectory: directory})
	require.NoError(t, err)
}
