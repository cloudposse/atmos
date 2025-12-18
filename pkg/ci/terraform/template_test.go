package terraform

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
)

var regenerateGolden = flag.Bool("regenerate-golden", false, "Regenerate golden files")

// TestTemplateRendering tests various template scenarios with different context combinations.
func TestTemplateRendering(t *testing.T) {
	tests := []struct {
		name            string
		templateName    string
		context         *TerraformTemplateContext
		wantContains    []string
		wantNotContains []string
	}{
		// Plan scenarios.
		{
			name:         "plan with creates only",
			templateName: "plan",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "vpc",
					ComponentType: "terraform",
					Stack:         "dev-us-east-1",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{Create: 3},
				CreatedResources: []string{
					"aws_vpc.main",
					"aws_subnet.a",
					"aws_subnet.b",
				},
			},
			wantContains: []string{
				"Changes Found",
				"CREATE-3-success",
				"vpc",
				"dev-us-east-1",
				"aws_vpc.main",
				"+ aws_subnet",
			},
			wantNotContains: []string{
				"DESTROY",
				"FAILED",
				"CAUTION",
			},
		},
		{
			name:         "plan with destroys shows warning",
			templateName: "plan",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "legacy",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources:        ci.ResourceCounts{Destroy: 5},
				DeletedResources: []string{"aws_instance.old[0]"},
				HasDestroy:       true,
			},
			wantContains: []string{
				"DESTROY-5-critical",
				"CAUTION",
				"Terraform will delete resources",
				"- aws_instance.old",
			},
		},
		{
			name:         "plan with errors",
			templateName: "plan",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "broken",
					ComponentType: "terraform",
					Stack:         "dev",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:  1,
						HasErrors: true,
						Errors:    []string{"Invalid provider configuration", "Missing required argument"},
					},
				},
				Resources: ci.ResourceCounts{},
			},
			wantContains: []string{
				"PLAN-FAILED-ff0000",
				"Plan Failed",
				"Error summary",
				":warning:",
				"Invalid provider configuration",
			},
		},
		{
			name:         "plan no changes",
			templateName: "plan",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "stable",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: false,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{},
			},
			wantContains: []string{
				"NO_CHANGE-inactive",
				"No Changes",
			},
			wantNotContains: []string{
				"CREATE",
				"DESTROY",
				"CAUTION",
			},
		},
		{
			name:         "plan with mixed operations",
			templateName: "plan",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "app",
					ComponentType: "terraform",
					Stack:         "staging",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{
					Create:  2,
					Change:  1,
					Replace: 1,
					Destroy: 1,
				},
				CreatedResources:  []string{"aws_instance.new"},
				UpdatedResources:  []string{"aws_instance.updated"},
				ReplacedResources: []string{"aws_instance.replaced"},
				DeletedResources:  []string{"aws_instance.deleted"},
				HasDestroy:        true,
			},
			wantContains: []string{
				"CREATE-2",
				"CHANGE-1",
				"REPLACE-1",
				"DESTROY-1",
				"CAUTION",
				"+ aws_instance.new",
				"~ aws_instance.updated",
				"-/+ aws_instance.replaced",
				"- aws_instance.deleted",
			},
		},
		{
			name:         "plan with imported resources",
			templateName: "plan",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "imports",
					ComponentType: "terraform",
					Stack:         "dev",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: false,
						HasErrors:  false,
					},
				},
				Resources:         ci.ResourceCounts{},
				ImportedResources: []string{"aws_instance.imported"},
			},
			wantContains: []string{
				"<= aws_instance.imported",
				"Import",
			},
		},

		// Apply scenarios.
		{
			name:         "apply success",
			templateName: "apply",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "bucket",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "apply",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{Create: 1},
			},
			wantContains: []string{
				"APPLY-SUCCESS-success",
				"Apply Succeeded",
			},
			wantNotContains: []string{
				"FAILED",
				"Error summary",
			},
		},
		{
			name:         "apply success with outputs",
			templateName: "apply",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "bucket",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "apply",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{Create: 1},
				Outputs: map[string]ci.TerraformOutput{
					"bucket_arn":  {Value: "arn:aws:s3:::prod-bucket", Sensitive: false},
					"secret_key":  {Value: "", Sensitive: true},
					"bucket_name": {Value: "prod-bucket", Sensitive: false},
				},
			},
			wantContains: []string{
				"APPLY-SUCCESS-success",
				"Terraform Outputs",
				"bucket_arn",
				"arn:aws:s3:::prod-bucket",
				"secret_key",
				"*(sensitive)*",
			},
		},
		{
			name:         "apply failure",
			templateName: "apply",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "broken",
					ComponentType: "terraform",
					Stack:         "dev",
					Command:       "apply",
					Result: &ci.OutputResult{
						ExitCode:  1,
						HasErrors: true,
						Errors:    []string{"Error creating resource: permission denied"},
					},
				},
				Resources: ci.ResourceCounts{},
			},
			wantContains: []string{
				"APPLY-FAILED-critical",
				"Apply Failed",
				"Error creating resource",
			},
		},
		{
			name:         "apply no changes",
			templateName: "apply",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "stable",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "apply",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: false,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{},
			},
			wantContains: []string{
				"APPLY-SUCCESS-success",
				"No changes applied",
			},
		},
	}

	p := &Provider{}
	fs := p.GetDefaultTemplates()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load template.
			content, err := fs.ReadFile("templates/" + tt.templateName + ".md")
			require.NoError(t, err)

			// Parse template.
			tmpl, err := template.New(tt.templateName).Parse(string(content))
			require.NoError(t, err)

			// Render template.
			var buf strings.Builder
			err = tmpl.Execute(&buf, tt.context)
			require.NoError(t, err)

			rendered := buf.String()

			// Check expected content.
			for _, want := range tt.wantContains {
				assert.Contains(t, rendered, want, "should contain: %s", want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, rendered, notWant, "should not contain: %s", notWant)
			}
		})
	}
}

