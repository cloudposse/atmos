package exec

import "github.com/cloudposse/atmos/pkg/terraform"

// TerraformLookupOptions is retained for compatibility with existing reference getters.
// Evaluation settings are owned by pkg/terraform.
type TerraformLookupOptions = terraform.LookupOptions
