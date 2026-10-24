package driver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestK3sPortOverrideValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ports   []any
		invalid bool
	}{
		{"defaults", nil, false},
		{"http only", []any{map[string]any{"host": 8080, "container": 80}}, true},
		{"http and api", []any{map[string]any{"host": 8080, "container": 80}, map[string]any{"container": 6443}}, false},
		{"pinned api", []any{map[string]any{"host": 16443, "container": 6443, "protocol": "tcp"}}, false},
		{"udp api", []any{map[string]any{"container": 6443, "protocol": "udp"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := emu.FromComponentSection(map[string]any{"driver": "k3s", "container": map[string]any{"ports": tc.ports}})
			require.NoError(t, err)
			err = spec.Validate()
			if tc.invalid {
				require.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
				assert.ErrorContains(t, err, "6443/tcp")
				assert.ErrorContains(t, err, "container.ports")
				return
			}
			require.NoError(t, err)
			ports, err := spec.ContainerPorts()
			require.NoError(t, err)
			if tc.ports != nil {
				assert.Len(t, ports, len(tc.ports))
			}
		})
	}
}
