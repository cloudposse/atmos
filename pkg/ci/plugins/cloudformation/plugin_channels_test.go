package cloudformation

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSummaryFailureStillWritesOutputs uses the real file writer behind GitHub
// Actions: an invalid summary destination must not suppress a writable output file.
func TestSummaryFailureStillWritesOutputs(t *testing.T) {
	for _, failure := range []string{"summary path", "summary template"} {
		for _, code := range []int{0, 1} {
			t.Run(failure+strconv.Itoa(code), func(t *testing.T) {
				root := t.TempDir()
				outputs, summary := filepath.Join(root, "outputs"), filepath.Join(root, "summary")
				config := &schema.AtmosConfiguration{}
				wantError := errUtils.ErrCISummaryWriteFailed
				if failure == "summary path" {
					require.NoError(t, os.Mkdir(summary, 0o755))
				} else {
					config.CI.Summary.Template = "missing-template"
					wantError = errUtils.ErrTemplateEvaluation
				}
				ctx := &plugin.HookContext{Provider: fakeProvider{writer: provider.NewFileOutputWriter(outputs, summary)}, TemplateLoader: templates.NewLoader(nil), Config: config, Command: "apply", ExitCode: code,
					Aggregate: &schema.CloudFormationCIResult{ChangeSetName: "reviewed", HasChanges: true, StackStatus: "ROLLBACK_COMPLETE"}}
				if code != 0 {
					ctx.CommandError = errUtils.ErrAwsCloudFormationOperationFailed
				}
				err := (&Plugin{}).onAfterOperation(ctx)
				require.ErrorIs(t, err, wantError)
				data, err := os.ReadFile(outputs)
				require.NoError(t, err)
				assert.Contains(t, string(data), "has_changes=true\n")
				assert.Contains(t, string(data), "changeset_name=reviewed\n")
				assert.Contains(t, string(data), "exit_code="+strconv.Itoa(code)+"\n")
				assert.Equal(t, code, ctx.ExitCode, "summary failures do not rewrite operation outcome")
			})
		}
	}
}

// TestOutputFailureDoesNotFailSummary keeps the opposite channel independent,
// while respecting explicit output disablement even when summary writing fails.
func TestOutputFailureDoesNotFailSummary(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(disabled), func(t *testing.T) {
			root := t.TempDir()
			outputs, summary := filepath.Join(root, "outputs"), filepath.Join(root, "summary")
			config := &schema.AtmosConfiguration{}
			enabled := !disabled
			config.CI.Output.Enabled = &enabled
			if disabled {
				require.NoError(t, os.Mkdir(summary, 0o755))
			} else {
				require.NoError(t, os.Mkdir(outputs, 0o755))
			}
			ctx := &plugin.HookContext{Provider: fakeProvider{writer: provider.NewFileOutputWriter(outputs, summary)}, TemplateLoader: templates.NewLoader(nil), Config: config, Command: "apply"}
			err := (&Plugin{}).onAfterOperation(ctx)
			if disabled {
				require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
				assert.NoFileExists(t, outputs)
			} else {
				require.NoError(t, err, "output failures remain diagnostic-only")
				data, err := os.ReadFile(summary)
				require.NoError(t, err)
				assert.Contains(t, string(data), "CloudFormation Apply Summary")
			}
		})
	}
}
