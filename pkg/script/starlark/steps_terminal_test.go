package starlark

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/automation"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestTestStepRetainsTerminalOwnership(t *testing.T) {
	for _, tc := range []struct {
		name         string
		kind         string
		stderr       io.Writer
		parallel     bool
		hostOwns     bool
		ownsTerminal bool
	}{
		{name: "standalone test", kind: "test", stderr: iolib.GetContext().UI(), ownsTerminal: true},
		{name: "captured test", kind: "test", stderr: &bytes.Buffer{}},
		{name: "wrapped host terminal", kind: "test", stderr: &bytes.Buffer{}, hostOwns: true, ownsTerminal: true},
		{name: "parallel overrides host ownership", kind: "test", stderr: &bytes.Buffer{}, hostOwns: true, parallel: true},
		{name: "parallel test", kind: "test", stderr: iolib.GetContext().UI(), parallel: true},
		{name: "ordinary step", kind: "shell", stderr: iolib.GetContext().UI()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			library := NewMockStepLibrary(gomock.NewController(t))
			library.EXPECT().Names().Return([]string{tc.kind})
			library.EXPECT().Fork().Return(library)
			library.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, call *automation.StepCall) (*automation.StepResult, error) {
				if tc.ownsTerminal {
					assert.Nil(t, call.Stderr, "let the test renderer detect its owned terminal")
				} else {
					assert.NotNil(t, call.Stderr, "captured and parallel output must stay scoped")
				}
				return &automation.StepResult{}, nil
			})
			_, err := New().Execute(t.Context(), script.Spec{
				Steps: library, Stderr: tc.stderr, Parallel: tc.parallel, OwnsTerminal: tc.hostOwns,
				Source: `steps.run("` + tc.kind + `")`,
			})
			require.NoError(t, err)
		})
	}
}
