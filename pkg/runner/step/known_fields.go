package step

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// This file declares, for every built-in step handler, the step fields it reads beyond the common
// step fields (see automationCommonFields). A direct step call (`steps.<type>(...)`) is validated
// against those fields, so a field that means something else for another type (for example `url`
// on `shell` or `command` on `join`) is rejected instead of being silently ignored.
//
// The names are YAML keys derived from the struct tags of schema.WorkflowStep, so a renamed key
// follows the schema automatically and a removed Go field fails at package initialization.

// stepYAMLFields returns the YAML keys of the named schema.WorkflowStep Go fields. It panics on a
// Go field that does not exist or has no YAML key, which is a programming error caught by every
// test run.
func stepYAMLFields(goFields ...string) []string {
	kind := reflect.TypeFor[schema.WorkflowStep]()
	keys := make([]string, 0, len(goFields))
	for _, goField := range goFields {
		field, ok := kind.FieldByName(goField)
		if !ok {
			panic(fmt.Sprintf("step: schema.WorkflowStep has no field %q", goField))
		}
		key := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if key == "" || key == "-" {
			panic(fmt.Sprintf("step: schema.WorkflowStep field %q has no YAML key", goField))
		}
		keys = append(keys, key)
	}
	return keys
}

// contentField is the Go name of the step field that holds the text most output steps print.
const contentField = "Content"

// Field sets shared by several handlers or too long to repeat.
var (
	// The fields of the `parallel` and `matrix` control steps.
	controlStepFields = stepYAMLFields("Steps", "MaxConcurrency", "Fail")

	// The fields of the `style` step. The YAML key `background` carries the
	// background color, so it is listed explicitly: the Go field has no YAML tag of its own.
	styleStepFields = append(stepYAMLFields(
		contentField, "Foreground", "Border", "BorderForeground", "BorderBackground", "Padding", "Margin",
		"Width", "Height", "Align", "Bold", "Italic", "Underline", "Strikethrough", "Faint", "Markdown",
	), "background")

	// The fields of the `cast` step and of its session and simulation actions.
	castStepFields = stepYAMLFields(
		"Mode", "Shell", "WriteRate", "KeyInterval", "Jitter", "Cursor", "Text", "Regex", "Key",
		"Duration", "Interval", "Repeat", "Defaults", "Steps", "Title", "Width", "Height", "Rate",
		"Command", contentField,
	)

	// The cross-cutting container fields. The action's own parameters
	// (image, command, ports, and so on) live under `with`, which every step type accepts.
	containerStepFields = stepYAMLFields("Action", "Provider", "Runtime", "RuntimeAutoStart", "Interactive", "Tty", "Stack")
)

// knownFields returns a copy of fields so a caller cannot change a handler's declaration.
func knownFields(fields []string) []string {
	return slices.Clone(fields)
}

var (
	alertFields    = stepYAMLFields(contentField)
	archiveFields  = stepYAMLFields("Action", "Source", "Destination", "Format", "Subpath", "Include", "Exclude", "Mtime")
	atmosFields    = stepYAMLFields("Command", "Stack", "Viewport")
	chooseFields   = stepYAMLFields("Prompt", "Options", "Default")
	confirmFields  = stepYAMLFields("Prompt", "Default")
	emulatorFields = stepYAMLFields("Action", "Component", "Stack", "Ephemeral")
	envFields      = stepYAMLFields("Vars", "Export")
	execFields     = stepYAMLFields("Command")
	exitFields     = stepYAMLFields("Code", contentField)
	fileFields     = stepYAMLFields("Prompt", "Default", "Path", "Extensions")
	filterFields   = stepYAMLFields("Prompt", "Options", "Default", "Multiple", "Limit")
	httpFields     = stepYAMLFields("URL", "Method", "Headers", "Query", "Body", "Form", "Expect")
	inputFields    = stepYAMLFields("Prompt", "Default", "Placeholder", "Password")
	joinFields     = stepYAMLFields(contentField, "Options", "Separator")
	junitFields    = stepYAMLFields("Action", "Files", "Title")
	logFields      = stepYAMLFields(contentField, "Level", "Fields")
	pagerFields    = stepYAMLFields(contentField, "Markdown", "Path", "Title")
	requireFields  = stepYAMLFields("Tools", "Files", "Dirs", "Hint")
	sayFields      = stepYAMLFields(contentField, "Voice", "Rate", "Print")
	scriptFields   = stepYAMLFields("Script", "Interpreter", "Viewport")
	shellFields    = stepYAMLFields("Command", "Interactive", "Tty", "Viewport")
	spinFields     = stepYAMLFields("Command", "Title")
	storeFields    = stepYAMLFields("Action", "Component", "Stack")
	tableFields    = stepYAMLFields(contentField, "Data", "Columns", "Title")
	tflintFields   = stepYAMLFields("Args", "Component", "Stack")
	toastFields    = stepYAMLFields(contentField, "Level")
	workdirFields  = stepYAMLFields("Path", "Source", "Reset")
	writeFields    = stepYAMLFields("Prompt", "Default", "Placeholder", "Height")
)

