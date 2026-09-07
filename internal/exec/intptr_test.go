package exec

// intPtr returns a pointer to the given int. Shared test helper used across
// several exec test files (previously defined in workflow_adapters_test.go).
func intPtr(i int) *int {
	return &i
}
