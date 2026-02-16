// Package terraform provides the CI provider implementation for Terraform.
package terraform

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformTemplateContext extends the base TemplateContext with terraform-specific fields.
// Templates access fields directly (e.g., {{ .Resources.Create }}) instead of through Result.Data.
type TerraformTemplateContext struct {
	*plugin.TemplateContext

	// Resources contains resource change counts.
	Resources plugin.ResourceCounts

	// CreatedResources contains addresses of resources to be created.
	CreatedResources []string

	// UpdatedResources contains addresses of resources to be updated.
	UpdatedResources []string

	// ReplacedResources contains addresses of resources to be replaced.
	ReplacedResources []string

	// DeletedResources contains addresses of resources to be destroyed.
	DeletedResources []string

	// MovedResources contains resources that have been moved.
	MovedResources []plugin.MovedResource

	// ImportedResources contains addresses of resources to be imported.
	ImportedResources []string

	// Outputs contains terraform output values (after apply).
	Outputs map[string]plugin.TerraformOutput

	// ChangedResult contains the plan summary text.
	ChangedResult string

	// HasDestroy indicates if there are resources to be destroyed.
	HasDestroy bool
}

// NewTemplateContext creates a TerraformTemplateContext from a base context and parsed output.
func NewTemplateContext(base *plugin.TemplateContext, data *plugin.TerraformOutputData) *TerraformTemplateContext {
	defer perf.Track(nil, "terraform.NewTemplateContext")()

	ctx := &TerraformTemplateContext{
		TemplateContext: base,
	}

	if data != nil {
		ctx.Resources = data.ResourceCounts
		ctx.CreatedResources = data.CreatedResources
		ctx.UpdatedResources = data.UpdatedResources
		ctx.ReplacedResources = data.ReplacedResources
		ctx.DeletedResources = data.DeletedResources
		ctx.MovedResources = data.MovedResources
		ctx.ImportedResources = data.ImportedResources
		ctx.Outputs = data.Outputs
		ctx.ChangedResult = data.ChangedResult
		ctx.HasDestroy = data.ResourceCounts.Destroy > 0
	}

	return ctx
}

// Target returns the stack-component slug for anchor IDs.
func (c *TerraformTemplateContext) Target() string {
	defer perf.Track(nil, "terraform.TerraformTemplateContext.Target")()

	if c.TemplateContext == nil {
		return ""
	}
	return c.Stack + "-" + c.Component
}

// HasChanges returns true if there are any resource changes.
func (c *TerraformTemplateContext) HasChanges() bool {
	defer perf.Track(nil, "terraform.TerraformTemplateContext.HasChanges")()

	return c.Resources.Create > 0 ||
		c.Resources.Change > 0 ||
		c.Resources.Replace > 0 ||
		c.Resources.Destroy > 0
}

// TotalChanges returns the total number of resource changes.
func (c *TerraformTemplateContext) TotalChanges() int {
	defer perf.Track(nil, "terraform.TerraformTemplateContext.TotalChanges")()

	return c.Resources.Create + c.Resources.Change + c.Resources.Replace + c.Resources.Destroy
}