// KnownFields lists the fields the alert step reads.
func (h *AlertHandler) KnownFields() []string {
	defer perf.Track(nil, "step.AlertHandler.KnownFields")()

	return knownFields(alertFields)
}

// KnownFields lists the fields the archive step reads.
func (h *ArchiveHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ArchiveHandler.KnownFields")()

	return knownFields(archiveFields)
}

// KnownFields lists the fields the atmos step reads.
func (h *AtmosHandler) KnownFields() []string {
	defer perf.Track(nil, "step.AtmosHandler.KnownFields")()

	return knownFields(atmosFields)
}

// KnownFields lists the fields the cancel step reads: the background steps to stop.
func (h *CancelHandler) KnownFields() []string {
	defer perf.Track(nil, "step.CancelHandler.KnownFields")()

	return []string{"for"}
}

// KnownFields lists the fields the cast step and its actions read.
func (h *CastHandler) KnownFields() []string {
	defer perf.Track(nil, "step.CastHandler.KnownFields")()

	return knownFields(castStepFields)
}

// KnownFields lists the fields the choose step reads.
func (h *ChooseHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ChooseHandler.KnownFields")()

	return knownFields(chooseFields)
}

// KnownFields lists the fields the clear step reads: none beyond the common fields.
func (h *ClearHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ClearHandler.KnownFields")()

	return []string{}
}

// KnownFields lists the fields the confirm step reads.
func (h *ConfirmHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ConfirmHandler.KnownFields")()

	return knownFields(confirmFields)
}

// KnownFields lists the fields the container step reads.
func (h *ContainerHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ContainerHandler.KnownFields")()

	return knownFields(containerStepFields)
}

// KnownFields lists the fields the emulator step reads.
func (h *EmulatorHandler) KnownFields() []string {
	defer perf.Track(nil, "step.EmulatorHandler.KnownFields")()

	return knownFields(emulatorFields)
}

// KnownFields lists the fields the env step reads.
func (h *EnvHandler) KnownFields() []string {
	defer perf.Track(nil, "step.EnvHandler.KnownFields")()

	return knownFields(envFields)
}

// KnownFields lists the fields the exec step reads.
func (h *ExecHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ExecHandler.KnownFields")()

	return knownFields(execFields)
}

// KnownFields lists the fields the exit step reads.
func (h *ExitHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ExitHandler.KnownFields")()

	return knownFields(exitFields)
}

// KnownFields lists the fields the file step reads.
func (h *FileHandler) KnownFields() []string {
	defer perf.Track(nil, "step.FileHandler.KnownFields")()

	return knownFields(fileFields)
}

// KnownFields lists the fields the filter step reads.
func (h *FilterHandler) KnownFields() []string {
	defer perf.Track(nil, "step.FilterHandler.KnownFields")()

	return knownFields(filterFields)
}

// KnownFields lists the fields the format step reads.
func (h *FormatHandler) KnownFields() []string {
	defer perf.Track(nil, "step.FormatHandler.KnownFields")()

	return stepYAMLFields(contentField)
}

// KnownFields lists the fields the hint step reads.
func (h *HintHandler) KnownFields() []string {
	defer perf.Track(nil, "step.HintHandler.KnownFields")()

	return stepYAMLFields(contentField)
}

// KnownFields lists the fields the http step (and its webhook alias) reads.
func (h *HTTPHandler) KnownFields() []string {
	defer perf.Track(nil, "step.HTTPHandler.KnownFields")()

	return knownFields(httpFields)
}

// KnownFields lists the fields the input step reads.
func (h *InputHandler) KnownFields() []string {
	defer perf.Track(nil, "step.InputHandler.KnownFields")()

	return knownFields(inputFields)
}

// KnownFields lists the fields the join step reads.
func (h *JoinHandler) KnownFields() []string {
	defer perf.Track(nil, "step.JoinHandler.KnownFields")()

	return knownFields(joinFields)
}

// KnownFields lists the fields the junit step reads.
func (h *JUnitHandler) KnownFields() []string {
	defer perf.Track(nil, "step.JUnitHandler.KnownFields")()

	return knownFields(junitFields)
}

// KnownFields lists the fields the linebreak step reads.
func (h *LinebreakHandler) KnownFields() []string {
	defer perf.Track(nil, "step.LinebreakHandler.KnownFields")()

	return stepYAMLFields("Count")
}

