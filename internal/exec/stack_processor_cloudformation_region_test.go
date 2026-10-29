package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func legacyCloudFormationRegion(region string) map[string]any {
	return map[string]any{"settings": map[string]any{"aws_cloudformation": map[string]any{"region": region}}}
}

func TestCloudFormationRegionScopes(t *testing.T) {
	tests := []struct {
		name                                         string
		global, defaults, base, component, overrides map[string]any
		want                                         string
	}{
		{name: "global", global: map[string]any{"region": "global"}, want: "global"},
		{name: "type overrides global", global: map[string]any{"region": "global"}, defaults: map[string]any{"region": "type"}, want: "type"},
		{name: "base overrides type", defaults: map[string]any{"region": "type"}, base: map[string]any{"region": "base"}, want: "base"},
		{name: "component overrides base", base: map[string]any{"region": "base"}, component: map[string]any{"region": "component"}, want: "component"},
		{name: "overrides", component: map[string]any{"region": "component"}, overrides: map[string]any{"region": "override"}, want: "override"},
		{name: "legacy global", global: legacyCloudFormationRegion("global"), want: "global"},
		{name: "legacy type overrides modern global", global: map[string]any{"region": "global"}, defaults: legacyCloudFormationRegion("type"), want: "type"},
		{name: "legacy base overrides modern type", defaults: map[string]any{"region": "type"}, base: legacyCloudFormationRegion("base"), want: "base"},
		{name: "legacy component overrides modern base", base: map[string]any{"region": "base"}, component: legacyCloudFormationRegion("component"), want: "component"},
		{name: "legacy overrides", component: map[string]any{"region": "component"}, overrides: legacyCloudFormationRegion("override"), want: "override"},
		{name: "modern component overrides legacy base", base: legacyCloudFormationRegion("base"), component: map[string]any{"region": "component"}, want: "component"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.global
			if config == nil {
				config = map[string]any{}
			}
			base := tt.base
			if base == nil {
				base = map[string]any{}
			}
			base["metadata"] = map[string]any{"type": "abstract"}
			component := tt.component
			if component == nil {
				component = map[string]any{}
			}
			component["metadata"] = map[string]any{"inherits": []any{"base"}}
			component["stack_name"] = "demo"
			component["path"] = "template.yaml"
			if tt.overrides != nil {
				component["overrides"] = tt.overrides
			}
			if tt.defaults != nil {
				config[cfg.CloudFormationSectionName] = tt.defaults
			}
			config["components"] = map[string]any{cfg.CloudFormationComponentType: map[string]any{"base": base, "demo": component}}
			dir := t.TempDir()
			result, _, err := ProcessStackConfig(&schema.AtmosConfiguration{}, dir, dir, dir, dir, dir, filepath.Join(dir, "test.yaml"), config, false, false, "", map[string]map[string][]string{}, map[string]map[string]any{}, false)
			require.NoError(t, err)
			components := result["components"].(map[string]any)[cfg.CloudFormationComponentType].(map[string]any)
			demo := components["demo"].(map[string]any)
			assert.Equal(t, tt.want, demo["region"])
			assert.Equal(t, tt.want, resolveCloudFormationRegion(demo), "output lookup uses the same inherited region")
			assert.Equal(t, tt.want, FilterComputedFields(demo)["region"])
		})
	}
}

func TestCloudFormationTimeoutScopes(t *testing.T) {
	for _, tt := range []struct {
		name                                         string
		global, defaults, base, component, overrides map[string]any
		want                                         any
	}{
		{name: "root", global: map[string]any{"timeout": "1h"}, want: "1h"},
		{name: "type", global: map[string]any{"timeout": "1h"}, defaults: map[string]any{"timeout": "45m"}, want: "45m"},
		{name: "legacy base", defaults: map[string]any{"timeout": "1h"}, base: map[string]any{"timeout_in_minutes": 30}, want: "30m"},
		{name: "component", base: map[string]any{"timeout_in_minutes": 30}, component: map[string]any{"timeout": 90}, want: 90},
		{name: "legacy overrides", component: map[string]any{"timeout": "1h"}, overrides: map[string]any{"timeout_in_minutes": 20}, want: "20m"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.global
			if config == nil {
				config = map[string]any{}
			}
			base := tt.base
			if base == nil {
				base = map[string]any{}
			}
			component := tt.component
			if component == nil {
				component = map[string]any{}
			}
			base["metadata"] = map[string]any{"type": "abstract"}
			component["metadata"] = map[string]any{"inherits": []any{"base"}}
			if tt.overrides != nil {
				component["overrides"] = tt.overrides
			}
			if tt.defaults != nil {
				config[cfg.CloudFormationSectionName] = tt.defaults
			}
			config["components"] = map[string]any{cfg.CloudFormationComponentType: map[string]any{"base": base, "demo": component}}
			dir := t.TempDir()
			result, _, err := ProcessStackConfig(&schema.AtmosConfiguration{}, dir, dir, dir, dir, dir, filepath.Join(dir, "test.yaml"), config, false, false, "", map[string]map[string][]string{}, map[string]map[string]any{}, false)
			require.NoError(t, err)
			demo := result["components"].(map[string]any)[cfg.CloudFormationComponentType].(map[string]any)["demo"].(map[string]any)
			assert.Equal(t, tt.want, demo["timeout"])
			assert.Equal(t, tt.want, FilterComputedFields(demo)["timeout"])
		})
	}
}
