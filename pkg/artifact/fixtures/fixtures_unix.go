//go:build unix

package fixtures

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/cloudposse/atmos/pkg/perf"
)

// pipePerm is the mode of the named pipe fixture.
const pipePerm = 0o600

// SpecialFile creates a named pipe (a special file) at rel below dir and reports whether it could.
func SpecialFile(t testing.TB, dir, rel string) bool {
	defer perf.Track(nil, "fixtures.SpecialFile")()

	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := syscall.Mkfifo(path, pipePerm); err != nil {
		t.Logf("named pipes are unavailable here: %v", err)
		return false
	}
	return true
}
