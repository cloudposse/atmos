package component

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// identityProvider returns a provider whose executable and schema live in one private directory,
// so tests can change their bytes without touching the shared test binary.
func identityProvider(t *testing.T) *Provider {
	t.Helper()
	provider := fixtureProvider(t)
	require.NoError(t, os.WriteFile(filepath.Join(provider.directory, "plugin.bin"), []byte("executable-v1"), 0o600))
	provider.executable = "plugin.bin"
	return provider
}

func TestProviderIdentityBindsExecutableSchemaAndNegotiatedMetadata(t *testing.T) {
	t.Parallel()
	provider := identityProvider(t)
	baseline, err := provider.Identity(t.Context())
	require.NoError(t, err)
	require.Len(t, baseline, 64, "identity is a hex SHA-256 digest")

	again, err := provider.Identity(t.Context())
	require.NoError(t, err)
	require.Equal(t, baseline, again, "unchanged inputs must produce an identical identity")

	for name, change := range map[string]func(*testing.T, *Provider){
		"executable bytes": func(t *testing.T, p *Provider) {
			require.NoError(t, os.WriteFile(filepath.Join(p.directory, "plugin.bin"), []byte("executable-v2"), 0o600))
		},
		"schema bytes": func(t *testing.T, p *Provider) {
			require.NoError(t, os.WriteFile(filepath.Join(p.directory, p.schemaPath), []byte(`{"type":"object"}`), 0o600))
		},
		"negotiated manifest": func(_ *testing.T, p *Provider) {
			p.expected = proto.Clone(p.expected).(*pluginv1.DescribeResponse)
			p.expected.ManifestDigest = "sha256:other"
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			provider := identityProvider(t)
			before, err := provider.Identity(t.Context())
			require.NoError(t, err)
			change(t, provider)
			after, err := provider.Identity(t.Context())
			require.NoError(t, err)
			require.NotEqual(t, before, after)
		})
	}
}

func TestProviderIdentityResolvesAbsoluteAndRelativePathsIdentically(t *testing.T) {
	t.Parallel()
	relative := identityProvider(t)
	absolute := identityProvider(t)
	absolute.directory = relative.directory
	absolute.expected = proto.Clone(relative.expected).(*pluginv1.DescribeResponse)
	absolute.schemaPath = relative.schemaPath
	absolute.executable = filepath.Join(relative.directory, "plugin.bin")

	want, err := relative.Identity(t.Context())
	require.NoError(t, err)
	got, err := absolute.Identity(t.Context())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestProviderIdentityFailsClosed(t *testing.T) {
	t.Parallel()
	t.Run("missing executable", func(t *testing.T) {
		t.Parallel()
		provider := identityProvider(t)
		provider.executable = "absent.bin"
		_, err := provider.Identity(t.Context())
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("missing schema", func(t *testing.T) {
		t.Parallel()
		provider := identityProvider(t)
		require.NoError(t, os.Remove(filepath.Join(provider.directory, provider.schemaPath)))
		_, err := provider.Identity(t.Context())
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("executable is a directory", func(t *testing.T) {
		t.Parallel()
		provider := identityProvider(t)
		require.NoError(t, os.Mkdir(filepath.Join(provider.directory, "dir.bin"), 0o700))
		provider.executable = "dir.bin"
		_, err := provider.Identity(t.Context())
		require.Error(t, err, "a directory cannot be hashed as executable bytes")
	})
	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := identityProvider(t).Identity(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})
}
