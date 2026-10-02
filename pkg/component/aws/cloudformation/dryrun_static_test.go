package cloudformation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// templateOfSize returns a valid YAML template body of exactly n bytes.
func templateOfSize(n int) string {
	const head = "Resources: {}\n#"
	return head + strings.Repeat("a", n-len(head))
}

func s3TargetBlock(extra map[string]any) map[string]any {
	block := map[string]any{"kind": kindAwsS3, "bucket": "templates", "region": "us-east-1"}
	for k, v := range extra {
		block[k] = v
	}
	return block
}

func dryRunSection(template string, targets map[string]any, extra map[string]any) map[string]any {
	section := map[string]any{"stack_name": "vpc", "template": template}
	if targets != nil {
		provision := map[string]any{"targets": targets}
		for k, v := range extra {
			provision[k] = v
		}
		section["provision"] = provision
	}
	return section
}

func runDryRun(t *testing.T, section map[string]any, flags map[string]any, operation Operation) error {
	t.Helper()
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", ComponentSection: section}
	return validateDryRun(&schema.AtmosConfiguration{}, info, flags, operation)
}

// Dry run must reject packaging configuration the real run rejects before it mutates anything.
func TestDryRunRejectsInvalidPackagingConfiguration(t *testing.T) {
	large := templateOfSize(templateInlineSizeLimit + 1)
	noRegion := map[string]any{"artifacts": map[string]any{"kind": kindAwsS3, "bucket": "templates"}}
	tests := []struct {
		name      string
		section   map[string]any
		flags     map[string]any
		operation Operation
		wantErr   error
	}{
		{
			name:      "selected s3 target without region",
			section:   dryRunSection("Resources: {}", noRegion, nil),
			flags:     map[string]any{"target": "artifacts"},
			operation: OperationApply,
			wantErr:   errUtils.ErrInvalidAwsCloudFormationSettings,
		},
		{
			name:      "default s3 target without region",
			section:   dryRunSection("Resources: {}", noRegion, map[string]any{"default": "artifacts"}),
			operation: OperationApply,
			wantErr:   errUtils.ErrInvalidAwsCloudFormationSettings,
		},
		{
			name:      "oversized template packaged through an s3 target without region",
			section:   dryRunSection(large, noRegion, nil),
			operation: OperationDiff,
			wantErr:   errUtils.ErrInvalidAwsCloudFormationSettings,
		},
		{
			name:      "oversized template without any packaging target",
			section:   dryRunSection(large, map[string]any{"deploy": map[string]any{"kind": "aws/cloudformation"}}, nil),
			operation: OperationValidate,
			wantErr:   errUtils.ErrInvalidAwsCloudFormationSettings,
		},
		{
			name:      "oversized template with no provision section",
			section:   dryRunSection(large, nil, nil),
			operation: OperationChangesetCreate,
			wantErr:   errUtils.ErrInvalidAwsCloudFormationSettings,
		},
		{
			name:      "apply with an unknown target",
			section:   dryRunSection("Resources: {}", map[string]any{"artifacts": s3TargetBlock(nil)}, nil),
			flags:     map[string]any{"target": "missing"},
			operation: OperationApply,
			wantErr:   errUtils.ErrProvisionTargetNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDryRun(t, tt.section, tt.flags, tt.operation)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// Negative path: valid or irrelevant packaging configuration must keep passing, including a
// template exactly at the inline limit and operations that never package.
func TestDryRunAcceptsValidPackagingConfiguration(t *testing.T) {
	atLimit := templateOfSize(templateInlineSizeLimit)
	large := templateOfSize(templateInlineSizeLimit + 1)
	artifacts := map[string]any{"artifacts": s3TargetBlock(nil)}
	noRegion := map[string]any{"artifacts": map[string]any{"kind": kindAwsS3, "bucket": "templates"}}
	tests := []struct {
		name      string
		section   map[string]any
		operation Operation
	}{
		{name: "template at the inline limit needs no target", section: dryRunSection(atLimit, nil, nil), operation: OperationDiff},
		{name: "oversized template with a valid s3 target", section: dryRunSection(large, artifacts, nil), operation: OperationDiff},
		{name: "apply with the implicit target", section: dryRunSection("Resources: {}", nil, nil), operation: OperationApply},
		{name: "unselected s3 target without region is not validated", section: dryRunSection("Resources: {}", noRegion, nil), operation: OperationApply},
		{name: "oversized template on a non-packaging verb", section: dryRunSection(large, nil, nil), operation: OperationDelete},
		{name: "oversized template on render", section: dryRunSection(large, nil, nil), operation: OperationRender},
		{name: "deferred template body", section: dryRunSection("{{ .vars.body }}", nil, nil), operation: OperationApply},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, runDryRun(t, tt.section, nil, tt.operation))
		})
	}
}

