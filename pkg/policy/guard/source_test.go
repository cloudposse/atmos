package guard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceNormalization(t *testing.T) {
	for _, tc := range []struct{ raw, scheme, host string }{
		{"git::https://user:secret@EXAMPLE.com./org/repo?token=secret", "https", "example.com"},
		{"git@example.com:org/repo", "ssh", "example.com"},
		{"github.com/org/repo", "https", "github.com"},
		{"s3::https://bucket.example.com/object", "https", "bucket.example.com"},
	} {
		input, err := DescribeSource("download", tc.raw, "")
		require.NoError(t, err)
		require.Equal(t, tc.scheme, input.Scheme)
		require.Equal(t, tc.host, input.Host)
		require.True(t, input.Remote)
		require.NotContains(t, input.Path, "secret")
	}
}

func TestRemoteProvenanceSurvivesCacheAndMerge(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	root := t.TempDir()
	cached := filepath.Join(root, "remote")
	require.NoError(t, os.Mkdir(cached, 0o700))
	RegisterRemote(cached, "https://user:secret@example.com/repo?token=secret")
	remote := FileOrigin(filepath.Join(cached, "nested", "stack.yaml"))
	require.Equal(t, "remote", remote.Provenance)
	require.NotContains(t, remote.SourceFile, "secret")
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(cached, alias))
	require.Equal(t, "remote", FileOrigin(filepath.Join(alias, "nested", "stack.yaml")).Provenance)
	local := FileOrigin(root)
	require.Equal(t, "local", local.Provenance)
	RememberValue("!exec identical", remote)
	RememberValue("!exec identical", local)
	require.Equal(t, "remote", ValueOrigin("!exec identical").Provenance)
	require.Equal(t, "unknown", ValueOrigin("!exec missing").Provenance)
	require.Equal(t, "unknown", FileOrigin("").Provenance)
	ResetOrigins()
	require.Equal(t, "unknown", ValueOrigin("!exec identical").Provenance)
}
