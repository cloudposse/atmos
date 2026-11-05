package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cfnGenerateStackConfig builds a stack with one CloudFormation and one Terraform component.
// The mutate callback customizes the stack before processing.
func cfnGenerateStackConfig(mutate func(config, cfnComponent map[string]any)) map[string]any {
	cfnComponent := map[string]any{
		cfg.TemplateSectionName:  "Resources: {}",
		cfg.StackNameSectionName: "acme-dev-vpc",
	}
	config := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.CloudFormationComponentType: map[string]any{"vpc": cfnComponent},
			cfg.TerraformComponentType:      map[string]any{"tf": map[string]any{cfg.VarsSectionName: map[string]any{"a": "b"}}},
		},
	}
	mutate(config, cfnComponent)
	return config
}

// TestProcessStackConfig_CloudFormationGenerateScopes verifies that a generate section is rejected in
// every CloudFormation scope, while the root-level (Terraform global) generate neither errors nor
// reaches CloudFormation components, and is still applied to Terraform components.
func TestProcessStackConfig_CloudFormationGenerateScopes(t *testing.T) {
	generate := map[string]any{"values.yaml": "key: value"}
	tests := []struct {
		name    string
		mutate  func(config, cfnComponent map[string]any)
		wantErr bool
		// rootLevel marks the case where generate is set at the stack root.
		rootLevel bool
	}{
		{name: "component level", mutate: func(_, c map[string]any) { c[cfg.GenerateSectionName] = generate }, wantErr: true},
		{name: "type level", mutate: func(config, _ map[string]any) {
			config[cfg.CloudFormationSectionName] = map[string]any{cfg.GenerateSectionName: generate}
		}, wantErr: true},
		{name: "component overrides", mutate: func(_, c map[string]any) {
			c[cfg.OverridesSectionName] = map[string]any{cfg.GenerateSectionName: generate}
		}, wantErr: true},
		{name: "root level is Terraform scope", rootLevel: true, mutate: func(config, _ map[string]any) { config[cfg.GenerateSectionName] = generate }},
		{name: "no generate anywhere", mutate: func(_, _ map[string]any) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inheritance is cached by stack name, so each case needs its own stack identity.
			stackFile := "cfn-generate-" + strings.ReplaceAll(tt.name, " ", "-") + ".yaml"
			result, _, err := ProcessStackConfig(
				&schema.AtmosConfiguration{}, "/test/stacks", "/test/terraform", "/test/helmfile", "/test/packer", "/test/ansible",
				stackFile, cfnGenerateStackConfig(tt.mutate), false, false, "",
				map[string]map[string][]string{}, map[string]map[string]any{}, false,
			)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationGenerateUnsupported)
				assert.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "inline 'template:'")
				return
			}
			require.NoError(t, err)
			components := result[cfg.ComponentsSectionName].(map[string]any)
			vpc := components[cfg.CloudFormationComponentType].(map[string]any)["vpc"].(map[string]any)
			assert.NotContains(t, vpc, cfg.GenerateSectionName, "root-level generate must not reach CloudFormation components")
			if tt.rootLevel {
				tf := components[cfg.TerraformComponentType].(map[string]any)["tf"].(map[string]any)
				assert.Equal(t, generate, tf[cfg.GenerateSectionName], "root-level generate still applies to Terraform")
			}
		})
	}
}
