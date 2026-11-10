package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// planData parses plan stdout and returns the result with its typed data.
func planData(t *testing.T, output string) (*plugin.OutputResult, *plugin.TerraformOutputData) {
	t.Helper()
	result := ParsePlanOutput(output)
	data, ok := result.Data.(*plugin.TerraformOutputData)
	require.True(t, ok, "Data should be *plugin.TerraformOutputData")
	return result, data
}

// applyData parses apply stdout and returns its typed data.
func applyData(t *testing.T, output string) *plugin.TerraformOutputData {
	t.Helper()
	data, ok := ParseApplyOutput(output).Data.(*plugin.TerraformOutputData)
	require.True(t, ok, "Data should be *plugin.TerraformOutputData")
	return data
}

func TestParsePlanOutput_ImportsAndMoves(t *testing.T) {
	t.Run("imports alongside changes are listed", func(t *testing.T) {
		result, data := planData(t, `
  # aws_s3_bucket.default[0] will be updated in-place
  # (imported from "example-data")
  ~ resource "aws_s3_bucket" "default" {
      ~ tags = {}
    }

  # aws_s3_bucket_policy.default[0] will be imported
    resource "aws_s3_bucket_policy" "default" {
        id = "example-data"
    }

Plan: 2 to import, 0 to add, 1 to change, 0 to destroy.
`)
		assert.True(t, result.HasChanges)
		assert.Equal(t, plugin.ResourceCounts{Change: 1}, data.ResourceCounts)
		assert.Equal(t, []string{"aws_s3_bucket.default[0]"}, data.UpdatedResources)
		assert.ElementsMatch(t, []string{"aws_s3_bucket.default[0]", "aws_s3_bucket_policy.default[0]"}, data.ImportedResources)
	})

	t.Run("import-only plan is a change", func(t *testing.T) {
		result, data := planData(t, `
  # aws_s3_bucket.b will be imported
    resource "aws_s3_bucket" "b" {
        id = "example-data"
    }

Plan: 1 to import, 0 to add, 0 to change, 0 to destroy.
`)
		assert.True(t, result.HasChanges)
		assert.False(t, data.HasOutputChanges)
		assert.Equal(t, plugin.ResourceCounts{}, data.ResourceCounts)
		assert.Equal(t, []string{"aws_s3_bucket.b"}, data.ImportedResources)
		assert.Equal(t, "1 resource to import", data.ChangedResult)
	})

	t.Run("moved-only plan is a change", func(t *testing.T) {
		result, data := planData(t, `
  # aws_s3_bucket.a has moved to aws_s3_bucket.b
    resource "aws_s3_bucket" "b" {
        id = "example-data"
    }

Plan: 0 to add, 0 to change, 0 to destroy.
`)
		assert.True(t, result.HasChanges)
		assert.Equal(t, plugin.ResourceCounts{}, data.ResourceCounts)
		assert.Equal(t, []plugin.MovedResource{{From: "aws_s3_bucket.a", To: "aws_s3_bucket.b"}}, data.MovedResources)
		assert.Equal(t, "1 resource to move", data.ChangedResult)
	})

	t.Run("move combined with an update is listed", func(t *testing.T) {
		_, data := planData(t, `
  # aws_s3_bucket.b will be updated in-place
  # (moved from aws_s3_bucket.a)
  ~ resource "aws_s3_bucket" "b" {
      ~ tags = {}
    }

Plan: 0 to add, 1 to change, 0 to destroy.
`)
		assert.Equal(t, []plugin.MovedResource{{From: "aws_s3_bucket.a", To: "aws_s3_bucket.b"}}, data.MovedResources)
		assert.Empty(t, data.ImportedResources)
	})

	t.Run("a plan with no imports or moves stays a no-op", func(t *testing.T) {
		result, data := planData(t, "\nPlan: 0 to add, 0 to change, 0 to destroy.\n")
		assert.False(t, result.HasChanges)
		assert.Empty(t, data.ChangedResult)
	})
}

