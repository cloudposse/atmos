package schema

// VendorLockConfig configures how `atmos vendor pull` reacts when a package's on-disk state no
// longer matches its vendor.lock.yaml receipt.
type VendorLockConfig struct {
	// Enforcement is "warn" (the default when omitted), "silent", or "strict".
	//   - "silent": re-fetch the drifted package with no reporting.
	//   - "warn": re-fetch the drifted package and print one warning per package naming why it
	//     drifted.
	//   - "strict": refuse to run (before any fetch/copy/write) when a drifted package is found and
	//     `--refresh-lock` was not explicitly passed, naming every drifted package and its reason.
	Enforcement string `yaml:"enforcement,omitempty" json:"enforcement,omitempty" mapstructure:"enforcement" validate:"omitempty,oneof=strict warn silent"`
}
