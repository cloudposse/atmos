package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/reexec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A standalone script run rewrites os.Args to hide the script from Atmos's own parsing. The
// version re-exec must still hand the new process the script and its arguments.
func TestDefaultReexecConfigUsesTheOriginalCommandLine(t *testing.T) {
	original := []string{"atmos", "--no-color", "./tool.star", "hello", "--chdir=script-owned"}
	defer reexec.SetOriginalArgs(original, 3)()

	config := DefaultReexecConfig()
	assert.Equal(t, original, config.Args)
	assert.Equal(t, 3, config.ScriptArgs)
}

func TestReexecForwardsScriptPathAndArguments(t *testing.T) {
	args := []string{"atmos", "--use-version", "1.160.0", "--chdir=project", "./tool.star", "hello", "--chdir=script-owned", "--use-version=script-owned", "-Cscript"}
	var forwarded []string
	cfg := &ReexecConfig{
		Finder: &mockVersionFinder{findBinaryPathFunc: func(_, _, _ string) (string, error) {
			return "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", nil
		}},
		Installer: &mockVersionInstaller{},
		ExecFn: func(_ string, argv []string, _ []string) error {
			forwarded = argv
			return nil
		},
		GetEnv:     func(string) string { return "" },
		SetEnv:     func(string, string) error { return nil },
		Args:       args,
		ScriptArgs: 5,
		Environ:    func() []string { return nil },
	}

	require.True(t, CheckAndReexecWithConfig(&schema.AtmosConfiguration{Version: schema.Version{Use: "1.160.0"}}, cfg))
	assert.Equal(t,
		[]string{"atmos", "./tool.star", "hello", "--chdir=script-owned", "--use-version=script-owned", "-Cscript"},
		forwarded,
		"Atmos's own --use-version and --chdir are consumed; the script's identical-looking flags are forwarded")
}

func TestReexecWithoutScriptStripsEverywhere(t *testing.T) {
	var forwarded []string
	cfg := &ReexecConfig{
		Finder: &mockVersionFinder{findBinaryPathFunc: func(_, _, _ string) (string, error) {
			return "/home/user/.atmos/bin/cloudposse/atmos/1.160.0/atmos", nil
		}},
		Installer: &mockVersionInstaller{},
		ExecFn: func(_ string, argv []string, _ []string) error {
			forwarded = argv
			return nil
		},
		GetEnv:  func(string) string { return "" },
		SetEnv:  func(string, string) error { return nil },
		Args:    []string{"atmos", "terraform", "plan", "--chdir=x", "--use-version=1.160.0"},
		Environ: func() []string { return nil },
	}
	require.True(t, CheckAndReexecWithConfig(&schema.AtmosConfiguration{Version: schema.Version{Use: "1.160.0"}}, cfg))
	assert.Equal(t, []string{"atmos", "terraform", "plan"}, forwarded)
}
