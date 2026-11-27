package terraform

import (
	"github.com/cloudposse/atmos/pkg/perf"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
	terraformadoption "github.com/cloudposse/atmos/pkg/terraform/adoption"
)

// AdoptionAdapters exposes the Terraform-family sources through the component registry.
func (*Provider) AdoptionAdapters() stackadoption.Adapters {
	defer perf.Track(nil, "terraform.Provider.AdoptionAdapters")()
	return terraformadoption.Adapters()
}
