package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/store"
)

// TestValidationResult_Valid covers the Valid() predicate across the three shapes a
// ValidationResult can take: all good, a missing-required entry, and an errored entry.
func TestValidationResult_Valid(t *testing.T) {
	tests := []struct {
		name   string
		result ValidationResult
		want   bool
	}{
		{
			name:   "all valid",
			result: ValidationResult{All: []Status{{Initialized: true}}},
			want:   true,
		},
		{
			name:   "missing required",
			result: ValidationResult{MissingRequired: []Status{{}}},
			want:   false,
		},
		{
			name:   "errored",
			result: ValidationResult{Errored: []Status{{Err: assertErr{}}}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.result.Valid())
		})
	}
}

// TestService_Validate_AllValid proves Validate() reports a clean result when the required
// secret is initialized in its backend.
func TestService_Validate_AllValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// Status falls back to Get for a plain store; a successful Get => initialized.
	mockStore.EXPECT().Get("prod", "api", "API_KEY").Return("v1", nil)

	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	result := svc.Validate()
	assert.True(t, result.Valid())
	assert.Empty(t, result.MissingRequired)
	assert.Empty(t, result.Errored)
	require.Len(t, result.All, 1)
	assert.Equal(t, "API_KEY", result.All[0].Declaration.Name)
	assert.True(t, result.All[0].Initialized)
}

// TestService_Validate_Errored proves a status error is classified into Errored (not
// MissingRequired) and makes the result invalid.
func TestService_Validate_Errored(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// A provider lookup failure surfaces as a Status error: point the declaration at a store
	// that exists in StoresConfig but is missing from the registry.
	cfg, section := serviceTestConfig(nil)
	cfg.Stores = store.StoreRegistry{}
	svc := NewService(cfg, "prod", "api", section)

	result := svc.Validate()
	assert.False(t, result.Valid())
	assert.Empty(t, result.MissingRequired)
	require.Len(t, result.Errored, 1)
	assert.Equal(t, "API_KEY", result.Errored[0].Declaration.Name)
	require.Error(t, result.Errored[0].Err)
}
