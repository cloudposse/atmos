package cloudformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDryRunValidatesStaticTargetValuesWithoutMutatingInputs defers dynamic expressions, rejects
// malformed static entries, and preserves nested input configuration.
func TestDryRunValidatesStaticTargetValuesWithoutMutatingInputs(t *testing.T) {
	for _, tt := range []struct {
		name      string
		accounts  any
		wantError bool
	}{
		{name: "scalar dynamic", accounts: "!exec select-account"},
		{name: "mixed dynamic and quoted account", accounts: []any{"!env ACCOUNT", "012345678901"}},
		{name: "mixed dynamic and malformed account", accounts: []any{"!env ACCOUNT", 123}, wantError: true},
		{name: "static quoted list", accounts: []string{"012345678901"}},
		{name: "static scalar", accounts: "012345678901"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := map[string]any{"kind": "aws/stackset", "accounts": tt.accounts, "regions": "!exec select-region"}
			targets := map[string]any{"fleet": target, "ignored": "malformed unrelated target"}
			section := map[string]any{"stack_name": "fleet", "template": "!exec generate-template", "provision": map[string]any{"targets": targets}}
			info := &schema.ConfigAndStacksInfo{ComponentSection: section}
			err := validateDryRun(&schema.AtmosConfiguration{}, info, map[string]any{"target": "fleet"}, OperationStackSetCreate)
			if tt.wantError {
				require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, "!exec generate-template", section["template"])
			assert.Equal(t, tt.accounts, target["accounts"])
			assert.Equal(t, "!exec select-region", target["regions"])
			assert.Equal(t, "malformed unrelated target", targets["ignored"])
		})
	}
}

// TestDryRunStackSetMissingProvisionIsValidationError requires delivery-target configuration even when
// execution is deferred.
func TestDryRunStackSetMissingProvisionIsValidationError(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stack_name": "fleet", "path": "unprovisioned.yaml"}}
	err := validateDryRun(&schema.AtmosConfiguration{}, info, nil, OperationStackSetUpdate)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// TestDryRunRecognizesConfiguredTemplateDelimiters defers custom-delimited templates and target values
// without rewriting the input.
func TestDryRunRecognizesConfiguredTemplateDelimiters(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Templates.Settings.Delimiters = []string{"[[", "]]"}
	body := `[[ env "TEMPLATE" ]]`
	accounts := []any{"[[ .vars.account ]]", "012345678901"}
	target := map[string]any{"kind": "aws/stackset", "accounts": accounts, "regions": []any{"us-east-2", "[[ .vars.region ]]"}}
	section := map[string]any{"stack_name": "fleet", "template": body, "provision": map[string]any{"targets": map[string]any{"fleet": target}}}
	require.NoError(t, validateDryRun(config, &schema.ConfigAndStacksInfo{ComponentSection: section}, nil, OperationStackSetUpdate))
	assert.Equal(t, body, section["template"])
	assert.Equal(t, accounts, target["accounts"])
}

// TestDryRunDoesNotSuppressInvalidStaticParameters rejects malformed parameters even when the template
// file will not be read.
func TestDryRunDoesNotSuppressInvalidStaticParameters(t *testing.T) {
	section := map[string]any{"stack_name": "fleet", "path": "unprovisioned.yaml", "parameters": map[string]any{"Invalid": map[string]any{"nested": "value"}}}
	err := validateDryRun(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{ComponentSection: section}, nil, OperationApply)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}
