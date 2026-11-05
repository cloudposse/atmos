package cast

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecorderCdTracksWorkingDirectoryForLaterActions(t *testing.T) {
	base := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "elsewhere", "..", "target")
	host := &testHost{}
	require.NoError(t, evalCast(fmt.Sprintf(`rec=cast.recorder(output="x", working_directory=%q)
rec.run("first")
rec.cd("sub")
rec.run("second")
rec.cd(%q)
rec.run("third")
rec.cd("child")
rec.run("fourth")
rec.validate()`, base, absolute), host))

	var directories []any
	var commands []any
	for _, action := range host.plan.Actions {
		if action.Kind == "run" {
			commands = append(commands, action.Fields["command"])
			directories = append(directories, action.Fields["working_directory"])
		}
	}
	cleaned := filepath.Clean(absolute)
	require.Equal(t, []any{"first", "second", "third", "fourth"}, commands)
	require.Equal(t, []any{
		base,
		filepath.Join(base, "sub"),
		cleaned,
		filepath.Join(cleaned, "child"),
	}, directories)
}
