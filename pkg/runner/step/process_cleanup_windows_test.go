//go:build windows

package step

// stepProcessGone is never consulted on Windows: the tests that use it skip there.
func stepProcessGone(int) bool { return true }

// stepProcessGroupsSupported is false on Windows, which terminates only the direct child.
const stepProcessGroupsSupported = false
