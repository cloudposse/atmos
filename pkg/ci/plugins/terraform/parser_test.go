package terraform

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

func TestParsePlanJSON(t *testing.T) {
	tests := []struct {
		name            string
		inputFile       string
		wantHasChanges  bool
		wantCreate      int
		wantChange      int
		wantReplace     int
		wantDestroy     int
		wantCreatedRes  []string
		wantUpdatedRes  []string
		wantReplacedRes []string
		wantDeletedRes  []string
		wantImportedRes []string
	}{
		{
			name:           "creates only",
			inputFile:      "testdata/plan_json/success_create.json",
			wantHasChanges: true,
			wantCreate:     2,
			wantCreatedRes: []string{
				"aws_instance.web",
				"aws_security_group.allow_http",
			},
		},
		{
			name:           "destroys only",
			inputFile:      "testdata/plan_json/success_destroy.json",
			wantHasChanges: true,
			wantDestroy:    5,
			wantDeletedRes: []string{
				"aws_instance.old[0]",
				"aws_instance.old[1]",
				"aws_instance.old[2]",
				"aws_instance.old[3]",
				"aws_instance.old[4]",
			},
		},
		{
			name:           "changes only",
			inputFile:      "testdata/plan_json/success_change.json",
			wantHasChanges: true,
			wantChange:     2,
			wantUpdatedRes: []string{
				"aws_instance.web",
				"aws_security_group.allow_http",
			},
		},
		{
			name:            "replace (delete then create)",
			inputFile:       "testdata/plan_json/success_replace.json",
			wantHasChanges:  true,
			wantReplace:     1,
			wantReplacedRes: []string{"aws_instance.replaced"},
		},
		{
			name:           "mixed operations",
			inputFile:      "testdata/plan_json/success_mixed.json",
			wantHasChanges: true,
			wantCreate:     2,
			wantChange:     1,
			wantDestroy:    1,
			wantCreatedRes: []string{"aws_instance.new", "aws_instance.new2"},
			wantUpdatedRes: []string{"aws_instance.updated"},
			wantDeletedRes: []string{"aws_instance.deleted"},
		},
		{
			name:           "no changes",
			inputFile:      "testdata/plan_json/success_no_changes.json",
			wantHasChanges: false,
		},
		{
			name:           "with outputs",
			inputFile:      "testdata/plan_json/success_with_outputs.json",
			wantHasChanges: true,
			wantCreate:     1,
			wantCreatedRes: []string{"aws_s3_bucket.main"},
		},
		{
			name:            "import",
			inputFile:       "testdata/plan_json/success_import.json",
			wantHasChanges:  false,
			wantImportedRes: []string{"aws_instance.imported"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.inputFile)
			require.NoError(t, err)

			result, err := ParsePlanJSON(data)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.wantHasChanges, result.HasChanges, "HasChanges mismatch")

			tfData, ok := result.Data.(*plugin.TerraformOutputData)
			require.True(t, ok, "Data should be *TerraformOutputData")

			assert.Equal(t, tt.wantCreate, tfData.ResourceCounts.Create, "Create count mismatch")
			assert.Equal(t, tt.wantChange, tfData.ResourceCounts.Change, "Change count mismatch")
			assert.Equal(t, tt.wantReplace, tfData.ResourceCounts.Replace, "Replace count mismatch")
			assert.Equal(t, tt.wantDestroy, tfData.ResourceCounts.Destroy, "Destroy count mismatch")

			if len(tt.wantCreatedRes) > 0 {
				assert.ElementsMatch(t, tt.wantCreatedRes, tfData.CreatedResources, "CreatedResources mismatch")
			}
			if len(tt.wantUpdatedRes) > 0 {
				assert.ElementsMatch(t, tt.wantUpdatedRes, tfData.UpdatedResources, "UpdatedResources mismatch")
			}
			if len(tt.wantReplacedRes) > 0 {
				assert.ElementsMatch(t, tt.wantReplacedRes, tfData.ReplacedResources, "ReplacedResources mismatch")
			}
			if len(tt.wantDeletedRes) > 0 {
				assert.ElementsMatch(t, tt.wantDeletedRes, tfData.DeletedResources, "DeletedResources mismatch")
			}
			if len(tt.wantImportedRes) > 0 {
				assert.ElementsMatch(t, tt.wantImportedRes, tfData.ImportedResources, "ImportedResources mismatch")
			}
		})
	}
}

