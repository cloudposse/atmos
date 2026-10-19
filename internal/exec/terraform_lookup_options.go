package exec

import "github.com/cloudposse/atmos/pkg/schema"

// TerraformLookupOptions preserves the enclosing secret-resolution mode across a
// nested state/output lookup. The zero value resolves real secrets for execution.
type TerraformLookupOptions struct {
	SecretsMaskOnly bool
}

// terraformLookupOptions forwards inspection mode without changing ordinary
// execution calls. Lookup methods accept at most one optional options value.
func terraformLookupOptions(info *schema.ConfigAndStacksInfo) []TerraformLookupOptions {
	if info != nil && info.SecretsMaskOnly {
		return []TerraformLookupOptions{{SecretsMaskOnly: true}}
	}
	return nil
}

// lookupSecretsMaskOnly reports whether a lookup was requested for inspection.
func lookupSecretsMaskOnly(options []TerraformLookupOptions) bool {
	return len(options) > 0 && options[0].SecretsMaskOnly
}