func TestParsePlanOutput_ResourceAddressForms(t *testing.T) {
	t.Run("for_each key containing spaces", func(t *testing.T) {
		_, data := planData(t, `
  # module.app["blue green"].aws_instance.x["my key"] will be created
  + resource "aws_instance" "x" {}

Plan: 1 to add, 0 to change, 0 to destroy.
`)
		assert.Equal(t, []string{`module.app["blue green"].aws_instance.x["my key"]`}, data.CreatedResources)
	})

	t.Run("tainted and requested replacements", func(t *testing.T) {
		_, data := planData(t, `
  # aws_instance.tainted is tainted, so must be replaced
-/+ resource "aws_instance" "tainted" {}

  # aws_instance.requested will be replaced, as requested
-/+ resource "aws_instance" "requested" {}

  # aws_instance.forced must be replaced
-/+ resource "aws_instance" "forced" {}

Plan: 3 to add, 0 to change, 3 to destroy.
`)
		assert.ElementsMatch(t,
			[]string{"aws_instance.tainted", "aws_instance.requested", "aws_instance.forced"},
			data.ReplacedResources)
	})

	t.Run("deposed object is listed under its address", func(t *testing.T) {
		_, data := planData(t, `
  # aws_instance.x (deposed object 1234abcd) will be destroyed
  - resource "aws_instance" "x" {}

Plan: 0 to add, 0 to change, 1 to destroy.
`)
		assert.Equal(t, []string{"aws_instance.x"}, data.DeletedResources)
	})
}

func TestParsePlanOutput_NoChangesWording(t *testing.T) {
	t.Run("phrase inside an attribute value is not an empty plan", func(t *testing.T) {
		result, data := planData(t, `
  # aws_ssm_parameter.p will be created
  + resource "aws_ssm_parameter" "p" {
      + description = "No changes."
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`)
		assert.True(t, result.HasChanges)
		assert.Equal(t, plugin.ResourceCounts{Create: 1}, data.ResourceCounts)
		assert.Equal(t, []string{"aws_ssm_parameter.p"}, data.CreatedResources)
	})

	t.Run("infrastructure phrase inside an attribute value is not an empty plan", func(t *testing.T) {
		result, data := planData(t, `
  # aws_ssm_parameter.p will be created
  + resource "aws_ssm_parameter" "p" {
      + description = "Your infrastructure matches the configuration"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`)
		assert.True(t, result.HasChanges)
		assert.Equal(t, plugin.ResourceCounts{Create: 1}, data.ResourceCounts)
	})

	t.Run("real no-changes line still reads as empty", func(t *testing.T) {
		for _, output := range []string{
			"\nNo changes. Your infrastructure matches the configuration.\n",
			"\n\x1b[0m\x1b[1m\x1b[32mNo changes.\x1b[0m\x1b[1m Your infrastructure matches the configuration.\x1b[0m\n",
			"\nNo changes. Infrastructure is up-to-date.\n",
		} {
			result, _ := planData(t, output)
			assert.False(t, result.HasChanges, "output: %q", output)
		}
	})
}