// KnownFields lists the fields the log step reads.
func (h *LogHandler) KnownFields() []string {
	defer perf.Track(nil, "step.LogHandler.KnownFields")()

	return knownFields(logFields)
}

// KnownFields lists the fields the markdown step reads.
func (h *MarkdownHandler) KnownFields() []string {
	defer perf.Track(nil, "step.MarkdownHandler.KnownFields")()

	return stepYAMLFields(contentField)
}

// KnownFields lists the fields the matrix step reads.
func (h *MatrixHandler) KnownFields() []string {
	defer perf.Track(nil, "step.MatrixHandler.KnownFields")()

	return append(knownFields(controlStepFields), stepYAMLFields("Matrix")...)
}

// KnownFields lists the fields the pager step reads.
func (h *PagerHandler) KnownFields() []string {
	defer perf.Track(nil, "step.PagerHandler.KnownFields")()

	return knownFields(pagerFields)
}

// KnownFields lists the fields the parallel step reads.
func (h *ParallelHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ParallelHandler.KnownFields")()

	return knownFields(controlStepFields)
}

// KnownFields lists the fields the require step (and its assert alias) reads.
func (h *RequireHandler) KnownFields() []string {
	defer perf.Track(nil, "step.RequireHandler.KnownFields")()

	return knownFields(requireFields)
}

// KnownFields lists the fields the say step reads.
func (h *SayHandler) KnownFields() []string {
	defer perf.Track(nil, "step.SayHandler.KnownFields")()

	return knownFields(sayFields)
}

// KnownFields lists the fields the script step reads. `command` is rejected: a script step
// carries its body in `script`.
func (h *ScriptHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ScriptHandler.KnownFields")()

	return knownFields(scriptFields)
}

// KnownFields lists the fields the shell step reads.
func (h *ShellHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ShellHandler.KnownFields")()

	return knownFields(shellFields)
}

// KnownFields lists the fields the spin step reads.
func (h *SpinHandler) KnownFields() []string {
	defer perf.Track(nil, "step.SpinHandler.KnownFields")()

	return knownFields(spinFields)
}

// KnownFields lists the fields the stage step reads.
func (h *StageHandler) KnownFields() []string {
	defer perf.Track(nil, "step.StageHandler.KnownFields")()

	return stepYAMLFields("Title")
}

// KnownFields lists the fields the store step reads.
func (h *StoreHandler) KnownFields() []string {
	defer perf.Track(nil, "step.StoreHandler.KnownFields")()

	return knownFields(storeFields)
}

// KnownFields lists the fields the style step reads.
func (h *StyleHandler) KnownFields() []string {
	defer perf.Track(nil, "step.StyleHandler.KnownFields")()

	return knownFields(styleStepFields)
}

// KnownFields lists the fields the table step reads.
func (h *TableHandler) KnownFields() []string {
	defer perf.Track(nil, "step.TableHandler.KnownFields")()

	return knownFields(tableFields)
}

// KnownFields lists the fields the test step reads: its checks and failure policy.
func (h *TestHandler) KnownFields() []string {
	defer perf.Track(nil, "step.TestHandler.KnownFields")()

	return stepYAMLFields("Steps", "Fail")
}

// KnownFields lists the fields the tflint step reads.
func (h *TFLintHandler) KnownFields() []string {
	defer perf.Track(nil, "step.TFLintHandler.KnownFields")()

	return knownFields(tflintFields)
}

// KnownFields lists the fields the title step reads.
func (h *TitleHandler) KnownFields() []string {
	defer perf.Track(nil, "step.TitleHandler.KnownFields")()

	return stepYAMLFields(contentField)
}

// KnownFields lists the fields the toast step reads.
func (h *ToastHandler) KnownFields() []string {
	defer perf.Track(nil, "step.ToastHandler.KnownFields")()

	return knownFields(toastFields)
}

// KnownFields lists the fields the wait step reads: the background steps to wait for.
func (h *WaitHandler) KnownFields() []string {
	defer perf.Track(nil, "step.WaitHandler.KnownFields")()

	return []string{"for"}
}

// KnownFields lists the fields the wait-all step reads: none beyond the common fields.
func (h *WaitAllHandler) KnownFields() []string {
	defer perf.Track(nil, "step.WaitAllHandler.KnownFields")()

	return []string{}
}

// KnownFields lists the fields the workdir step reads.
func (h *WorkdirHandler) KnownFields() []string {
	defer perf.Track(nil, "step.WorkdirHandler.KnownFields")()

	return knownFields(workdirFields)
}

// KnownFields lists the fields the write step reads.
func (h *WriteHandler) KnownFields() []string {
	defer perf.Track(nil, "step.WriteHandler.KnownFields")()

	return knownFields(writeFields)
}
