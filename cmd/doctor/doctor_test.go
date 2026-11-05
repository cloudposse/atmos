package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/validation"
)

func TestDoctorMachineReports(t *testing.T) {
	t.Cleanup(ui.Reset)
	for _, format := range []string{"json", "sarif"} {
		t.Run(format, func(t *testing.T) {
			var output, human bytes.Buffer
			ctx, err := iolib.NewContext(iolib.WithStreams(&testStreams{out: &output, err: &human}))
			require.NoError(t, err)
			ui.InitFormatter(ctx)
			data.InitWriter(ctx)
			r := diagnostic.Report{Version: 1, OperationalError: "injected failure", Backup: "/backup", Findings: []diagnostic.Finding{{RuleID: "test", Severity: validation.SeverityWarning, Message: "manual repair", Status: "manual"}}}
			require.NoError(t, render(&r, format, renderOptions{}))
			var doc map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &doc))
			require.Empty(t, human.String())
			require.Contains(t, output.String(), "injected failure")
			require.Contains(t, output.String(), "/backup")
		})
	}
}

type testStreams struct{ out, err io.Writer }

func (s *testStreams) Input() io.Reader     { return bytes.NewReader(nil) }
func (s *testStreams) Output() io.Writer    { return s.out }
func (s *testStreams) Error() io.Writer     { return s.err }
func (s *testStreams) RawOutput() io.Writer { return s.out }
func (s *testStreams) RawError() io.Writer  { return s.err }

func TestDoctorInvocationErrors(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"--format=xml"}, {"--fail-on=error|warning"}, {"extra"}, {"--base=main"}} {
		t.Run(args[0], func(t *testing.T) {
			cmd := NewCommand()
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(context.Background())
			require.Error(t, err)
			require.Equal(t, 2, errUtils.GetExitCode(err))
		})
	}
}
