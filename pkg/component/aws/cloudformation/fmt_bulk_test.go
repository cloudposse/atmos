package cloudformation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteBulk_FmtContinuesAfterInlineTemplate(t *testing.T) {
	for _, selection := range []struct {
		name string
		info schema.ConfigAndStacksInfo
	}{
		{name: "all", info: schema.ConfigAndStacksInfo{All: true}},
		{name: "affected", info: schema.ConfigAndStacksInfo{Affected: true}},
		{name: "tags", info: schema.ConfigAndStacksInfo{Tags: []string{"format"}}},
		{name: "labels", info: schema.ConfigAndStacksInfo{Labels: map[string]string{"team": "platform"}}},
	} {
		t.Run(selection.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "template.yaml")
			original := "AWSTemplateFormatVersion:   '2010-09-09'\nResources: {}\n"
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			components := map[string]any{
				"a-inline": map[string]any{"stack_name": "a-inline", "template": "Resources: {}", "metadata": map[string]any{"tags": []any{"format"}, "labels": map[string]any{"team": "platform"}}},
				"z-file":   map[string]any{"stack_name": "z-file", "path": "template.yaml", "metadata": map[string]any{"tags": []any{"format"}, "labels": map[string]any{"team": "platform"}}},
			}
			components["z-file"].(map[string]any)["dependencies"] = map[string]any{"components": []any{map[string]any{"component": "a-inline"}}}
			stacks := map[string]any{"dev": map[string]any{"components": map[string]any{cfg.CloudFormationComponentType: components}}}
			oldDescribe := executeDescribeStacks
			executeDescribeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
				return stacks, nil
			}
			t.Cleanup(func() { executeDescribeStacks = oldDescribe })
			oldAffected := affectedCloudFormationComponentsFunc
			affectedCloudFormationComponentsFunc = func(_ *component.ExecutionContext, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
				return []schema.Affected{{Component: "a-inline", Stack: "dev", ComponentType: cfg.CloudFormationComponentType}, {Component: "z-file", Stack: "dev", ComponentType: cfg.CloudFormationComponentType}}, nil
			}
			t.Cleanup(func() { affectedCloudFormationComponentsFunc = oldAffected })
			var visited []string
			installExecutorSeamStubs(t, executorSeamStubs{
				initCliConfig: func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, nil
				},
				processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
					visited = append(visited, info.ComponentFromArg)
					info.ComponentIsEnabled = true
					info.ComponentSection = components[info.ComponentFromArg].(map[string]any)
					return info, nil
				},
				provisionAndResolveComponentPath: func(_ context.Context, _ provisioner.OutputWriters, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _, _ string) (string, bool, error) {
					return dir, false, nil
				},
				getHooks: noopGetHooks,
			})
			flags := map[string]any{"check": false}
			output := captureStderr(t, func() {
				require.NoError(t, executeBulk(&component.ExecutionContext{Flags: flags}, &schema.AtmosConfiguration{}, &selection.info, OperationFmt))
			})
			require.Equal(t, []string{"a-inline", "z-file"}, visited)
			require.Contains(t, output, "a-inline: skipped")
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			formatted, err := formatTemplate(original)
			require.NoError(t, err)
			require.NotEqual(t, original, formatted)
			require.Equal(t, formatted, string(contents))
			require.Equal(t, map[string]any{"check": false}, flags)
		})
	}
}

func TestRunFmt_InlineSkipOnlyInBulk(t *testing.T) {
	spec := &stackSpec{StackName: "inline", TemplateBody: "Resources: {}"}
	_, err := runFmt(spec, nil, map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationFmtRequiresPath)
	flags := bulkOperationFlags(OperationFmt, nil)
	summary, err := runFmt(spec, flags, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, true, summary["skipped"])
}

func TestBulkOperationFlags_IsolatesCaller(t *testing.T) {
	original := map[string]any{"check": true}
	copy := bulkOperationFlags(OperationFmt, original)
	copy["check"] = false
	require.Equal(t, true, original["check"])
	original["other"] = true
	require.NotContains(t, copy, "other")
}
