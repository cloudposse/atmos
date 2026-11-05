package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPushRefs(t *testing.T) {
	// Build tags drive the push set: deduped, order-preserving, empties dropped.
	multi := ContainerSpec{
		Image: "app:v1",
		Build: &schema.ContainerBuildStep{Tags: []string{"reg1/app:v1", "reg2/app:v1", "", "reg1/app:v1"}},
	}
	assert.Equal(t, []string{"reg1/app:v1", "reg2/app:v1"}, multi.PushRefs())

	// Build present but no usable tags → fall back to the single image.
	emptyTags := ContainerSpec{Image: "app:v1", Build: &schema.ContainerBuildStep{Tags: []string{""}}}
	assert.Equal(t, []string{"app:v1"}, emptyTags.PushRefs())

	// No build → the single image.
	imageOnly := ContainerSpec{Image: "app:v1"}
	assert.Equal(t, []string{"app:v1"}, imageOnly.PushRefs())

	// Nothing to push.
	assert.Nil(t, (&ContainerSpec{}).PushRefs())
}

// buildSection returns a component section with a build that carries the given tags.
func buildSection(image string, tags ...string) map[string]any {
	anyTags := make([]any, len(tags))
	for i, tag := range tags {
		anyTags[i] = tag
	}
	return map[string]any{
		"image": image,
		"build": map[string]any{"context": ".", "tags": anyTags},
	}
}

func TestExecutePush_PushesEveryBuildTagInOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, buildSection("app:v1", "reg1.example.com/app:v1", "reg2.example.com/app:v1"), nil, rt)

	gomock.InOrder(
		rt.EXPECT().Push(gomock.Any(), "reg1.example.com/app:v1").Return(nil, nil),
		rt.EXPECT().Push(gomock.Any(), "reg2.example.com/app:v1").Return(nil, nil),
	)
	require.NoError(t, ExecutePush(context.Background(), infoFor("app")))
}

func TestExecutePush_FailFast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, buildSection("app:v1", "reg1.example.com/app:v1", "reg2.example.com/app:v1"), nil, rt)

	// First registry errors → second registry is never pushed (no expectation set
	// for it, so gomock would fail on an unexpected call).
	rt.EXPECT().Push(gomock.Any(), "reg1.example.com/app:v1").Return(nil, assert.AnError)
	require.Error(t, ExecutePush(context.Background(), infoFor("app")))
}

func TestExecutePush_SingleImageFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "app:v1"}, nil, rt)

	rt.EXPECT().Push(gomock.Any(), "app:v1").Return(nil, nil)
	require.NoError(t, ExecutePush(context.Background(), infoFor("app")))
}

func TestExecutePush_DryRunPushesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl) // no calls expected
	withStubs(t, buildSection("app:v1", "reg1.example.com/app:v1", "reg2.example.com/app:v1"), nil, rt)

	info := infoFor("app")
	info.DryRun = true
	require.NoError(t, ExecutePush(context.Background(), info))
}

func TestExecutePush_NoImageOrTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl) // no calls expected
	withStubs(t, map[string]any{}, nil, rt)

	require.Error(t, ExecutePush(context.Background(), infoFor("app")))
}
