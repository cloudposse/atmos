package doctor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDoctorNamingRejectsUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		configure     func(*schema.AtmosConfiguration)
	}{
		{"delimiters", "custom template delimiters", func(c *schema.AtmosConfiguration) { c.Templates.Settings.Delimiters = []string{"[[", "]]"} }},
		{"missing values ignored", "ignored missing template inputs", func(c *schema.AtmosConfiguration) { c.Templates.Settings.IgnoreMissingTemplateValues = true }},
		{"unsafe stack", "unsafe naming template", func(c *schema.AtmosConfiguration) { c.Stacks.NameTemplate = `{{ env "SECRET" }}` }},
		{"unsafe cluster", "unsafe naming template", func(c *schema.AtmosConfiguration) {
			c.Components.Helmfile.ClusterNameTemplate = `{{ readFile "secret" }}`
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			tc.configure(config)
			results, err := staticNamingResults(context.Background(), config, namingContractInputs(t, nil))
			require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, results)
		})
	}
}

func TestDoctorNamingPropagatesResolutionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, kind, message string
		configure           func(*schema.AtmosConfiguration)
	}{
		{"missing stack input", "terraform", "missing", func(c *schema.AtmosConfiguration) { c.Stacks.NameTemplate = `{{ .vars.missing }}` }},
		{"missing legacy stack input", "terraform", "stage", func(c *schema.AtmosConfiguration) { c.Stacks.NamePattern = "{stage}" }},
		{"missing helmfile cluster input", "helmfile", "missing", func(c *schema.AtmosConfiguration) { c.Components.Helmfile.ClusterNameTemplate = `{{ .vars.missing }}` }},
		{"missing kubeconfig cluster input", "terraform", "missing", func(c *schema.AtmosConfiguration) { c.Components.Helmfile.ClusterNameTemplate = `{{ .vars.missing }}` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			tc.configure(config)
			inputs := staticInputsFromMaps(t, map[string]map[string]any{"dev.yaml": {
				"components": map[string]any{tc.kind: map[string]any{"app": map[string]any{}}},
			}})
			results, err := staticNamingResults(context.Background(), config, inputs)
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, results, "failed resolution must not advertise a complete comparison")
		})
	}
}

func TestDoctorNamingCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := staticNamingResults(ctx, &schema.AtmosConfiguration{}, namingContractInputs(t, nil))
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, results)

	results = map[naming.Identity]naming.Result{}
	err = productionNamingReporter().addStaticManifestNames(ctx, &schema.AtmosConfiguration{}, "dev", map[string]any{
		"components": map[string]any{"terraform": map[string]any{"app": map[string]any{}}},
	}, results)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, results)
}

func TestDoctorWorkspaceRejectsUnsafeExpressions(t *testing.T) {
	for _, field := range []string{"terraform_workspace_template", "terraform_workspace_pattern"} {
		t.Run(field, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Templates.Settings.Enabled = true
			info := schema.ConfigAndStacksInfo{ComponentMetadataSection: map[string]any{field: `{{ env "SECRET" }}`}}
			workspace, err := productionNamingReporter().staticWorkspaceName(config, &info)
			require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
			require.ErrorContains(t, err, "unsafe workspace")
			require.Empty(t, workspace)
		})
	}
}
