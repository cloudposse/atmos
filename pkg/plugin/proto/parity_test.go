package proto_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratedProtocolParity(t *testing.T) {
	t.Parallel()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	directory := filepath.Dir(source)
	sdk := filepath.Join(directory, "..", "..", "..", "sdk", "proto")
	files := []string{
		filepath.Join("atmos", "api", "v1", "api.pb.go"),
		filepath.Join("atmos", "api", "v1", "api_grpc.pb.go"),
		filepath.Join("atmos", "plugin", "v1", "plugin.pb.go"),
		filepath.Join("atmos", "plugin", "v1", "plugin_grpc.pb.go"),
	}
	for _, file := range files {
		cli, err := os.ReadFile(filepath.Join(directory, file))
		require.NoError(t, err)
		author, err := os.ReadFile(filepath.Join(sdk, file))
		require.NoError(t, err)
		require.Equal(t, string(cli), string(author), "regenerate both bindings using atmos dev generate proto")
	}
}