func TestParsePlanJSON_WithOutputs(t *testing.T) {
	data, err := os.ReadFile("testdata/plan_json/success_with_outputs.json")
	require.NoError(t, err)

	result, err := ParsePlanJSON(data)
	require.NoError(t, err)

	tfData, ok := result.Data.(*plugin.TerraformOutputData)
	require.True(t, ok)

	// Check that outputs were parsed.
	assert.Len(t, tfData.Outputs, 2)
	assert.Contains(t, tfData.Outputs, "bucket_arn")
	assert.Contains(t, tfData.Outputs, "bucket_name")
	assert.Equal(t, "arn:aws:s3:::my-bucket", tfData.Outputs["bucket_arn"].Value)
	assert.Equal(t, "my-bucket", tfData.Outputs["bucket_name"].Value)
}

func TestParsePlanJSON_InvalidJSON(t *testing.T) {
	_, err := ParsePlanJSON([]byte("not valid json"))
	assert.Error(t, err)
}

func TestParseOutputJSON(t *testing.T) {
	tests := []struct {
		name        string
		inputFile   string
		wantOutputs map[string]struct {
			value     any
			sensitive bool
			typeStr   string
		}
	}{
		{
			name:      "simple strings",
			inputFile: "testdata/output_json/simple_strings.json",
			wantOutputs: map[string]struct {
				value     any
				sensitive bool
				typeStr   string
			}{
				"bucket_name": {value: "my-bucket", sensitive: false, typeStr: "string"},
				"region":      {value: "us-east-1", sensitive: false, typeStr: "string"},
			},
		},
		{
			name:      "sensitive values",
			inputFile: "testdata/output_json/sensitive_values.json",
			wantOutputs: map[string]struct {
				value     any
				sensitive bool
				typeStr   string
			}{
				"api_key":           {value: "super-secret-key-12345", sensitive: true, typeStr: "string"},
				"database_password": {value: "db-password-xyz", sensitive: true, typeStr: "string"},
				"public_endpoint":   {value: "https://api.example.com", sensitive: false, typeStr: "string"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.inputFile)
			require.NoError(t, err)

			outputs, err := ParseOutputJSON(data)
			require.NoError(t, err)

			for name, want := range tt.wantOutputs {
				got, ok := outputs[name]
				assert.True(t, ok, "output %q not found", name)
				assert.Equal(t, want.sensitive, got.Sensitive, "Sensitive mismatch for %s", name)
				assert.Equal(t, want.value, got.Value, "Value mismatch for %s", name)
				assert.Equal(t, want.typeStr, got.Type, "Type mismatch for %s", name)
			}
		})
	}
}

func TestParseOutputJSON_ComplexTypes(t *testing.T) {
	data, err := os.ReadFile("testdata/output_json/complex_types.json")
	require.NoError(t, err)

	outputs, err := ParseOutputJSON(data)
	require.NoError(t, err)

	// Check list type.
	instanceIDs, ok := outputs["instance_ids"]
	require.True(t, ok)
	assert.Equal(t, "list", instanceIDs.Type)
	assert.False(t, instanceIDs.Sensitive)

	// Check map type.
	tags, ok := outputs["tags"]
	require.True(t, ok)
	assert.Equal(t, "map", tags.Type)
	assert.False(t, tags.Sensitive)

	// Check object type.
	config, ok := outputs["config"]
	require.True(t, ok)
	assert.Equal(t, "object", config.Type)
	assert.False(t, config.Sensitive)
}

