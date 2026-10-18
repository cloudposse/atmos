//go:build !darwin && !windows && !linux && !freebsd

package toolchain

// toolVersionsPreserveCase conservatively folds lock keys on platforms without
// a native filesystem case-policy query. Access errors still prevent locking.
func toolVersionsPreserveCase(path string) (bool, error) {
	return toolVersionsFallbackCasePolicy(path)
}
