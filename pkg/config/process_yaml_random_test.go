package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreprocessAtmosYamlFunc_Random(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		wantErr     bool
		checkFunc   func(t *testing.T, v *viper.Viper)
	}{
		{
			name: "random with two arguments in devcontainer spec",
			yamlContent: `
devcontainer:
  test:
    spec:
      image: "alpine:latest"
      forwardPorts:
        - !random 8080 8099
        - !random 3000 3099
`,
			wantErr: false,
			checkFunc: func(t *testing.T, v *viper.Viper) {
				ports := v.Get("devcontainer.test.spec.forwardPorts")
				require.NotNil(t, ports, "forwardPorts should not be nil")

				portsSlice, ok := ports.([]any)
				require.True(t, ok, "forwardPorts should be a slice")
				require.Len(t, portsSlice, 2, "should have 2 ports")

				// Check first port is in range 8080-8099
				port1, ok := portsSlice[0].(int)
				require.True(t, ok, "first port should be an integer")
				assert.GreaterOrEqual(t, port1, 8080, "first port should be >= 8080")
				assert.LessOrEqual(t, port1, 8099, "first port should be <= 8099")

				// Check second port is in range 3000-3099
				port2, ok := portsSlice[1].(int)
				require.True(t, ok, "second port should be an integer")
				assert.GreaterOrEqual(t, port2, 3000, "second port should be >= 3000")
				assert.LessOrEqual(t, port2, 3099, "second port should be <= 3099")
			},
		},
		{
			name: "random with one argument",
			yamlContent: `
devcontainer:
  test:
    spec:
      forwardPorts:
        - !random 9999
`,
			wantErr: false,
			checkFunc: func(t *testing.T, v *viper.Viper) {
				ports := v.Get("devcontainer.test.spec.forwardPorts")
				require.NotNil(t, ports)

				portsSlice, ok := ports.([]any)
				require.True(t, ok, "expected []any for ports")
				require.Len(t, portsSlice, 1)

				port, ok := portsSlice[0].(int)
				require.True(t, ok, "expected int for port")
				assert.GreaterOrEqual(t, port, 0)
				assert.LessOrEqual(t, port, 9999)
			},
		},
		{
			name: "random with no arguments",
			yamlContent: `
devcontainer:
  test:
    spec:
      forwardPorts:
        - !random
`,
			wantErr: false,
			checkFunc: func(t *testing.T, v *viper.Viper) {
				ports := v.Get("devcontainer.test.spec.forwardPorts")
				require.NotNil(t, ports)

				portsSlice, ok := ports.([]any)
				require.True(t, ok, "expected ports to be []any")
				require.Len(t, portsSlice, 1)

				portVal, ok := portsSlice[0].(int)
				require.True(t, ok, "expected port element to be int")
				assert.GreaterOrEqual(t, portVal, 0)
				assert.LessOrEqual(t, portVal, 65535)
			},
		},
		{
			name: "random in vars section",
			yamlContent: `
vars:
  app_port: !random 49152 65535
  worker_id: !random 1000 9999
`,
			wantErr: false,
			checkFunc: func(t *testing.T, v *viper.Viper) {
				appPort := v.GetInt("vars.app_port")
				assert.GreaterOrEqual(t, appPort, 49152)
				assert.LessOrEqual(t, appPort, 65535)

				workerID := v.GetInt("vars.worker_id")
				assert.GreaterOrEqual(t, workerID, 1000)
				assert.LessOrEqual(t, workerID, 9999)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			err := preprocessAtmosYamlFunc([]byte(tt.yamlContent), v)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkFunc != nil {
					tt.checkFunc(t, v)
				}
			}
		})
	}
}
