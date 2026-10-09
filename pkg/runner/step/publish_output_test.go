package step

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPublishResultUI(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, kind         string
		changed, unchanged int
		locations          []string
		metadata           map[string]any
		want               []string
	}{
		{name: "uploaded", kind: "aws/s3", changed: 1, locations: []string{"s3://artifacts/lambda/v1/handler.zip"}, want: []string{"Publish artifacts (aws/s3): 1 uploaded, 0 unchanged", "  → s3://artifacts/lambda/v1/handler.zip"}},
		{name: "unchanged", kind: "aws/s3", unchanged: 1, locations: []string{"s3://artifacts/lambda/v1/handler.zip"}, want: []string{"0 uploaded, 1 unchanged", "  → s3://artifacts/lambda/v1/handler.zip"}},
		{name: "mixed directory", kind: "aws/s3", changed: 1, unchanged: 1, locations: []string{"s3://artifacts/a.txt", "s3://artifacts/b.txt"}, want: []string{"1 uploaded, 1 unchanged", "  → s3://artifacts/a.txt", "  → s3://artifacts/b.txt"}},
		{name: "empty directory", kind: "aws/s3", want: []string{"0 uploaded, 0 unchanged"}},
		{name: "git", kind: "git", changed: 1, locations: []string{"releases/handler.zip"}, metadata: map[string]any{"repository": "deployment", "branch": "release"}, want: []string{"1 written, 0 unchanged", "Repository: deployment (branch release)", "  → releases/handler.zip"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vars := NewVariables()
			var output, data bytes.Buffer
			vars.OutputWriters = OutputWriters{Stdout: &data, Stderr: &output}
			result := publishStepResult("artifacts", tc.kind, &target.PublishResult{Locations: tc.locations, Changed: tc.changed, Unchanged: tc.unchanged, Metadata: tc.metadata})
			reportPublishResult(t.Context(), &schema.WorkflowStep{}, vars, result)
			for _, want := range tc.want {
				assert.Contains(t, output.String(), want)
			}
			assert.Empty(t, data.String())
		})
	}
}

func TestPublishResultRespectsQuietOutput(t *testing.T) {
	t.Parallel()
	for _, suppressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "output none", true: "suppressed context"}[suppressed], func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			step := &schema.WorkflowStep{Output: "none"}
			if suppressed {
				ctx = WithOutputSuppressed(ctx)
				step.Output = ""
			}
			vars := NewVariables()
			var output bytes.Buffer
			vars.OutputWriters = OutputWriters{Stderr: &output}
			reportPublishResult(ctx, step, vars, publishStepResult("artifacts", "aws/s3", &target.PublishResult{Locations: []string{"s3://artifacts/handler.zip"}, Changed: 1}))
			assert.Empty(t, output.String())
		})
	}
}
