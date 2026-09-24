package list

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Inventory must not authenticate just because processing is enabled.
func TestDeferredDefersDefaultAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)
	original := listAuthManagerFactory
	t.Cleanup(func() { listAuthManagerFactory = original })
	listAuthManagerFactory = NewMockAuthManagerFactory(ctrl)
	cmd := &cobra.Command{Use: "stacks"}
	cmd.Flags().String("identity", "", "identity")
	require.NoError(t, cmd.Flags().Set("identity", ""))
	config := &schema.AtmosConfiguration{}
	manager, err := createAuthManagerForList(cmd, config, true, true)
	require.NoError(t, err)
	require.Nil(t, manager)
}

func TestDeferredStackInventoryFormats(t *testing.T) {
	initExecutorTestIO(t)
	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "list-deferred-auth"))
	ctrl := gomock.NewController(t)
	original := listAuthManagerFactory
	t.Cleanup(func() { listAuthManagerFactory = original })
	listAuthManagerFactory = NewMockAuthManagerFactory(ctrl)
	for _, format := range []string{"", "table", "json", "yaml", "tree"} {
		t.Run(format, func(t *testing.T) {
			e.ClearFindStacksMapCache()
			cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
			require.NoError(t, cmd.Flags().Set("identity", ""))
			opts := &StacksOptions{Format: format, ErrorMode: "strict", ProcessTemplates: true, ProcessFunctions: true}
			ac, manager, err := initStacksConfig(cmd, nil, opts)
			require.NoError(t, err)
			errOpts, _ := describeStacksErrorOptions(opts.ErrorMode)
			rows, _, err := executeAndExtractStacks(&ac, opts, manager, errOpts)
			require.NoError(t, err)
			require.Len(t, rows, 3)
			names := []any{rows[0]["stack"], rows[1]["stack"], rows[2]["stack"]}
			require.ElementsMatch(t, []any{"dev", "prod", "staging"}, names)
			require.NoError(t, listStacksWithOptions(cmd, nil, opts))
			// A literal column must not evaluate a credential-backed sibling in vars.
			opts.Columns = []string{"Stack={{ .stack }}", "Literal={{ .vars.literal }}"}
			require.NoError(t, listStacksWithOptions(cmd, nil, opts))
		})
	}
}
