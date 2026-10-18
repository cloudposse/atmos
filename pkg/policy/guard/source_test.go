package guard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
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

func TestFileOriginRequiresExistingLocalPath(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	root := t.TempDir()
	file := filepath.Join(root, "stack.yaml")
	require.NoError(t, os.WriteFile(file, []byte("vars: {}"), 0o600))
	origin := FileOrigin(file)
	require.Equal(t, "local", origin.Provenance)
	alias := filepath.Join(root, "alias.yaml")
	require.NoError(t, os.Symlink(file, alias))
	require.Equal(t, origin, FileOrigin(alias), "symlinks preserve the target's attribution")
	require.Equal(t, "unknown", FileOrigin(filepath.Join(root, "missing", "stack.yaml")).Provenance)
	require.Equal(t, "unknown", FileOrigin(file+"/child").Provenance)
	require.NoError(t, os.Remove(file))
	require.Equal(t, "unknown", FileOrigin(alias).Provenance, "dangling symlinks are not local evidence")
}

func TestValueFingerprintsPreserveConservativeAttribution(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	local := Origin{Provenance: "local", SourceFile: "stack.yaml"}
	unknown := Origin{Provenance: "unknown"}
	remote := Origin{Provenance: "remote", SourceFile: "https://example.com/stack.yaml"}
	const value = "password: private-configuration-value"
	RememberValue(value, local)
	require.Equal(t, local, ValueOrigin(value))
	RememberValue(value, unknown)
	RememberValue(value, local)
	require.Equal(t, unknown, ValueOrigin(value), "local attribution cannot override unknown provenance")
	RememberValue(value, remote)
	RememberValue(value, unknown)
	require.Equal(t, remote, ValueOrigin(value), "remote attribution remains the most restrictive")
	require.Equal(t, unknown, ValueOrigin(value+"-different"), "different values cannot share attribution")
	ResetOrigins()
	require.Equal(t, unknown, ValueOrigin(value), "invocations cannot inherit value attribution")
}

func TestRememberNodeRecordsEveryScalar(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("name: top-level\nitems:\n  - first\n  - nested: !exec echo hi\n"), &doc))

	RememberNode(&doc, Origin{Provenance: "remote", SourceFile: "https://example.com/stack.yaml"})

	for _, value := range []string{"top-level", "first", "echo hi"} {
		require.Equal(t, "remote", ValueOrigin(value).Provenance, value)
	}
	require.Equal(t, "unknown", ValueOrigin("never included").Provenance)
	RememberNode(nil, Origin{Provenance: "local"}) // A nil node is ignored.
}
