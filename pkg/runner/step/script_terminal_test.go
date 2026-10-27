package step

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestScriptHandlerPreservesTerminalOwnership(t *testing.T) {
	initShellTestIO(t)
	engine := &ciRecordingEngine{}
	script.Register("recording-terminal", engine)
	for _, tc := range []struct {
		name, mode string
		stderr     io.Writer
		parallel   bool
		want       bool
	}{
		{name: "command", want: true},
		{name: "hook", stderr: os.Stderr, want: true},
		{name: "captured", stderr: &bytes.Buffer{}},
		{name: "hidden", mode: "none"},
		{name: "logged", mode: "log"},
		{name: "viewport", mode: "viewport"},
		{name: "parallel", parallel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := NewVariables()
			vars.OutputWriters.Stderr = tc.stderr
			vars.automationParallel = tc.parallel
			_, err := (&ScriptHandler{}).Execute(t.Context(), &schema.WorkflowStep{
				Type: "script", Interpreter: "recording-terminal", Script: "ignored", Output: tc.mode,
			}, vars)
			require.NoError(t, err)
			require.Equal(t, tc.want, engine.specs[len(engine.specs)-1].OwnsTerminal)
		})
	}
}

var _ = script.Spec{OwnsTerminal: false}
