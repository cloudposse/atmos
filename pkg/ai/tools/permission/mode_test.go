package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields used below.
var _ = schema.AIToolSettings{Mode: "allow", YOLOMode: true, RequireConfirmation: nil} //nolint:staticcheck // Deprecated aliases are still honored for backwards compatibility.

func boolRef(b bool) *bool { return &b }

//nolint:staticcheck // Deprecated aliases are still honored for backwards compatibility.
func TestResolveMode(t *testing.T) {
	tests := []struct {
		name     string
		settings schema.AIToolSettings
		mode     Mode
		yolo     bool
	}{
		{name: "empty defaults to prompt", settings: schema.AIToolSettings{}, mode: ModePrompt},
		{name: "mode require_confirmation", settings: schema.AIToolSettings{Mode: "require_confirmation"}, mode: ModePrompt},
		{name: "mode allow", settings: schema.AIToolSettings{Mode: "allow"}, mode: ModeAllow},
		{name: "mode yolo", settings: schema.AIToolSettings{Mode: "yolo"}, mode: ModeYOLO, yolo: true},
		{name: "mode is case-insensitive and trimmed", settings: schema.AIToolSettings{Mode: "  YOLO "}, mode: ModeYOLO, yolo: true},
		{name: "deprecated yolo_mode true", settings: schema.AIToolSettings{YOLOMode: true}, mode: ModeYOLO, yolo: true},
		{name: "deprecated yolo_mode beats require_confirmation false", settings: schema.AIToolSettings{YOLOMode: true, RequireConfirmation: boolRef(false)}, mode: ModeYOLO, yolo: true},
		{name: "deprecated require_confirmation false", settings: schema.AIToolSettings{RequireConfirmation: boolRef(false)}, mode: ModeAllow},
		{name: "deprecated require_confirmation true", settings: schema.AIToolSettings{RequireConfirmation: boolRef(true)}, mode: ModePrompt},
		{name: "mode wins over yolo_mode", settings: schema.AIToolSettings{Mode: "require_confirmation", YOLOMode: true}, mode: ModePrompt},
		{name: "mode wins over require_confirmation false", settings: schema.AIToolSettings{Mode: "require_confirmation", RequireConfirmation: boolRef(false)}, mode: ModePrompt},
		{name: "mode allow wins over yolo_mode", settings: schema.AIToolSettings{Mode: "allow", YOLOMode: true}, mode: ModeAllow},
		{name: "mode yolo wins over require_confirmation true", settings: schema.AIToolSettings{Mode: "yolo", RequireConfirmation: boolRef(true)}, mode: ModeYOLO, yolo: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, yolo, err := ResolveMode(&tt.settings)
			require.NoError(t, err)
			assert.Equal(t, tt.mode, mode)
			assert.Equal(t, tt.yolo, yolo)
		})
	}
}

//nolint:staticcheck // Deprecated aliases are still honored for backwards compatibility.
func TestResolveMode_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "unknown word", value: "permissive"},
		{name: "legacy prompt constant is not a setting", value: "prompt"},
		{name: "deny is not a setting", value: "deny"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A deprecated alias must not rescue an invalid mode.
			_, _, err := ResolveMode(&schema.AIToolSettings{Mode: tt.value, YOLOMode: true})
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrAIToolsInvalidMode)
		})
	}
}
