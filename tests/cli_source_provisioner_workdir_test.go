package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
)

// TestSourceWorkdir_SourceOnly tests source describe for component with source but no workdir.
func TestSourceWorkdir_SourceOnly(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-remote", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_SourceWithWorkdir tests source describe for component with both source and workdir.
func TestSourceWorkdir_SourceWithWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-remote-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_LocalWithWorkdir_NoSource tests that local component with workdir has no source.
func TestSourceWorkdir_LocalWithWorkdir_NoSource(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "mock-workdir", "--stack", "dev"})

	err := cmd.Execute()
	// Should return error because component has no source configured.
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "source") || strings.Contains(err.Error(), "uri"),
		"Expected error about missing source")
}

// TestSourceWorkdir_DescribeComponent_SourceOnly tests describe component shows source config.
func TestSourceWorkdir_DescribeComponent_SourceOnly(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"describe", "component", "vpc-remote", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_DescribeComponent_SourceWithWorkdir tests describe component shows both configs.
func TestSourceWorkdir_DescribeComponent_SourceWithWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"describe", "component", "vpc-remote-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_DescribeComponent_LocalWithWorkdir tests describe component shows workdir config.
func TestSourceWorkdir_DescribeComponent_LocalWithWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"describe", "component", "mock-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_DeleteMissingForce tests that delete requires --force flag.
func TestSourceWorkdir_DeleteMissingForce(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "delete", "vpc-remote-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "force") || strings.Contains(err.Error(), "--force"),
		"Expected error about missing --force flag")
}