// TestTemplateWithCIContext tests that CI context is properly rendered.
func TestTemplateWithCIContext(t *testing.T) {
	p := &Provider{}
	fs := p.GetDefaultTemplates()

	ctx := &TerraformTemplateContext{
		TemplateContext: &ci.TemplateContext{
			Component:     "vpc",
			ComponentType: "terraform",
			Stack:         "dev",
			Command:       "plan",
			CI: &ci.Context{
				SHA:        "abc123def456",
				Repository: "owner/repo",
				Actor:      "testuser",
			},
			Result: &ci.OutputResult{
				ExitCode:   0,
				HasChanges: true,
			},
		},
		Resources:        ci.ResourceCounts{Create: 1},
		CreatedResources: []string{"aws_vpc.main"},
	}

	content, err := fs.ReadFile("templates/plan.md")
	require.NoError(t, err)

	tmpl, err := template.New("plan").Parse(string(content))
	require.NoError(t, err)

	var buf strings.Builder
	err = tmpl.Execute(&buf, ctx)
	require.NoError(t, err)

	rendered := buf.String()

	// Verify CI context is in metadata.
	assert.Contains(t, rendered, "abc123def456")
}

// TestTerraformTemplateContextHelpers tests helper methods.
func TestTerraformTemplateContextHelpers(t *testing.T) {
	t.Run("Target", func(t *testing.T) {
		ctx := &TerraformTemplateContext{
			TemplateContext: &ci.TemplateContext{
				Stack:     "dev",
				Component: "vpc",
			},
		}
		assert.Equal(t, "dev-vpc", ctx.Target())
	})

	t.Run("Target with nil base", func(t *testing.T) {
		ctx := &TerraformTemplateContext{}
		assert.Equal(t, "", ctx.Target())
	})

	t.Run("HasChanges", func(t *testing.T) {
		tests := []struct {
			name      string
			resources ci.ResourceCounts
			want      bool
		}{
			{"no changes", ci.ResourceCounts{}, false},
			{"create only", ci.ResourceCounts{Create: 1}, true},
			{"change only", ci.ResourceCounts{Change: 1}, true},
			{"replace only", ci.ResourceCounts{Replace: 1}, true},
			{"destroy only", ci.ResourceCounts{Destroy: 1}, true},
			{"mixed", ci.ResourceCounts{Create: 1, Destroy: 1}, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx := &TerraformTemplateContext{Resources: tt.resources}
				assert.Equal(t, tt.want, ctx.HasChanges())
			})
		}
	})

	t.Run("TotalChanges", func(t *testing.T) {
		ctx := &TerraformTemplateContext{
			Resources: ci.ResourceCounts{
				Create:  1,
				Change:  2,
				Replace: 3,
				Destroy: 4,
			},
		}
		assert.Equal(t, 10, ctx.TotalChanges())
	})
}

