package server

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		wantErr     bool
	}{
		{
			name:        "with nil config",
			atmosConfig: nil,
			wantErr:     false,
		},
		{
			name:        "with empty config",
			atmosConfig: &schema.AtmosConfiguration{},
			wantErr:     false,
		},
		{
			name: "with valid config",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/path",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, tt.atmosConfig)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, server)
			assert.NotNil(t, server.server)
			assert.NotNil(t, server.handler)
			assert.NotNil(t, server.atmosConfig)
			assert.Equal(t, ctx, server.ctx)

			// Verify handler is properly initialized.
			handler := server.GetHandler()
			assert.NotNil(t, handler)
			assert.Equal(t, server, handler.server)
			assert.NotNil(t, handler.documents)
		})
	}
}

func TestServer_GetHandler(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	assert.NotNil(t, handler)
	assert.Equal(t, server, handler.server)
}

func TestServer_Shutdown(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	err = server.Shutdown()
	assert.NoError(t, err)
}

func TestServer_ContextPropagation(t *testing.T) {
	// Test that context is properly stored.
	ctx := context.WithValue(context.Background(), "test-key", "test-value")
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	assert.Equal(t, ctx, server.ctx)
	assert.Equal(t, "test-value", server.ctx.Value("test-key"))
}

func TestServer_AtmosConfigDefaults(t *testing.T) {
	// Test that nil config is replaced with empty config.
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	assert.NotNil(t, server.atmosConfig)
	assert.IsType(t, &schema.AtmosConfiguration{}, server.atmosConfig)
}
