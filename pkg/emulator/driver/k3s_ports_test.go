package driver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestK3sPortOverrideValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ports   []any
		invalid bool
		want    []schema.ContainerPort
	}{
		{"defaults", nil, false, []schema.ContainerPort{{Container: 6443}}},
		{"http only", []any{map[string]any{"host": 8080, "container": 80}}, true, nil},
		{"http and api", []any{map[string]any{"host": 8080, "container": 80}, map[string]any{"container": 6443}}, false, []schema.ContainerPort{{Host: 8080, Container: 80}, {Container: 6443}}},
		{"pinned api", []any{map[string]any{"host": 16443, "container": 6443, "protocol": "tcp"}}, false, []schema.ContainerPort{{Host: 16443, Container: 6443, Protocol: "tcp"}}},
		{"udp api", []any{map[string]any{"container": 6443, "protocol": "udp"}}, true, nil},
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
			assert.Equal(t, tc.want, ports)
		})
	}
}
