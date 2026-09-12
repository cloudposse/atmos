package step

import (
	"fmt"
	"io"
	"maps"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Clone isolates mutable execution state for a concurrent branch.
// Published StepResults are immutable; each branch owns its maps and later results.
func (v *Variables) Clone() *Variables {
	defer perf.Track(nil, "step.Variables.Clone")()

	c := *v
	c.Steps = maps.Clone(v.Steps)
	c.Env = maps.Clone(v.Env)
	c.templateEnv = maps.Clone(v.templateEnv)
	c.Flags = maps.Clone(v.Flags)
	c.templateRoots = maps.Clone(v.templateRoots)
	c.protectedRoots = maps.Clone(v.protectedRoots)
	return &c
}

// UI returns a writer scoped to this execution, falling back to the normal UI.
func (v *Variables) UI() *scopedUI {
	defer perf.Track(nil, "step.Variables.UI")()
	return &scopedUI{writer: v.OutputWriters.Stderr}
}

type scopedUI struct{ writer io.Writer }

// Write sends masked text to the execution writer or the default UI.
func (s *scopedUI) Write(content string) {
	defer perf.Track(nil, "step.scopedUI.Write")()

	if s.writer != nil {
		_, _ = io.WriteString(s.writer, iolib.MaskString(content))
		return
	}
	ui.Write(content)
}

// Writeln appends a newline to execution-scoped UI text.
func (s *scopedUI) Writeln(content string) {
	defer perf.Track(nil, "step.scopedUI.Writeln")()
	s.Write(content + "\n")
}

// Writef formats a message before masking and routing it to the execution writer.
func (s *scopedUI) Writef(format string, args ...any) {
	defer perf.Track(nil, "step.scopedUI.Writef")()
	s.Write(fmt.Sprintf(format, args...))
}

// Info preserves informational styling outside captures and plain text inside them.
func (s *scopedUI) Info(content string) {
	defer perf.Track(nil, "step.scopedUI.Info")()

	if s.writer == nil {
		ui.Info(content)
	} else {
		s.Writeln(content)
	}
}

// Infof formats an informational message for the scoped UI.
func (s *scopedUI) Infof(format string, args ...any) {
	defer perf.Track(nil, "step.scopedUI.Infof")()
	s.Info(fmt.Sprintf(format, args...))
}

// Success emits a success message through the execution writer or default UI.
func (s *scopedUI) Success(content string) {
	defer perf.Track(nil, "step.scopedUI.Success")()

	if s.writer == nil {
		ui.Success(content)
	} else {
		s.Writeln(content)
	}
}

// Warning emits a warning through the execution writer or default UI.
func (s *scopedUI) Warning(content string) {
	defer perf.Track(nil, "step.scopedUI.Warning")()

	if s.writer == nil {
		ui.Warning(content)
	} else {
		s.Writeln(content)
	}
}

// Error emits an error message through the execution writer or default UI.
func (s *scopedUI) Error(content string) {
	defer perf.Track(nil, "step.scopedUI.Error")()

	if s.writer == nil {
		ui.Error(content)
	} else {
		s.Writeln(content)
	}
}

// Hint keeps captured guidance in the current test output buffer.
func (s *scopedUI) Hint(content string) {
	defer perf.Track(nil, "step.scopedUI.Hint")()

	if s.writer == nil {
		ui.Hint(content)
	} else {
		s.Writeln(content)
	}
}

// Markdown retains source text during capture and renders it in the default UI otherwise.
func (s *scopedUI) Markdown(content string) {
	defer perf.Track(nil, "step.scopedUI.Markdown")()

	if s.writer == nil {
		ui.Markdown(content)
	} else {
		s.Writeln(content)
	}
}

// MarkdownMessage routes a Markdown message through the current execution.
func (s *scopedUI) MarkdownMessage(content string) {
	defer perf.Track(nil, "step.scopedUI.MarkdownMessage")()

	if s.writer == nil {
		ui.MarkdownMessage(content)
	} else {
		s.Writeln(content)
	}
}

// WriteDataLine writes captured data without changing process-global streams.
func (v *Variables) WriteDataLine(content string) error {
	defer perf.Track(nil, "step.Variables.WriteDataLine")()

	if v.OutputWriters.Stdout == nil {
		return data.Writeln(content)
	}
	_, err := io.WriteString(v.OutputWriters.Stdout, iolib.MaskString(content)+"\n")
	return err
}
