package reexec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExec_VarIsSet verifies that the platform default Exec var is bound
// on every supported platform. The actual process-replacing default is
// not exercised here (doing so would kill the test process on Unix and
// call os.Exit on Windows); instead we just assert the default is non-nil.
func TestExec_VarIsSet(t *testing.T) {
	assert.NotNil(t, Exec, "pkg/reexec.Exec must have a platform default")
}

// TestExec_InjectableForTests verifies the package var can be swapped out
// by tests, which is how every call-site exercises its re-exec logic.
func TestExec_InjectableForTests(t *testing.T) {
	original := Exec
	defer func() { Exec = original }()

	var (
		gotArgv0 string
		gotArgv  []string
		gotEnv   []string
	)
	Exec = func(argv0 string, argv []string, envv []string) error {
		gotArgv0 = argv0
		gotArgv = argv
		gotEnv = envv
		return nil
	}

	err := Exec("/bin/atmos", []string{"atmos", "version"}, []string{"FOO=bar"})

	assert.NoError(t, err)
	assert.Equal(t, "/bin/atmos", gotArgv0)
	assert.Equal(t, []string{"atmos", "version"}, gotArgv)
	assert.Equal(t, []string{"FOO=bar"}, gotEnv)
}
