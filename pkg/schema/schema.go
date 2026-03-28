// RequiredProvider defines the required providers for the module.
type RequiredProvider struct {
	Source  string `json:"source"`
	Version string `json:"version"`
}

// Terraform represents the Terraform module configuration.
type Terraform struct {
	// existing fields...
	CIDefinition CIDefinition `json:"ci"`
	RequiredVersion string `json:"required_version"` // new field
	RequiredProviders map[string]RequiredProvider `json:"required_providers"` // new field
}