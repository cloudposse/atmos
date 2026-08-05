package pro

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// Stack-section keys used when resolving/collapsing the Atmos Pro enabled hierarchy.
const (
	sectionKey        = "pro"
	enabledKey        = "enabled"
	driftDetectionKey = "drift_detection"
)

// ResolveSection returns the effective `pro:` section for a component: the new top-level
// `pro:` block (proSection) if present, else the deprecated `settings.pro:` alias read out of
// settingsSection. An explicit pro: block takes whole-block precedence over settings.pro: --
// the two are never merged together, matching this codebase's other component-map alias
// precedents (component/metadata.component, settings.depends_on/dependencies.components).
// Returns nil if neither is present. Emits a deprecation notice when only the legacy path
// is used.
func ResolveSection(proSection, settingsSection map[string]any) map[string]any {
	if pro, ok := SanitizeForJSON(proSection).(map[string]any); ok && len(pro) > 0 {
		return pro
	}
	if pro, ok := SanitizeForJSON(settingsSection[sectionKey]).(map[string]any); ok {
		log.Debug("'settings.pro' is deprecated, use 'pro' instead. See: https://atmos.tools/cli/configuration/settings/pro")
		return pro
	}
	return nil
}

// metadataEnabled reports whether a component's metadata marks it enabled. Mirrors
// isComponentEnabled (internal/exec/component_utils.go): a component is enabled by default;
// only an explicit metadata.enabled: false disables it.
func metadataEnabled(metadata map[string]any) bool {
	if enabled, ok := metadata[enabledKey].(bool); ok {
		return enabled
	}
	return true
}

// proSettingEnabled reads pro.enabled from an already-resolved pro section (see
// ResolveSection), defaulting to true. Only an explicit boolean false disables; a missing or
// non-boolean value (e.g. the string "true") defaults to true, matching the Atmos Pro
// server-side default. The caller is responsible for confirming a pro section exists.
func proSettingEnabled(pro map[string]any) bool {
	if enabled, ok := pro[enabledKey].(bool); ok {
		return enabled
	}
	return true
}

// driftSettingEnabled reads pro.drift_detection.enabled from an already-resolved pro section,
// defaulting to false. Only an explicit boolean true enables drift detection. The drift block
// is normalized with SanitizeForJSON first: nested YAML maps can arrive as
// map[interface{}]interface{}, which would otherwise fail the map[string]any assertion and
// read as "no drift block".
func driftSettingEnabled(pro map[string]any) bool {
	drift, ok := SanitizeForJSON(pro[driftDetectionKey]).(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := drift[enabledKey].(bool)
	return ok && enabled
}

// EffectiveEnabledState collapses the enabled hierarchy metadata.enabled > pro.enabled >
// pro.drift_detection.enabled for an already-resolved pro section (see ResolveSection). An
// outer disable forces all inner levels off, so the single signal Atmos Pro persists already
// reflects the component's resolved state:
//
//	proEnabled   = metadata.enabled && pro.enabled                     (both default true)
//	driftEnabled = proEnabled       && pro.drift_detection.enabled     (drift defaults false)
//
// A nil pro section means the component is not Pro-enabled (and therefore not drift-enabled).
// This is the single source of truth shared by the upload payload, the CI PR-comment badge, and
// the `atmos list instances --upload` success-toast counts, so none of them can diverge.
func EffectiveEnabledState(pro, metadata map[string]any) (proEnabled, driftEnabled bool) {
	if pro == nil {
		return false, false
	}
	proEnabled = metadataEnabled(metadata) && proSettingEnabled(pro)
	driftEnabled = proEnabled && driftSettingEnabled(pro)
	return proEnabled, driftEnabled
}

// MetadataDisabledPro reports whether metadata.enabled: false is the reason an otherwise
// Pro-enabled component is treated as disabled downstream. It is true only when the pro
// section itself would be enabled (pro.enabled true or defaulted) but metadata.enabled is
// explicitly false, i.e. the higher-precedence metadata disable overrides pro.enabled. Used
// only for a debug log so operators can trace why a component is uploaded/reported as disabled.
func MetadataDisabledPro(pro, metadata map[string]any) bool {
	if pro == nil {
		return false
	}
	return proSettingEnabled(pro) && !metadataEnabled(metadata)
}

// SanitizeForJSON recursively converts map[interface{}]interface{} to map[string]interface{}
// for JSON compatibility. YAML unmarshaling can produce map[interface{}]interface{} for nested
// maps, which fails map[string]any type assertions elsewhere in this package.
func SanitizeForJSON(v any) any {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[fmt.Sprintf("%v", k)] = SanitizeForJSON(v)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[k] = SanitizeForJSON(v)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(val))
		for i, v := range val {
			s[i] = SanitizeForJSON(v)
		}
		return s
	default:
		return v
	}
}
