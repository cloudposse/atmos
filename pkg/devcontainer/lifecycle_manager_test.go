package devcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewManager(t *testing.T) {
	t.Run("creates manager with default dependencies", func(t *testing.T) {
		mgr := NewManager()

		assert.NotNil(t, mgr)
		assert.NotNil(t, mgr.configLoader)
		assert.NotNil(t, mgr.identityManager)
		assert.NotNil(t, mgr.runtimeDetector)
	})

	t.Run("creates manager with custom ConfigLoader", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLoader := NewMockConfigLoader(ctrl)
		mgr := NewManager(WithConfigLoader(mockLoader))

		assert.Equal(t, mockLoader, mgr.configLoader)
		assert.NotNil(t, mgr.identityManager)
		assert.NotNil(t, mgr.runtimeDetector)
	})

	t.Run("creates manager with custom IdentityManager", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockIdentity := NewMockIdentityManager(ctrl)
		mgr := NewManager(WithIdentityManager(mockIdentity))

		assert.NotNil(t, mgr.configLoader)
		assert.Equal(t, mockIdentity, mgr.identityManager)
		assert.NotNil(t, mgr.runtimeDetector)
	})

	t.Run("creates manager with custom RuntimeDetector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDetector := NewMockRuntimeDetector(ctrl)
		mgr := NewManager(WithRuntimeDetector(mockDetector))

		assert.NotNil(t, mgr.configLoader)
		assert.NotNil(t, mgr.identityManager)
		assert.Equal(t, mockDetector, mgr.runtimeDetector)
	})

	t.Run("creates manager with multiple custom dependencies", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLoader := NewMockConfigLoader(ctrl)
		mockIdentity := NewMockIdentityManager(ctrl)
		mockDetector := NewMockRuntimeDetector(ctrl)

		mgr := NewManager(
			WithConfigLoader(mockLoader),
			WithIdentityManager(mockIdentity),
			WithRuntimeDetector(mockDetector),
		)

		assert.Equal(t, mockLoader, mgr.configLoader)
		assert.Equal(t, mockIdentity, mgr.identityManager)
		assert.Equal(t, mockDetector, mgr.runtimeDetector)
	})
}
