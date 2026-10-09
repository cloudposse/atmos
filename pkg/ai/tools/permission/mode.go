package permission

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Values accepted by the ai.tools.mode setting.
const (
	// SettingModeRequireConfirmation prompts for tools that need approval (the default).
	SettingModeRequireConfirmation = "require_confirmation"
	// SettingModeAllow never prompts but still honors the blocked list.
	SettingModeAllow = "allow"
	// SettingModeYOLO bypasses every check, including the blocked list.
	SettingModeYOLO = "yolo"
)

// ResolveMode returns the permission mode and YOLO flag for the given tool settings.
//
// Precedence: a non-empty ai.tools.mode always wins. Otherwise the deprecated
// booleans apply: yolo_mode: true selects yolo, require_confirmation: false selects
// allow, and anything else selects require_confirmation. An unknown mode value
// returns an error wrapping ErrAIToolsInvalidMode.
func ResolveMode(settings *schema.AIToolSettings) (Mode, bool, error) {
	defer perf.Track(nil, "permission.ResolveMode")()

	if raw := strings.TrimSpace(settings.Mode); raw != "" {
		switch strings.ToLower(raw) {
		case SettingModeRequireConfirmation:
			return ModePrompt, false, nil
		case SettingModeAllow:
			return ModeAllow, false, nil
		case SettingModeYOLO:
			return ModeYOLO, true, nil
		default:
			return "", false, invalidModeError(settings.Mode)
		}
	}

	// Deprecated aliases.
	if settings.YOLOMode { //nolint:staticcheck // Deprecated aliases are still honored for backwards compatibility.
		return ModeYOLO, true, nil
	}
	if settings.RequireConfirmation != nil && !*settings.RequireConfirmation { //nolint:staticcheck // Deprecated aliases are still honored for backwards compatibility.
		return ModeAllow, false, nil
	}

	return ModePrompt, false, nil
}

// invalidModeError builds the error for an unknown ai.tools.mode value.
func invalidModeError(value string) error {
	return errUtils.Build(errUtils.ErrAIToolsInvalidMode).
		WithContext("mode", value).
		WithHint("Use `require_confirmation` (default) to prompt for tools that need approval.").
		WithHint("Use `allow` to never prompt while still honoring `ai.tools.blocked`.").
		WithHint("Use `yolo` to bypass every check, including `ai.tools.blocked` (dangerous).").
		Err()
}
