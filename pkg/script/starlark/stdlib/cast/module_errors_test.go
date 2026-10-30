package cast

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/castplan"
)

func evalCastWithHost(source string, host Host) error {
	_, err := starlark.ExecFileOptions(&syntax.FileOptions{TopLevelControl: true}, &starlark.Thread{Name: "test"}, "test.star", source, starlark.StringDict{"cast": New(host)})
	return err
}

func TestRecorderRejectsUnencodableAndMisplacedFields(t *testing.T) {
	tests := []struct{ name, source, want string }{
		{
			name:   "recorder option that cannot be encoded",
			source: `cast.recorder(output="x", extra=lambda: None)`,
			want:   "cannot encode",
		},
		{
			name:   "action field that cannot be encoded",
			source: `rec=cast.recorder(output="x"); rec.run("x", extra=lambda: None)`,
			want:   "cannot encode",
		},
		{
			name:   "policy on an action",
			source: `rec=cast.recorder(output="x"); rec.run("x", policy=lambda text: None)`,
			want:   "policy belongs on cast.recorder",
		},
		{
			name:   "policy on a terminal action",
			source: `rec=cast.recorder(output="x"); rec.terminal.write("x", policy=lambda text: None)`,
			want:   "policy belongs on cast.recorder",
		},
		{
			name:   "assertions on a terminal action",
			source: `rec=cast.recorder(output="x"); rec.terminal.write("x", expect="y")`,
			want:   "belong on rec.run",
		},
		{
			name:   "terminal action with an invalid exit code",
			source: `rec=cast.recorder(output="x"); rec.terminal.key("enter", exit_code=999)`,
			want:   "exit_code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, evalCast(tt.source, &testHost{}), tt.want)
		})
	}
}

func TestRecorderFinishRequiresHost(t *testing.T) {
	for _, call := range []string{"generate", "validate"} {
		t.Run(call, func(t *testing.T) {
			err := evalCastWithHost(`rec=cast.recorder(output="x"); rec.prompt(); rec.`+call+`()`, nil)
			require.ErrorContains(t, err, "host has not provided cast services")
		})
	}
}

// failingHost rejects the declaration structurally and records whether it was asked to record.
type failingHost struct {
	testHost
	checkErr error
}

func (h *failingHost) Check(_ *starlark.Thread, _ *castplan.Plan) error { return h.checkErr }

func TestRecorderStructuralFailureSkipsRecording(t *testing.T) {
	errStructure := errors.New("structure rejected")
	host := &failingHost{checkErr: errStructure}
	err := evalCastWithHost(`rec=cast.recorder(output="x"); rec.prompt(); rec.generate()`, host)
	require.ErrorIs(t, err, errStructure)
	require.False(t, host.generated, "a declaration that fails the structural check must never be recorded")
	require.False(t, host.validated)
}

func TestTerminalActionsRejectFrozenRecorder(t *testing.T) {
	rec := &recorder{plan: &castplan.Plan{}}
	value, err := rec.Attr("terminal")
	require.NoError(t, err)
	terminal := value.(*terminal)
	send, err := terminal.Attr("send")
	require.NoError(t, err)
	terminal.Freeze()
	_, err = starlark.Call(&starlark.Thread{Name: "test"}, send, starlark.Tuple{starlark.String("echo hi")}, nil)
	require.ErrorContains(t, err, "frozen")
	require.Empty(t, rec.plan.Actions, "a frozen recorder must not accept new actions")
}
