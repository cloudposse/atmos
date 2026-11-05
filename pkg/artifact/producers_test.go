package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProducerRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	files := NewMockProducer(ctrl)
	files.EXPECT().Kind().Return(KindFiles).AnyTimes()
	scaffold := NewMockProducer(ctrl)
	scaffold.EXPECT().Kind().Return("scaffold").AnyTimes()
	invalid := NewMockProducer(ctrl)
	invalid.EXPECT().Kind().Return("Bad Kind").AnyTimes()

	registry := NewProducerRegistry()
	require.NoError(t, registry.Register(scaffold))
	require.NoError(t, registry.Register(files))
	require.ErrorIs(t, registry.Register(files), errUtils.ErrArtifactConflict)
	require.ErrorIs(t, registry.Register(invalid), errUtils.ErrArtifactConfigInvalid)
	require.ErrorIs(t, registry.Register(nil), errUtils.ErrArtifactConfigInvalid)

	found, ok := registry.Lookup("")
	require.True(t, ok)
	assert.Equal(t, KindFiles, found.Kind(), "an empty kind selects the generic files producer")
	found, ok = registry.Lookup("scaffold")
	require.True(t, ok)
	assert.Equal(t, "scaffold", found.Kind())
	_, ok = registry.Lookup("component/terraform")
	assert.False(t, ok)
	assert.Equal(t, []string{KindFiles, "scaffold"}, registry.Kinds())
}
