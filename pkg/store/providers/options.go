package providers

// firstNonEmptyStringPtr returns the dereferenced value of the first non-nil,
// non-empty string pointer, or "" when none qualify. It is used to resolve
// endpoint-style fallbacks across store options.
func firstNonEmptyStringPtr(values ...*string) string {
	for _, value := range values {
		if value != nil && *value != "" {
			return *value
		}
	}
	return ""
}
