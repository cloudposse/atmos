package terraform

// Blank imports register the built-in hook kinds via each package's init().
// This file is the wiring point because cmd/terraform sits above pkg/hooks
// and pkg/hooks/kinds/* in the dependency graph, avoiding the import cycle
// that would result from registering kinds inside pkg/hooks itself.
import (
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/checkov"   // kind: checkov.
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/infracost" // kind: infracost.
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/kics"      // kind: kics.
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/trivy"     // kind: trivy.
)
