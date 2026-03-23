package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfgName   string
		cfg       schema.MCPServerConfig
		wantErr   error
		checkFunc func(t *testing.T, pc *ParsedConfig)
	}{
		{
			name:    "valid config with all fields",
			cfgName: "aws-eks",
			cfg: schema.MCPServerConfig{
				Description: "Amazon EKS",
				Command:     "uvx",
				Args:        []string{"awslabs.amazon-eks-mcp-server@latest"},
				Env:         map[string]string{"AWS_REGION": "us-east-1"},
				AutoStart:   true,
				Timeout:     "45s",
			},
			checkFunc: func(t *testing.T, pc *ParsedConfig) {
				t.Helper()
				assert.Equal(t, "aws-eks", pc.Name)
				assert.Equal(t, "Amazon EKS", pc.Description)
				assert.Equal(t, "uvx", pc.Command)
				assert.Equal(t, []string{"awslabs.amazon-eks-mcp-server@latest"}, pc.Args)
				assert.Equal(t, "us-east-1", pc.Env["AWS_REGION"])
				assert.True(t, pc.AutoStart)
				assert.Equal(t, 45*time.Second, pc.Timeout)
			},
		},
		{
			name:    "default timeout when empty",
			cfgName: "test",
			cfg: schema.MCPServerConfig{
				Command: "echo",
			},
			checkFunc: func(t *testing.T, pc *ParsedConfig) {
				t.Helper()
				assert.Equal(t, defaultTimeout, pc.Timeout)
			},
		},
		{
			name:    "nil env becomes empty map",
			cfgName: "test",
			cfg: schema.MCPServerConfig{
				Command: "echo",
			},
			checkFunc: func(t *testing.T, pc *ParsedConfig) {
				t.Helper()
				assert.NotNil(t, pc.Env)
				assert.Empty(t, pc.Env)
			},
		},
		{
			name:    "empty command returns error",
			cfgName: "bad",
			cfg:     schema.MCPServerConfig{},
			wantErr: errUtils.ErrMCPServerCommandEmpty,
		},
		{
			name:    "invalid timeout returns error",
			cfgName: "bad",
			cfg: schema.MCPServerConfig{
				Command: "echo",
				Timeout: "not-a-duration",
			},
			wantErr: errUtils.ErrMCPServerInvalidTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, err := ParseConfig(tt.cfgName, tt.cfg)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, pc)
			}
		})
	}
}
