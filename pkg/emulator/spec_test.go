package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFromComponentSection_FullSpec(t *testing.T) {
	section := map[string]any{
		"driver":   "test/aws",
		"cloud":    "aws",
		"region":   "us-west-2",
		"services": []any{"s3", "sqs"},
		"container": map[string]any{
			"image": "custom/img:1.5",
			"ports": []any{map[string]any{"container": 4566}},
		},
	}

	spec, err := FromComponentSection(section)
	require.NoError(t, err)
	assert.Equal(t, "test/aws", spec.Driver)
	assert.Equal(t, "aws", spec.Cloud)
	assert.Equal(t, "us-west-2", spec.Region)
	assert.Equal(t, []string{"s3", "sqs"}, spec.Services)
	require.NotNil(t, spec.Container)
	assert.Equal(t, "custom/img:1.5", spec.Container.Image)
	require.Len(t, spec.Container.Ports, 1)
	assert.Equal(t, 4566, spec.Container.Ports[0].Container)
}

func TestSpec_Validate(t *testing.T) {
	t.Run("missing driver", func(t *testing.T) {
		spec := Spec{}
		err := spec.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
	})

	t.Run("unknown driver", func(t *testing.T) {
		spec := Spec{Driver: "nope"}
		err := spec.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
	})

	t.Run("cloud mismatches driver target", func(t *testing.T) {
		spec := Spec{Driver: testDriverName, Cloud: "gcp"}
		err := spec.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmulatorTargetMismatch)
	})

	t.Run("valid", func(t *testing.T) {
		spec := Spec{Driver: testDriverName, Cloud: "aws"}
		require.NoError(t, spec.Validate())
	})
}

func TestSpec_Target_DerivedFromDriver(t *testing.T) {
	spec := Spec{Driver: testDriverName} // no explicit cloud.
	target, err := spec.Target()
	require.NoError(t, err)
	assert.Equal(t, TargetAWS, target)
}

func TestSpec_Image_ExplicitBeatsDriverDefault(t *testing.T) {
	explicit := Spec{Driver: testDriverName, Container: &schema.ContainerRunStep{Image: "custom/img:1"}}
	img, err := explicit.Image()
	require.NoError(t, err)
	assert.Equal(t, "custom/img:1", img)

	defaulted := Spec{Driver: testDriverName}
	img, err = defaulted.Image()
	require.NoError(t, err)
	assert.Equal(t, testDriverImage, img, "falls back to the driver default image")
}

func TestSpec_ContainerPorts_DefaultsFromDriver(t *testing.T) {
	spec := Spec{Driver: testDriverName} // no explicit ports.
	ports, err := spec.ContainerPorts()
	require.NoError(t, err)
	require.Len(t, ports, 1)
	assert.Equal(t, testDriverPort, ports[0].Container)
	assert.Zero(t, ports[0].Host, "host port auto-assigned (0) unless pinned")
}