// A template file already on disk is measured without being read into the spec.
func TestDryRunMeasuresTemplateFileSize(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.yaml")
	small := filepath.Join(dir, "small.yaml")
	require.NoError(t, os.WriteFile(large, []byte(templateOfSize(templateInlineSizeLimit+1)), 0o600))
	require.NoError(t, os.WriteFile(small, []byte("Resources: {}"), 0o600))

	section := func(path string) map[string]any {
		return map[string]any{"stack_name": "vpc", "path": path}
	}
	require.ErrorIs(t, runDryRun(t, section(large), nil, OperationDiff), errUtils.ErrInvalidAwsCloudFormationSettings)
	require.NoError(t, runDryRun(t, section(small), nil, OperationDiff))
	// A template that cannot be located statically is not reported as oversized.
	require.NoError(t, runDryRun(t, section(filepath.Join(dir, "absent.yaml")), nil, OperationDiff))
}

func stackSetSection(block map[string]any) map[string]any {
	block["kind"] = kindAwsStackSet
	return map[string]any{"stack_name": "fleet", "template": "Resources: {}", "provision": map[string]any{"targets": map[string]any{"fleet": block}}}
}

// Dry run must apply the StackSet validators the real run uses, plus the permission_model check.
func TestDryRunValidatesStackSetTargets(t *testing.T) {
	tests := []struct {
		name      string
		block     map[string]any
		operation Operation
		wantErr   bool
	}{
		{
			name:      "service managed with accounts and regions",
			block:     map[string]any{"permission_model": "SERVICE_MANAGED", "accounts": []any{"111111111111"}, "regions": []any{"us-east-1"}},
			operation: OperationStackSetCreate,
			wantErr:   true,
		},
		{
			name:      "service managed with accounts only",
			block:     map[string]any{"permission_model": "SERVICE_MANAGED", "accounts": []any{"111111111111"}},
			operation: OperationStackSetCreate,
		},
		{
			name:      "service managed update does not target instances",
			block:     map[string]any{"permission_model": "SERVICE_MANAGED", "accounts": []any{"111111111111"}, "regions": []any{"us-east-1"}},
			operation: OperationStackSetUpdate,
		},
		{
			name:      "self managed with accounts and regions",
			block:     map[string]any{"permission_model": "SELF_MANAGED", "accounts": []any{"111111111111"}, "regions": []any{"us-east-1"}},
			operation: OperationStackSetCreate,
		},
		{
			name:      "default permission model with accounts and regions",
			block:     map[string]any{"accounts": []any{"111111111111"}, "regions": []any{"us-east-1"}},
			operation: OperationStackSetCreate,
		},
		{
			name:      "permission model typo on create",
			block:     map[string]any{"permission_model": "SELF-MANAGED"},
			operation: OperationStackSetCreate,
			wantErr:   true,
		},
		{
			name:      "permission model typo on update",
			block:     map[string]any{"permission_model": "self_managed"},
			operation: OperationStackSetUpdate,
			wantErr:   true,
		},
		{
			name:      "deferred permission model is not judged",
			block:     map[string]any{"permission_model": "{{ .vars.model }}"},
			operation: OperationStackSetCreate,
		},
		{
			name:      "deferred accounts are not judged against service managed",
			block:     map[string]any{"permission_model": "SERVICE_MANAGED", "accounts": "!exec accounts", "regions": []any{"us-east-1"}},
			operation: OperationStackSetCreate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDryRun(t, stackSetSection(tt.block), nil, tt.operation)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStackSetPermissionModelTypoListsValidModels(t *testing.T) {
	err := runDryRun(t, stackSetSection(map[string]any{"permission_model": "SELF-MANAGED"}), nil, OperationStackSetCreate)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "SELF_MANAGED")
	assert.Contains(t, hints, "SERVICE_MANAGED")
}

// The dry-run line names the verb, component and stack, so bulk output is distinguishable.
func TestDryRunOutputNamesVerbComponentAndStack(t *testing.T) {
	for _, tt := range []struct {
		operation Operation
		want      string
	}{
		{OperationApply, "Dry run: apply vpc in dev: configuration validated; no AWS calls made"},
		{OperationDelete, "Dry run: delete vpc in dev: configuration validated; no AWS calls made"},
		{OperationChangesetCreate, "Dry run: changeset-create vpc in dev: configuration validated; no AWS calls made"},
	} {
		t.Run(string(tt.operation), func(t *testing.T) {
			var err error
			out := captureStderr(t, func() {
				err = runDryRun(t, dryRunSection("Resources: {}", nil, nil), nil, tt.operation)
			})
			require.NoError(t, err)
			assert.Contains(t, normalizeUIOutput(out), tt.want)
		})
	}
}

// A real apply rejects a `kind: aws/stackset` target before packaging, so the
// dry run must reject it too instead of reporting the configuration valid. The
// StackSet verbs themselves still accept the same target.
func TestDryRunApplyRejectsStackSetTarget(t *testing.T) {
	targets := map[string]any{"fleet": map[string]any{
		"kind": kindAwsStackSet, "accounts": []any{"111111111111"}, "regions": []any{"us-east-1"},
	}}
	section := dryRunSection("Resources: {}", targets, nil)

	err := runDryRun(t, section, map[string]any{targetKey: "fleet"}, OperationApply)
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackSetTargetNotApplicable)

	require.NoError(t, runDryRun(t, section, map[string]any{targetKey: "fleet"}, OperationStackSetCreate))
}
