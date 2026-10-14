package schema

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// ErrInvalidPlanDiffIgnoreChange indicates an invalid resource-type-qualified attribute.
var ErrInvalidPlanDiffIgnoreChange = errors.New("invalid plan-diff ignore_changes entry")

// TerraformPlanDiff configures Atmos plan comparisons, not Terraform lifecycle behavior.
type TerraformPlanDiff struct {
	// IgnoreChanges excludes exact resource_type.attribute names from Atmos comparisons.
	// Only top-level attributes are supported; globs and nested paths are invalid.
	IgnoreChanges []string `yaml:"ignore_changes,omitempty" json:"ignore_changes,omitempty" mapstructure:"ignore_changes"`
}

// Validate rejects malformed exclusions before Terraform executes.
func (p TerraformPlanDiff) Validate() error {
	for _, entry := range p.IgnoreChanges {
		resourceType, attribute, found := strings.Cut(entry, ".")
		if !found || !hclsyntax.ValidIdentifier(resourceType) || !hclsyntax.ValidIdentifier(attribute) {
			return fmt.Errorf("%w: %q in components.terraform.plan_diff.ignore_changes; expected resource_type.attribute (no globs or nested paths)",
				ErrInvalidPlanDiffIgnoreChange, entry)
		}
	}
	return nil
}
