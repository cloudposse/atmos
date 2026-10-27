package cloudformation

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Exercise real stack discovery and command hooks through Execute. The operation
// handler captures the template that would be sent to AWS, without cloud access.
func TestExecute_SynthesisBeforeTemplateRead(t *testing.T) {
	for _, operation := range []Operation{OperationDiff, OperationApply, OperationChangesetCreate} {
		for _, scenario := range []string{"first-run", "replace-template", "failed-synthesis", "missing-artifact", "skip-hook"} {
			t.Run(string(operation)+"/"+scenario, func(t *testing.T) {
				root := t.TempDir()
				dir := filepath.Join(root, "components", "cloudformation", "demo")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: .
components:
  aws/cloudformation:
    base_path: components/cloudformation
stacks:
  base_path: stacks
  included_paths: ["**/*"]
`), 0o600))
				templatePath := filepath.Join(dir, "template.json")
				const generated = `{"Resources":{"Marker":{"Type":"AWS::SSM::Parameter","Properties":{"Type":"String","Value":"new"}}}}`
				if scenario == "replace-template" || scenario == "failed-synthesis" {
					require.NoError(t, os.WriteFile(templatePath, []byte(`{"Resources":{}}`), 0o600))
				}
				executable, err := os.Executable()
				require.NoError(t, err)
				command := executable
				if scenario == "failed-synthesis" {
					command = "missing-synth-binary"
				}
				outputPath := "template.json" // Relative output also verifies the hook's working directory.
				if scenario == "missing-artifact" {
					outputPath = "wrong-output.json"
				}
				before, after := eventsFor(operation)
				require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(fmt.Sprintf(`name: dev
components:
  aws/cloudformation:
    demo:
      path: template.json
      stack_name: synthesis-test
      region: us-east-1
      hooks:
        synth:
          events: [%s]
          kind: command
          command: %q
          args: ["-test.run", "^$"]
          on_failure: fail
          env:
            _ATMOS_TEST_WRITE_MARKER: %q
            _ATMOS_TEST_MARKER_CONTENT: %q
        after:
          events: [%s]
          when: always
          kind: command
          command: %q
          args: ["-test.run", "^$"]
          env:
            _ATMOS_TEST_WRITE_MARKER: after-marker
`, before, command, outputPath, generated, after, executable)), 0o600))
				t.Chdir(root)
				t.Setenv("AWS_ACCESS_KEY_ID", "testing")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "testing")
				t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
				original := operationHandlers[operation]
				t.Cleanup(func() { operationHandlers[operation] = original })
				called := false
				operationHandlers[operation] = func(_ *opContext, _ CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
					called = true
					assert.JSONEq(t, generated, spec.TemplateBody)
					return summary, nil
				}
				captured := captureHookCI(t)
				info := schema.ConfigAndStacksInfo{ComponentFromArg: "demo", Stack: "dev", Identity: cfg.IdentityFlagDisabledValue}
				if scenario == "skip-hook" {
					previous := viper.Get("skip-hooks")
					viper.Set("skip-hooks", "synth")
					t.Cleanup(func() { viper.Set("skip-hooks", previous) })
				}
				err = Execute(&component.ExecutionContext{ConfigAndStacksInfo: info}, operation)
				if scenario == "first-run" || scenario == "replace-template" {
					require.NoError(t, err)
					assert.True(t, called)
					assert.FileExists(t, filepath.Join(dir, "after-marker"))
				} else {
					require.Error(t, err)
					assert.False(t, called, "failed synthesis or missing files must stop deployment")
					assert.NoFileExists(t, filepath.Join(dir, "after-marker"))
					require.Len(t, *captured, 1)
					assert.Same(t, err, (*captured)[0].CommandError)
					assert.Equal(t, after, (*captured)[0].Event)
				}
			})
		}
	}
}
