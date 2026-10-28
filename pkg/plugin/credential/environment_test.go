package credential

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestStringCredentialContracts(t *testing.T) {
	oversized := make(map[string]any)
	for i := range 257 {
		oversized[fmt.Sprintf("KEY_%d", i)] = "value"
	}
	tests := []struct {
		name                          string
		data                          map[string]any
		validSecret, validEnvironment bool
	}{
		{"strings", map[string]any{"TOKEN": "private-token", "EMPTY": ""}, true, true},
		{"logical names", map[string]any{"logical-name": "value"}, true, false},
		{"case collision", map[string]any{"Token": "one", "TOKEN": "two"}, true, false},
		{"empty", map[string]any{}, false, false},
		{"too many", oversized, false, false},
		{"non-string", map[string]any{"TOKEN": 1}, false, false},
		{"empty name", map[string]any{"": "value"}, false, false},
		{"long name", map[string]any{strings.Repeat("a", 257): "value"}, false, false},
		{"nul value", map[string]any{"TOKEN": "a\x00b"}, false, false},
		{"large value", map[string]any{"TOKEN": strings.Repeat("a", (1<<20)+1)}, false, false},
	}
	for _, tt := range tests {
		for _, kind := range []string{"secret-values/v1", "environment/v1"} {
			t.Run(tt.name+"/"+kind, func(t *testing.T) {
				data, err := structpb.NewStruct(tt.data)
				require.NoError(t, err)
				envelope := &pluginv1.Credentials{Type: kind, Data: data, ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))}
				decoded, err := New().Decode(envelope, []string{kind})
				valid := tt.validSecret
				if kind == "environment/v1" {
					valid = tt.validEnvironment
				}
				if !valid {
					require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
					return
				}
				require.NoError(t, err)
				credentials := decoded.(*types.PluginCredentials)
				require.Equal(t, tt.data, credentials.Data)
				require.Equal(t, kind, credentials.Type)
				for key := range tt.data {
					require.Contains(t, credentials.SensitiveKeys, key)
				}
			})
		}
	}
}
