package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyDirectoryDiscovery(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())
	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Empty(t, directory)
	require.NoError(t, os.Mkdir("policy.d", 0o700))
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	want, err := filepath.Abs("policy.d")
	require.NoError(t, err)
	require.Equal(t, want, directory)
	t.Setenv("ATMOS_POLICY_DIR", "explicit")
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Equal(t, "explicit", directory)
}

func TestPolicyDirectoryDiscoveryRejectsFile(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("policy.d", []byte("not a directory"), 0o600))
	_, err := DirectoryFromEnvironment()
	require.Error(t, err)
}
