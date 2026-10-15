package plandiff

import (
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Policy resolves configuration before executing Terraform. Edition pins
// only select display behavior; explicit exclusions apply under every edition.
func Policy(config *schema.AtmosConfiguration) (Option, error) {
	defer perf.Track(config, "plandiff.Policy")()

	if err := config.Components.Terraform.PlanDiff.Validate(); err != nil {
		return nil, err
	}
	show, err := edition.BehaviorChanged(config.Edition, "terraform.plan_diff.show_computed_attributes")
	if err != nil {
		return nil, err
	}
	ignore := WithIgnoreChanges(config.Components.Terraform.PlanDiff.IgnoreChanges)
	legacy := WithLegacyDisplay(!show)
	return func(o *options) {
		ignore(o)
		legacy(o)
	}, nil
}
