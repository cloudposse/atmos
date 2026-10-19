package utils_test

// Loading admission installs the policy engine behind the guard hooks. The utils package cannot import admission
// itself, which would form an import cycle through CI reporting, so its tests load it from this external
// test package. The hooks are process-wide, so the package's internal tests exercise real policies too.
import _ "github.com/cloudposse/atmos/pkg/policy/admission"
