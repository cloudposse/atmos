package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSpec_DriverAccessors_UnknownDriverErrors(t *testing.T) {
	s := &Spec{Driver: "does/not-exist"}

	_, err := s.Image()
	require.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	_, err = s.ContainerPorts()
	require.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	_, err = s.RootlessOverride()
	require.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	_, err = s.DefaultCommand()
	require.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	_, err = s.DefaultEnv()
	require.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	_, err = s.Privileged()
	require.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
}

func TestSpec_ContainerPorts_ExplicitOverride(t *testing.T) {
	s := &Spec{Driver: testDriverName, Container: &schema.ContainerRunStep{
		Ports: []schema.ContainerPort{{Container: 8080, Host: 18080, Protocol: "tcp"}},
	}}
	ports, err := s.ContainerPorts()
	require.NoError(t, err)
	require.Len(t, ports, 1)
	assert.Equal(t, schema.ContainerPort{Container: 8080, Host: 18080, Protocol: "tcp"}, ports[0])
}

func TestSpec_RootlessOverride_Applies(t *testing.T) {
	applies, err := (&Spec{Driver: rootlessTestDriverName}).RootlessOverride()
	require.NoError(t, err)
	assert.True(t, applies.Applies, "a RootlessOverrider driver reports an override")

	none, err := (&Spec{Driver: testDriverName}).RootlessOverride()
	require.NoError(t, err)
	assert.False(t, none.Applies, "a plain driver reports no rootless override")
}

func TestSpec_FromComponentSection_BadContainerErrors(t *testing.T) {
	// A scalar where a container map is expected fails the YAML decode.
	_, err := FromComponentSection(map[string]any{"driver": "floci/aws", "container": "not-a-map"})
	require.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
}

func TestSpec_PersistEnabled_Tristate(t *testing.T) {
	// Default (nil) persists; explicit false persists; explicit true is ephemeral.
	assert.True(t, (&Spec{}).PersistEnabled(), "nil ephemeral persists by default")

	persist := false
	assert.True(t, (&Spec{Ephemeral: &persist}).PersistEnabled(), "ephemeral:false persists")

	ephemeral := true
	assert.False(t, (&Spec{Ephemeral: &ephemeral}).PersistEnabled(), "ephemeral:true does not persist")
}

func TestSpec_FromComponentSection_DecodesEphemeral(t *testing.T) {
	spec, err := FromComponentSection(map[string]any{"driver": "floci/aws", "ephemeral": true})
	require.NoError(t, err)
	require.NotNil(t, spec.Ephemeral)
	assert.True(t, *spec.Ephemeral)
	assert.False(t, spec.PersistEnabled())

	// Omitted -> nil -> persists.
	spec, err = FromComponentSection(map[string]any{"driver": "floci/aws"})
	require.NoError(t, err)
	assert.Nil(t, spec.Ephemeral)
	assert.True(t, spec.PersistEnabled())
}

func TestSpec_FromComponentSection_RejectsInvalidEphemeral(t *testing.T) {
	_, err := FromComponentSection(map[string]any{"driver": "floci/aws", "ephemeral": "true"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
}

func TestSpec_DataDir(t *testing.T) {
	dir, err := (&Spec{Driver: persistDriverName}).DataDir()
	require.NoError(t, err)
	assert.Equal(t, persistDataDir, dir)

	// A driver with no data dir reports an empty path (persistence no-op).
	dir, err = (&Spec{Driver: testDriverName}).DataDir()
	require.NoError(t, err)
	assert.Empty(t, dir)
}

func TestToStringSlice(t *testing.T) {
	got, err := toStringSlice("a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, got)

	got, err = toStringSlice([]string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)

	got, err = toStringSlice([]any{"x", "z"})
	require.NoError(t, err)
	assert.Equal(t, []string{"x", "z"}, got)

	_, err = toStringSlice([]any{"x", 1, "z"})
	require.Error(t, err)

	_, err = toStringSlice(42)
	require.Error(t, err)
}
