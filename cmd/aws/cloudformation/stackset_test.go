package cloudformation

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CloudFormationCmd must mount the stackset verb group with its four
// subcommands (create/update/delete/instances).
func TestCloudFormationCmd_RegistersStackSetSubcommand(t *testing.T) {
	var found *cobra.Command
	for _, sub := range CloudFormationCmd.Commands() {
		if sub.Name() == "stackset" {
			found = sub
		}
	}
	require.NotNil(t, found, "expected `atmos aws cloudformation stackset` to be registered")

	names := make([]string, 0, len(found.Commands()))
	for _, sub := range found.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"create", "update", "delete", "instances"}, names)
}

// newStackSetCmd's create/update subcommands must register --auto-approve
// (defaulting false, unlike deploy) and --target; delete must register only
// --auto-approve; instances must register neither.
func TestNewStackSetCmd_RegistersFlags(t *testing.T) {
	cmd := newStackSetCmd()

	var create, update, del, instances *cobra.Command
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "create":
			create = sub
		case "update":
			update = sub
		case "delete":
			del = sub
		case "instances":
			instances = sub
		}
	}
	require.NotNil(t, create)
	require.NotNil(t, update)
	require.NotNil(t, del)
	require.NotNil(t, instances)

	for _, sub := range []*cobra.Command{create, update} {
		autoApprove := sub.Flag(flagAutoApprove)
		require.NotNil(t, autoApprove, "%s must register --auto-approve", sub.Name())
		assert.Equal(t, "false", autoApprove.DefValue)
		assert.NotNil(t, sub.Flag("target"), "%s must register --target", sub.Name())
	}

	deleteAutoApprove := del.Flag(flagAutoApprove)
	require.NotNil(t, deleteAutoApprove, "delete must register --auto-approve")
	assert.Nil(t, del.Flag("target"), "delete must not register --target")

	assert.Nil(t, instances.Flag(flagAutoApprove), "instances is read-only and must not register --auto-approve")
	assert.Nil(t, instances.Flag("target"), "instances must not register --target")
}