// TestTemplateGolden tests template rendering against golden files for regression testing.
func TestTemplateGolden(t *testing.T) {
	goldenDir := "testdata/golden"

	tests := []struct {
		name         string
		templateName string
		goldenFile   string
		context      *TerraformTemplateContext
	}{
		{
			name:         "plan creates only",
			templateName: "plan",
			goldenFile:   "plan_creates_only.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "vpc",
					ComponentType: "terraform",
					Stack:         "dev-us-east-1",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{Create: 3},
				CreatedResources: []string{
					"aws_vpc.main",
					"aws_subnet.a",
					"aws_subnet.b",
				},
			},
		},
		{
			name:         "plan destroys warning",
			templateName: "plan",
			goldenFile:   "plan_destroys_warning.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "legacy",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources:        ci.ResourceCounts{Destroy: 5},
				DeletedResources: []string{"aws_instance.old[0]", "aws_instance.old[1]"},
				HasDestroy:       true,
			},
		},
		{
			name:         "plan no changes",
			templateName: "plan",
			goldenFile:   "plan_no_changes.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "stable",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: false,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{},
			},
		},
		{
			name:         "plan failure",
			templateName: "plan",
			goldenFile:   "plan_failure.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "broken",
					ComponentType: "terraform",
					Stack:         "dev",
					Command:       "plan",
					Result: &ci.OutputResult{
						ExitCode:  1,
						HasErrors: true,
						Errors:    []string{"Invalid provider configuration", "Missing required argument"},
					},
				},
				Resources: ci.ResourceCounts{},
			},
		},
		{
			name:         "apply success",
			templateName: "apply",
			goldenFile:   "apply_success.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "bucket",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "apply",
					Output:        "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{Create: 1},
			},
		},
		{
			name:         "apply with outputs",
			templateName: "apply",
			goldenFile:   "apply_with_outputs.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "bucket",
					ComponentType: "terraform",
					Stack:         "prod",
					Command:       "apply",
					Output:        "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.",
					Result: &ci.OutputResult{
						ExitCode:   0,
						HasChanges: true,
						HasErrors:  false,
					},
				},
				Resources: ci.ResourceCounts{Create: 1},
				Outputs: map[string]ci.TerraformOutput{
					"bucket_arn":  {Value: "arn:aws:s3:::prod-bucket", Sensitive: false},
					"bucket_name": {Value: "prod-bucket", Sensitive: false},
					"secret_key":  {Value: "", Sensitive: true},
				},
			},
		},
		{
			name:         "apply failure",
			templateName: "apply",
			goldenFile:   "apply_failure.md",
			context: &TerraformTemplateContext{
				TemplateContext: &ci.TemplateContext{
					Component:     "broken",
					ComponentType: "terraform",
					Stack:         "dev",
					Command:       "apply",
					Result: &ci.OutputResult{
						ExitCode:  1,
						HasErrors: true,
						Errors:    []string{"Error creating resource: permission denied"},
					},
				},
				Resources: ci.ResourceCounts{},
			},
		},
	}

	p := &Provider{}
	fs := p.GetDefaultTemplates()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load template.
			content, err := fs.ReadFile("templates/" + tt.templateName + ".md")
			require.NoError(t, err)

			// Parse template.
			tmpl, err := template.New(tt.templateName).Parse(string(content))
			require.NoError(t, err)

			// Render template.
			var buf strings.Builder
			err = tmpl.Execute(&buf, tt.context)
			require.NoError(t, err)

			rendered := buf.String()
			goldenPath := filepath.Join(goldenDir, tt.goldenFile)

			if *regenerateGolden {
				// Write golden file.
				err := os.MkdirAll(goldenDir, 0755)
				require.NoError(t, err)
				err = os.WriteFile(goldenPath, []byte(rendered), 0644)
				require.NoError(t, err)
				t.Logf("Regenerated golden file: %s", goldenPath)
				return
			}

			// Read golden file.
			expected, err := os.ReadFile(goldenPath)
			if os.IsNotExist(err) {
				t.Fatalf("Golden file not found: %s. Run with -regenerate-golden to create it.", goldenPath)
			}
			require.NoError(t, err)

			// Compare.
			assert.Equal(t, string(expected), rendered, "Output does not match golden file. Run with -regenerate-golden to update.")
		})
	}
}
