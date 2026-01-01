package types

// CredentialField describes a single credential input field.
type CredentialField struct {
	Name        string             // Field identifier (e.g., "access_key_id").
	Title       string             // Display title (e.g., "AWS Access Key ID").
	Description string             // Help text.
	Required    bool               // Must be non-empty.
	Secret      bool               // Mask input (password mode).
	Default     string             // Pre-populated value.
	Validator   func(string) error // Optional validation function.
}

// CredentialPromptSpec defines what credentials to prompt for.
type CredentialPromptSpec struct {
	IdentityName string            // Name of the identity requiring credentials.
	CloudType    string            // Cloud provider: "aws", "azure", "gcp".
	Fields       []CredentialField // Fields to prompt for.
}

// CredentialPromptFunc is the generic prompting interface.
// It takes a specification of what credentials to collect and returns them as a map.
// Each identity type can define its own fields, and the prompting UI is generic.
type CredentialPromptFunc func(spec CredentialPromptSpec) (map[string]string, error)

// CredentialPromptResult holds the result of a credential prompt operation.
type CredentialPromptResult struct {
	Values map[string]string // Field name -> value.
}

// Get returns the value for a field, or empty string if not found.
func (r *CredentialPromptResult) Get(name string) string {
	if r.Values == nil {
		return ""
	}
	return r.Values[name]
}