func TestParseApplyOutput_ResourceAddressForms(t *testing.T) {
	t.Run("create_before_destroy replacement reconciles through the deposed object", func(t *testing.T) {
		data := applyData(t, `
aws_instance.x: Creating...
aws_instance.x: Creation complete after 1s [id=i-2]
aws_instance.x (deposed object 1234abcd): Destroying... [id=i-1]
aws_instance.x: Destruction complete after 1s

Apply complete! Resources: 1 added, 0 changed, 1 destroyed.
`)
		assert.Equal(t, []string{"aws_instance.x"}, data.ReplacedResources)
		assert.Empty(t, data.CreatedResources)
		assert.Empty(t, data.DeletedResources)
		assert.Equal(t, plugin.ResourceCounts{Replace: 1}, data.ResourceCounts)
	})

	t.Run("for_each key containing spaces", func(t *testing.T) {
		data := applyData(t, `
aws_instance.x["my key"]: Creating...

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
`)
		assert.Equal(t, []string{`aws_instance.x["my key"]`}, data.CreatedResources)
	})

	t.Run("imports are listed", func(t *testing.T) {
		data := applyData(t, `
aws_s3_bucket.b: Importing... [id=example-data]
aws_s3_bucket.b: Import complete [id=example-data]

Apply complete! Resources: 1 imported, 0 added, 0 changed, 0 destroyed.
`)
		assert.Equal(t, []string{"aws_s3_bucket.b"}, data.ImportedResources)
	})

	t.Run("import-only apply is a change", func(t *testing.T) {
		result := ParseApplyOutput("\nApply complete! Resources: 2 imported, 0 added, 0 changed, 0 destroyed.\n")
		assert.True(t, result.HasChanges)
		data, ok := result.Data.(*plugin.TerraformOutputData)
		require.True(t, ok)
		assert.Equal(t, plugin.ResourceCounts{}, data.ResourceCounts)
	})

	t.Run("failed import attempt alone is not a change", func(t *testing.T) {
		result := ParseApplyOutput(`
aws_s3_bucket.b: Importing... [id=example-data]

Apply complete! Resources: 0 imported, 0 added, 0 changed, 0 destroyed.
`)
		assert.False(t, result.HasChanges)
	})

	t.Run("provisioner output is not a resource action", func(t *testing.T) {
		data := applyData(t, `
null_resource.x: Creating...
null_resource.x (local-exec): Destroying...
null_resource.x: Creation complete after 0s

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
`)
		assert.Equal(t, []string{"null_resource.x"}, data.CreatedResources)
		assert.Empty(t, data.DeletedResources)
		assert.Empty(t, data.ReplacedResources)
	})
}

func TestParseApplyOutput_OutputForms(t *testing.T) {
	const header = "Apply complete! Resources: 0 added, 0 changed, 0 destroyed.\n\nOutputs:\n\n"

	t.Run("hyphenated output name", func(t *testing.T) {
		data := applyData(t, header+"first = \"1\"\nmy-output = \"v\"\nlast = \"3\"\n")
		require.Len(t, data.Outputs, 3)
		assert.Equal(t, "1", data.Outputs["first"].Value)
		assert.Equal(t, "v", data.Outputs["my-output"].Value)
		assert.Equal(t, "3", data.Outputs["last"].Value)
	})

	t.Run("bracket inside a string value", func(t *testing.T) {
		data := applyData(t, header+"a = \"x[y\"\nb = \"{\\\"z\"\nc = \"w\"\n")
		require.Len(t, data.Outputs, 3)
		assert.Equal(t, "x[y", data.Outputs["a"].Value)
		assert.Equal(t, `{\"z`, data.Outputs["b"].Value)
		assert.Equal(t, "w", data.Outputs["c"].Value)
	})

	t.Run("heredoc value", func(t *testing.T) {
		data := applyData(t, header+"a = <<EOT\nhello\n\nworld = 1\nEOT\nb = <<-EOT\n  indented\nEOT\nc = \"z\"\n")
		require.Len(t, data.Outputs, 3)
		assert.Equal(t, "hello\n\nworld = 1", data.Outputs["a"].Value)
		assert.Equal(t, "  indented", data.Outputs["b"].Value)
		assert.Equal(t, "z", data.Outputs["c"].Value)
	})

	t.Run("multi-line list still parses", func(t *testing.T) {
		data := applyData(t, header+"ids = [\n  \"a]\",\n  \"b\",\n]\nlast = \"3\"\n")
		require.Len(t, data.Outputs, 2)
		assert.Equal(t, "[\n\"a]\",\n\"b\",\n]", data.Outputs["ids"].Value)
		assert.Equal(t, "3", data.Outputs["last"].Value)
	})
}
