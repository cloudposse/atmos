package filemanager

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRegistry_AddTool(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("success with multiple managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().AddTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().AddTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.AddTool(ctx, "terraform", "1.13.4")
		assert.NoError(t, err)
	})

	t.Run("skips disabled managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().AddTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(false)
		// mgr2.AddTool should NOT be called

		registry := NewRegistry(mgr1, mgr2)
		err := registry.AddTool(ctx, "terraform", "1.13.4")
		assert.NoError(t, err)
	})

	t.Run("returns error if any manager fails", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		expectedErr := errors.New("write failed")

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().AddTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().AddTool(ctx, "terraform", "1.13.4").Return(expectedErr)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.AddTool(ctx, "terraform", "1.13.4")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
		assert.Contains(t, err.Error(), "manager2")
	})

	t.Run("passes options to managers", func(t *testing.T) {
		mgr := NewMockFileManager(ctrl)

		mgr.EXPECT().Enabled().Return(true)
		mgr.EXPECT().AddTool(ctx, "terraform", "1.13.4", gomock.Any()).Return(nil)
		mgr.EXPECT().Name().Return("manager1")

		registry := NewRegistry(mgr)
		err := registry.AddTool(ctx, "terraform", "1.13.4", WithAsDefault())
		assert.NoError(t, err)
	})
}

func TestRegistry_RemoveTool(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("success with multiple managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().RemoveTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().RemoveTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.RemoveTool(ctx, "terraform", "1.13.4")
		assert.NoError(t, err)
	})

	t.Run("skips disabled managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().RemoveTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(false)
		// mgr2.RemoveTool should NOT be called

		registry := NewRegistry(mgr1, mgr2)
		err := registry.RemoveTool(ctx, "terraform", "1.13.4")
		assert.NoError(t, err)
	})

	t.Run("returns error if any manager fails", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		expectedErr := errors.New("remove failed")

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().RemoveTool(ctx, "terraform", "1.13.4").Return(expectedErr)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().RemoveTool(ctx, "terraform", "1.13.4").Return(nil)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.RemoveTool(ctx, "terraform", "1.13.4")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
	})
}

func TestRegistry_SetDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("success with multiple managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().SetDefault(ctx, "terraform", "1.13.4").Return(nil)
		mgr1.EXPECT().Name().Return("manager1")

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().SetDefault(ctx, "terraform", "1.13.4").Return(nil)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.SetDefault(ctx, "terraform", "1.13.4")
		assert.NoError(t, err)
	})

	t.Run("skips disabled managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(false)
		// mgr1.SetDefault should NOT be called

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().SetDefault(ctx, "terraform", "1.13.4").Return(nil)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.SetDefault(ctx, "terraform", "1.13.4")
		assert.NoError(t, err)
	})

	t.Run("returns error if any manager fails", func(t *testing.T) {
		mgr := NewMockFileManager(ctrl)

		expectedErr := errors.New("set default failed")

		mgr.EXPECT().Enabled().Return(true)
		mgr.EXPECT().SetDefault(ctx, "terraform", "1.13.4").Return(expectedErr)
		mgr.EXPECT().Name().Return("manager1")

		registry := NewRegistry(mgr)
		err := registry.SetDefault(ctx, "terraform", "1.13.4")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
	})
}

func TestRegistry_VerifyAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("success with multiple managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().Verify(ctx).Return(nil)

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().Verify(ctx).Return(nil)

		registry := NewRegistry(mgr1, mgr2)
		err := registry.VerifyAll(ctx)
		assert.NoError(t, err)
	})

	t.Run("skips disabled managers", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		mgr1.EXPECT().Enabled().Return(false)
		// mgr1.Verify should NOT be called

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().Verify(ctx).Return(nil)

		registry := NewRegistry(mgr1, mgr2)
		err := registry.VerifyAll(ctx)
		assert.NoError(t, err)
	})

	t.Run("returns error if any manager fails", func(t *testing.T) {
		mgr1 := NewMockFileManager(ctrl)
		mgr2 := NewMockFileManager(ctrl)

		expectedErr := errors.New("verification failed")

		mgr1.EXPECT().Enabled().Return(true)
		mgr1.EXPECT().Verify(ctx).Return(nil)

		mgr2.EXPECT().Enabled().Return(true)
		mgr2.EXPECT().Verify(ctx).Return(expectedErr)
		mgr2.EXPECT().Name().Return("manager2")

		registry := NewRegistry(mgr1, mgr2)
		err := registry.VerifyAll(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verification failed")
	})
}

func TestRegistry_EmptyRegistry(t *testing.T) {
	ctx := context.Background()

	registry := NewRegistry()

	// All operations should succeed on empty registry
	err := registry.AddTool(ctx, "terraform", "1.13.4")
	assert.NoError(t, err)

	err = registry.RemoveTool(ctx, "terraform", "1.13.4")
	assert.NoError(t, err)

	err = registry.SetDefault(ctx, "terraform", "1.13.4")
	assert.NoError(t, err)

	err = registry.VerifyAll(ctx)
	assert.NoError(t, err)
}
