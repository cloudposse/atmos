package proto_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
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

// The ignored-run fields are additive: they use numbers above every field of the message, and an
// ignored run survives the wire while a response that never sets them reads as not ignored.
func TestPrepareComponentResponseIgnoredRunRoundTrip(t *testing.T) {
	t.Parallel()
	fields := (&pluginv1.PrepareComponentResponse{}).ProtoReflect().Descriptor().Fields()
	for name, number := range map[string]int32{"run_ignored": 23, "ignored_policy": 24, "ignored_message": 25} {
		field := fields.ByName(protoreflect.Name(name))
		require.NotNil(t, field, name)
		require.EqualValues(t, number, field.Number(), name)
	}

	data, err := proto.Marshal(&pluginv1.PrepareComponentResponse{Engine: "terraform", RunIgnored: true, IgnoredPolicy: "registry", IgnoredMessage: "down"})
	require.NoError(t, err)
	var got pluginv1.PrepareComponentResponse
	require.NoError(t, proto.Unmarshal(data, &got))
	require.True(t, got.GetRunIgnored())
	require.Equal(t, "registry", got.GetIgnoredPolicy())
	require.Equal(t, "down", got.GetIgnoredMessage())
	require.Equal(t, "terraform", got.GetEngine())

	data, err = proto.Marshal(&pluginv1.PrepareComponentResponse{Engine: "terraform"})
	require.NoError(t, err)
	var plain pluginv1.PrepareComponentResponse
	require.NoError(t, proto.Unmarshal(data, &plain))
	require.False(t, plain.GetRunIgnored())
	require.Empty(t, plain.GetIgnoredPolicy())
}
