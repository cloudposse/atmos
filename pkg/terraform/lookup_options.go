package terraform

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LookupOptions preserves evaluation settings across nested state/output lookups.
// The zero value resolves real secrets and evaluates all YAML functions.
type LookupOptions struct {
	SecretsMaskOnly bool
	Skip            []string
}

// LookupOptionsFromInfo forwards the enclosing evaluation's exclusions and inspection mode.
func LookupOptionsFromInfo(info *schema.ConfigAndStacksInfo) []LookupOptions {
	defer perf.Track(nil, "terraform.LookupOptionsFromInfo")()

	if info != nil && (info.SecretsMaskOnly || len(info.Skip) > 0) {
		return []LookupOptions{{SecretsMaskOnly: info.SecretsMaskOnly, Skip: info.Skip}}
	}
	return nil
}

// ResolveLookupOptions returns the first optional settings value or execution defaults.
func ResolveLookupOptions(options []LookupOptions) LookupOptions {
	defer perf.Track(nil, "terraform.ResolveLookupOptions")()

	if len(options) == 0 {
		return LookupOptions{}
	}
	return options[0]
}
