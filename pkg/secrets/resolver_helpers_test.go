package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// TestComponentName covers every fallback branch of componentName.
func TestComponentName(t *testing.T) {
	tests := []struct {
		name      string
		stackInfo *schema.ConfigAndStacksInfo
		want      string
	}{
		{
			name:      "nil stackInfo",
			stackInfo: nil,
			want:      "",
		},
		{
			name:      "Component wins",
			stackInfo: &schema.ConfigAndStacksInfo{Component: "comp", ComponentFromArg: "arg", FinalComponent: "final"},
			want:      "comp",
		},
		{
			name:      "ComponentFromArg fallback",
			stackInfo: &schema.ConfigAndStacksInfo{ComponentFromArg: "arg", FinalComponent: "final"},
			want:      "arg",
		},
		{
			name:      "FinalComponent fallback",
			stackInfo: &schema.ConfigAndStacksInfo{FinalComponent: "final"},
			want:      "final",
		},
		{
			name:      "all empty",
			stackInfo: &schema.ConfigAndStacksInfo{},
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, componentName(tt.stackInfo))
		})
	}
}

// TestResolve_PathModifier proves the `| path` modifier applies a YQ expression to a structured
// value and that the resolved value is registered with the masker.
func TestResolve_PathModifier(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		Get("prod", "api", "DATADOG_API_KEY").
		Return(map[string]any{"host": "db.internal", "port": 5432}, nil).
		Times(1)

	cfg, componentSection := newSecretTestConfig(mockStore)
	require.NoError(t, iolib.Initialize())

	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
	}

	got, err := Resolve(cfg, `!secret DATADOG_API_KEY | path ".host"`, "prod", info)
	require.NoError(t, err)
	assert.Equal(t, "db.internal", got)

	// The retrieved value must be registered with the masker so it is redacted in output.
	masked := iolib.GetContext().Masker().Mask("connecting to db.internal now")
	assert.NotContains(t, masked, "db.internal", "resolved secret value must be masked")
}
