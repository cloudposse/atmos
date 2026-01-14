package renderer

// mockBadFilter is a filter that returns an invalid type for testing error handling.
type mockBadFilter struct{}

// Apply returns an invalid type to trigger error path in Render.
func (f *mockBadFilter) Apply(data interface{}) (interface{}, error) {
	// Return wrong type - string instead of []map[string]any.
	return "invalid type", nil
}
