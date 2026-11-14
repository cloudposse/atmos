//go:build !unix

package fixtures

import "testing"

// SpecialFile reports false: special files cannot be created portably on this platform.
func SpecialFile(t testing.TB, _, _ string) bool {
	t.Helper()
	return false
}