func TestParseOutputJSON_Mixed(t *testing.T) {
	data, err := os.ReadFile("testdata/output_json/mixed.json")
	require.NoError(t, err)

	outputs, err := ParseOutputJSON(data)
	require.NoError(t, err)

	assert.Len(t, outputs, 5)

	// Check non-sensitive string.
	bucketArn, ok := outputs["bucket_arn"]
	require.True(t, ok)
	assert.Equal(t, "arn:aws:s3:::prod-bucket", bucketArn.Value)
	assert.False(t, bucketArn.Sensitive)

	// Check sensitive value.
	apiKey, ok := outputs["api_key"]
	require.True(t, ok)
	assert.True(t, apiKey.Sensitive)

	// Check object type.
	config, ok := outputs["config"]
	require.True(t, ok)
	assert.Equal(t, "object", config.Type)

	// Check list type.
	instanceIDs, ok := outputs["instance_ids"]
	require.True(t, ok)
	assert.Equal(t, "list", instanceIDs.Type)
}

func TestParseOutputJSON_InvalidJSON(t *testing.T) {
	_, err := ParseOutputJSON([]byte("not valid json"))
	assert.Error(t, err)
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name       string
		inputFile  string
		wantErrors []string
	}{
		{
			name:      "plan failure",
			inputFile: "testdata/stdout/plan_failure.txt",
			wantErrors: []string{
				"Invalid provider configuration",
				"Missing required argument",
			},
		},
		{
			name:      "apply failure",
			inputFile: "testdata/stdout/apply_failure.txt",
			wantErrors: []string{
				"Error creating EC2 instance: UnauthorizedOperation: You are not authorized to perform this operation.",
				"Error launching source instance: InvalidAMIID.NotFound: The image id '[ami-invalid]' does not exist",
			},
		},
		{
			name:       "apply success (no errors)",
			inputFile:  "testdata/stdout/apply_success.txt",
			wantErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.inputFile)
			require.NoError(t, err)

			errors := ExtractErrors(string(data))
			assert.Equal(t, tt.wantErrors, errors)
		})
	}
}

func TestExtractWarnings(t *testing.T) {
	data, err := os.ReadFile("testdata/stdout/warnings.txt")
	require.NoError(t, err)

	warnings := ExtractWarnings(string(data))

	assert.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "Argument is deprecated")
	assert.Contains(t, warnings[1], "Applied changes may be incomplete")
}

// Legacy tests for stdout parsing (fallback behavior).
func TestParsePlanOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		hasChanges bool
		hasErrors  bool
		create     int
		change     int
		destroy    int
	}{
		{
			name: "plan with changes",
			output: `
Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami           = "ami-12345678"
      + instance_type = "t2.micro"
    }

Plan: 2 to add, 1 to change, 0 to destroy.
`,
			hasChanges: true,
			create:     2,
			change:     1,
			destroy:    0,
		},
		{
			name: "plan no changes",
			output: `
No changes. Your infrastructure matches the configuration.

Terraform has compared your real infrastructure against your configuration
and found no differences, so no changes are needed.
`,
			hasChanges: false,
			create:     0,
			change:     0,
			destroy:    0,
		},
		{
			name: "plan with destroy",
			output: `
Plan: 0 to add, 0 to change, 5 to destroy.
`,
			hasChanges: true,
			create:     0,
			change:     0,
			destroy:    5,
		},
		{
			name: "plan with errors",
			output: `
Error: Invalid provider configuration

Provider "aws" requires explicit configuration.

Error: Reference to undeclared resource
`,
			hasChanges: false,
			hasErrors:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParsePlanOutput(tt.output)
			require.NotNil(t, result)

			assert.Equal(t, tt.hasChanges, result.HasChanges, "HasChanges mismatch")
			assert.Equal(t, tt.hasErrors, result.HasErrors, "HasErrors mismatch")

			if data, ok := result.Data.(*plugin.TerraformOutputData); ok {
				assert.Equal(t, tt.create, data.ResourceCounts.Create, "Create count mismatch")
				assert.Equal(t, tt.change, data.ResourceCounts.Change, "Change count mismatch")
				assert.Equal(t, tt.destroy, data.ResourceCounts.Destroy, "Destroy count mismatch")
			}
		})
	}
}

func TestParseApplyOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		hasChanges bool
		hasErrors  bool
		create     int
		change     int
		destroy    int
	}{
		{
			name: "apply complete with changes",
			output: `
aws_instance.example: Creating...
aws_instance.example: Creation complete after 45s [id=i-12345678]

Apply complete! Resources: 2 added, 1 changed, 0 destroyed.
`,
			hasChanges: true,
			create:     2,
			change:     1,
			destroy:    0,
		},
		{
			name: "apply with destroy",
			output: `
aws_instance.old: Destroying...
aws_instance.old: Destruction complete after 10s

Apply complete! Resources: 0 added, 0 changed, 3 destroyed.
`,
			hasChanges: true,
			create:     0,
			change:     0,
			destroy:    3,
		},
		{
			name: "apply with errors",
			output: `
aws_instance.example: Creating...

Error: Error creating EC2 instance

Error: resource already exists
`,
			hasChanges: false,
			hasErrors:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseApplyOutput(tt.output)
			require.NotNil(t, result)

			assert.Equal(t, tt.hasChanges, result.HasChanges, "HasChanges mismatch")
			assert.Equal(t, tt.hasErrors, result.HasErrors, "HasErrors mismatch")

			if data, ok := result.Data.(*plugin.TerraformOutputData); ok && !tt.hasErrors {
				assert.Equal(t, tt.create, data.ResourceCounts.Create, "Create count mismatch")
				assert.Equal(t, tt.change, data.ResourceCounts.Change, "Change count mismatch")
				assert.Equal(t, tt.destroy, data.ResourceCounts.Destroy, "Destroy count mismatch")
			}
		})
	}
}

func TestParseOutput(t *testing.T) {
	t.Run("plan command", func(t *testing.T) {
		result := ParseOutput("Plan: 1 to add, 0 to change, 0 to destroy.", "plan")
		assert.True(t, result.HasChanges)
	})

	t.Run("apply command", func(t *testing.T) {
		result := ParseOutput("Apply complete! Resources: 1 added, 0 changed, 0 destroyed.", "apply")
		assert.True(t, result.HasChanges)
	})

	t.Run("unknown command", func(t *testing.T) {
		result := ParseOutput("some output", "unknown")
		require.NotNil(t, result)
		assert.False(t, result.HasChanges)
	})
}

func TestBuildChangeSummary(t *testing.T) {
	tests := []struct {
		name   string
		counts plugin.ResourceCounts
		want   string
	}{
		{
			name:   "no changes",
			counts: plugin.ResourceCounts{},
			want:   "No changes",
		},
		{
			name:   "single create",
			counts: plugin.ResourceCounts{Create: 1},
			want:   "1 resource to add",
		},
		{
			name:   "multiple creates",
			counts: plugin.ResourceCounts{Create: 3},
			want:   "3 resources to add",
		},
		{
			name:   "create and destroy",
			counts: plugin.ResourceCounts{Create: 2, Destroy: 1},
			want:   "2 resources to add, 1 resource to destroy",
		},
		{
			name:   "all types",
			counts: plugin.ResourceCounts{Create: 1, Change: 2, Replace: 3, Destroy: 4},
			want:   "1 resource to add, 2 resources to change, 3 resources to replace, 4 resources to destroy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildChangeSummary(tt.counts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatType(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "nil",
			input: nil,
			want:  "",
		},
		{
			name:  "string",
			input: "string",
			want:  "string",
		},
		{
			name:  "list",
			input: []any{"list", "string"},
			want:  "list",
		},
		{
			name:  "map",
			input: []any{"map", "string"},
			want:  "map",
		},
		{
			name:  "object",
			input: []any{"object", map[string]any{"key": "string"}},
			want:  "object",
		},
		{
			name:  "empty array",
			input: []any{},
			want:  "complex",
		},
		{
			name:  "number",
			input: 42,
			want:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatType(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
