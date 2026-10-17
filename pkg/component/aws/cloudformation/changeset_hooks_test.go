package cloudformation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// changesetHookEvents are the four canonical changeset lifecycle events, in the
// order the fixture below declares their marker hooks.
var changesetHookEvents = []hooks.HookEvent{
	hooks.BeforeAwsCloudFormationChangesetCreate,
	hooks.AfterAwsCloudFormationChangesetCreate,
	hooks.BeforeAwsCloudFormationChangesetExecute,
	hooks.AfterAwsCloudFormationChangesetExecute,
}

// writeChangesetHookFixture writes a project whose component declares one marker
// hook per changeset event. Each hook runs this test binary (see TestMain),
// which writes a file named after the event into markerDir, so a test can tell
// exactly which events dispatched without any platform-specific binary.
func writeChangesetHookFixture(t *testing.T, root, markerDir string) {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "cloudformation", "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: "./"
components:
  aws/cloudformation:
    base_path: "components/cloudformation"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"
schemas: {}
logs:
  level: Info
`), 0o644))

	hooksYAML := ""
	for i, event := range changesetHookEvents {
		hooksYAML += fmt.Sprintf(`        marker-%d:
          events:
            - %s
          kind: command
          command: %s
          args: ["-test.run", "^$"]
          env:
            _ATMOS_TEST_WRITE_MARKER: %s
`, i, event, exe, markerPath(markerDir, event))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "test.yaml"), []byte(`vars:
  stage: test
components:
  aws/cloudformation:
    vpc:
      hooks:
`+hooksYAML), 0o644))
}

// markerPath is the file a marker hook writes when its event dispatches.
func markerPath(markerDir string, event hooks.HookEvent) string {
	return filepath.Join(markerDir, strings.NewReplacer("/", "_", ".", "_").Replace(string(event))+".marker")
}

// firedChangesetEvents returns the changeset events whose marker file exists.
func firedChangesetEvents(markerDir string) []hooks.HookEvent {
	var fired []hooks.HookEvent
	for _, event := range changesetHookEvents {
		if _, err := os.Stat(markerPath(markerDir, event)); err == nil {
			fired = append(fired, event)
		}
	}
	return fired
}

// The before/after changeset events must dispatch through runWithHooks for
// changeset create and execute, and must stay silent for every other
// operation, including changeset list/delete, validate, render, and output.
// A dry run reaches the real hook dispatch (getHooks, RunAll, the CI hook) but
// stops runOperation before any AWS call, so no client is needed.
func TestRunWithHooks_ChangesetEventsDispatch(t *testing.T) {
	tests := []struct {
		operation Operation
		want      []hooks.HookEvent
	}{
		{OperationChangesetCreate, []hooks.HookEvent{hooks.BeforeAwsCloudFormationChangesetCreate, hooks.AfterAwsCloudFormationChangesetCreate}},
		{OperationChangesetExecute, []hooks.HookEvent{hooks.BeforeAwsCloudFormationChangesetExecute, hooks.AfterAwsCloudFormationChangesetExecute}},
		{OperationChangesetList, nil},
		{OperationChangesetDelete, nil},
		{OperationValidate, nil},
		{OperationRender, nil},
		{OperationOutput, nil},
		{OperationApply, nil},
		{OperationDiff, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			root := t.TempDir()
			markerDir := t.TempDir()
			writeChangesetHookFixture(t, root, markerDir)

			t.Chdir(root)
			info := schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "test", DryRun: true}
			loadedConfig, err := cfg.InitCliConfig(info, true)
			require.NoError(t, err)

			ctx := &component.ExecutionContext{}
			spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
			require.NoError(t, runWithHooks(ctx, &loadedConfig, &info, tt.operation, spec))

			assert.Equal(t, tt.want, firedChangesetEvents(markerDir))
		})
	}
}
