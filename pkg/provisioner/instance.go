package provisioner

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// instanceKeySeparator joins stack and component in the per-instance lock filename.
const instanceKeySeparator = "-"

// InstanceKey derives a filesystem-safe identifier for a (stack, component) instance
// from its component config, matching the <stack>-<component> scheme used elsewhere
// (e.g. workdir.BuildPath). It is the disambiguator for per-instance lock files so that
// divergent stacks of one Terraform root module never collide.
func InstanceKey(componentConfig map[string]any) string {
	defer perf.Track(nil, "provisioner.InstanceKey")()

	stack, _ := componentConfig["atmos_stack"].(string)
	component, _ := componentConfig["atmos_component"].(string)
	if component == "" {
		component, _ = componentConfig["component"].(string)
	}
	return sanitizeInstanceKey(stack + instanceKeySeparator + component)
}

// InstanceLockFilename returns the per-instance lock filename for an instance, e.g.
// ".dev-vpc.terraform.lock.hcl". The leading dot keeps it hidden alongside the canonical
// .terraform.lock.hcl; for ephemeral/vendored components the canonical file is treated as
// scratch while this per-instance file is the committed source of truth.
func InstanceLockFilename(componentConfig map[string]any) string {
	defer perf.Track(nil, "provisioner.InstanceLockFilename")()

	return "." + InstanceKey(componentConfig) + ".terraform.lock.hcl"
}

// sanitizeInstanceKey replaces path separators and whitespace so the key is safe as a
// single filename component (stack names may contain '/').
func sanitizeInstanceKey(s string) string {
	replacer := strings.NewReplacer(
		"/", instanceKeySeparator,
		"\\", instanceKeySeparator,
		string(filepath.Separator), instanceKeySeparator,
		" ", instanceKeySeparator,
	)
	return strings.Trim(replacer.Replace(s), instanceKeySeparator)
}
