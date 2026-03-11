package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestNewHandler(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := NewHandler(server)

	assert.NotNil(t, handler)
	assert.Equal(t, server, handler.server)
	assert.NotNil(t, handler.documents)
	assert.False(t, handler.initialized)
}

func TestHandler_Initialize(t *testing.T) {
	tests := []struct {
		name        string
		params      *protocol.InitializeParams
		wantErr     bool
		checkResult func(t *testing.T, result any)
	}{
		{
			name:    "with nil params",
			params:  nil,
			wantErr: false,
			checkResult: func(t *testing.T, result any) {
				initResult, ok := result.(protocol.InitializeResult)
				require.True(t, ok)
				assert.NotNil(t, initResult.Capabilities)
				assert.NotNil(t, initResult.ServerInfo)
				assert.Equal(t, "atmos-lsp", initResult.ServerInfo.Name)
			},
		},
		{
			name: "with valid params",
			params: &protocol.InitializeParams{
				RootURI: strPtr("file:///test/path"),
			},
			wantErr: false,
			checkResult: func(t *testing.T, result any) {
				initResult, ok := result.(protocol.InitializeResult)
				require.True(t, ok)
				assert.NotNil(t, initResult.Capabilities)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			result, err := handler.Initialize(glspContext, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestHandler_InitializeCapabilities(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	result, err := handler.Initialize(glspContext, nil)
	require.NoError(t, err)

	initResult, ok := result.(protocol.InitializeResult)
	require.True(t, ok)

	caps := initResult.Capabilities

	// Verify text document sync capabilities.
	assert.NotNil(t, caps.TextDocumentSync)

	// Verify completion provider is advertised.
	assert.NotNil(t, caps.CompletionProvider)

	// Verify hover provider is advertised.
	assert.NotNil(t, caps.HoverProvider)

	// Verify definition provider is advertised (stub).
	assert.NotNil(t, caps.DefinitionProvider)

	// Verify document symbols and file operations are NOT advertised.
	// These were removed per verification report.
	assert.Nil(t, caps.DocumentSymbolProvider)
	assert.Nil(t, caps.Workspace)
}

func TestHandler_Initialized(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	// Initially not initialized.
	assert.False(t, handler.IsInitialized())

	// Call Initialized.
	err = handler.Initialized(glspContext, &protocol.InitializedParams{})
	require.NoError(t, err)

	// Now initialized.
	assert.True(t, handler.IsInitialized())
}

func TestHandler_Shutdown(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	// Initialize first.
	err = handler.Initialized(glspContext, &protocol.InitializedParams{})
	require.NoError(t, err)
	assert.True(t, handler.IsInitialized())

	// Shutdown.
	err = handler.Shutdown(glspContext)
	require.NoError(t, err)

	// No longer initialized.
	assert.False(t, handler.IsInitialized())
}

func TestHandler_SetTrace(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	tests := []struct {
		name   string
		params *protocol.SetTraceParams
	}{
		{
			name:   "set trace off",
			params: &protocol.SetTraceParams{Value: "off"},
		},
		{
			name:   "set trace messages",
			params: &protocol.SetTraceParams{Value: "messages"},
		},
		{
			name:   "set trace verbose",
			params: &protocol.SetTraceParams{Value: "verbose"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.SetTrace(glspContext, tt.params)
			assert.NoError(t, err)
		})
	}
}

func TestHandler_IsInitialized(t *testing.T) {
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()

	// Initially false.
	assert.False(t, handler.IsInitialized())

	// Set initialized manually for testing.
	handler.mu.Lock()
	handler.initialized = true
	handler.mu.Unlock()

	assert.True(t, handler.IsInitialized())

	// Set back to false.
	handler.mu.Lock()
	handler.initialized = false
	handler.mu.Unlock()

	assert.False(t, handler.IsInitialized())
}

func TestHandler_ConcurrentAccess(t *testing.T) {
	// Test concurrent access to initialized flag.
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	done := make(chan bool, 10)

	// Spawn 5 goroutines setting initialized.
	for i := 0; i < 5; i++ {
		go func() {
			_ = handler.Initialized(glspContext, &protocol.InitializedParams{})
			done <- true
		}()
	}

	// Spawn 5 goroutines checking initialized.
	for i := 0; i < 5; i++ {
		go func() {
			_ = handler.IsInitialized()
			done <- true
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be initialized at the end.
	assert.True(t, handler.IsInitialized())
}

// Helper functions.
func strPtr(s string) *string {
	return &s
}
