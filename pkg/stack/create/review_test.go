package create

import (
	"bytes"
	"context"
	stdio "io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	iolib "github.com/cloudposse/atmos/pkg/io"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

type creationStreams struct{ output bytes.Buffer }

func (s *creationStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *creationStreams) Output() stdio.Writer    { return &s.output }
func (s *creationStreams) Error() stdio.Writer     { return &s.output }
func (s *creationStreams) RawOutput() stdio.Writer { return &s.output }
func (s *creationStreams) RawError() stdio.Writer  { return &s.output }

func TestPrepareDoesNotPrintScaffoldPreview(t *testing.T) {
	cfg, _ := fixture(t)
	streams := &creationStreams{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	atmosui.InitFormatter(ioCtx)
	t.Cleanup(atmosui.Reset)
	renderer := generatorui.NewInitUI(ioCtx, terminal.New(terminal.WithIO(iolib.NewTerminalWriter(ioCtx))))
	customFixture(t, customDefinition)
	write(t, "scaffolds/app/README.md", "# Preview-only README\n")
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}, renderer)
	require.NoError(t, err)
	require.NotEmpty(t, plan.Generation.Files)
	assert.NotContains(t, streams.output.String(), "would create")
	assert.NotContains(t, streams.output.String(), "Would generate")
	assert.NotContains(t, streams.output.String(), "Preview-only README")
	assert.NoFileExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "dev.yaml"))
}

func TestDeletedManifestRecordConflictNamesBothPaths(t *testing.T) {
	cfg, renderer := fixture(t)
	opts := &Options{File: "stacks/deploy/dev.yaml", Defaults: true}
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.NoError(t, plan.Apply(context.Background(), nil))
	recordPath := plan.Generation.Files[len(plan.Generation.Files)-1].Path
	before, err := os.ReadFile(filepath.Join(plan.Root, recordPath))
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(plan.Root, plan.Manifest)))

	_, err = Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrFileWrite)
	details := strings.Join(errUtils.AllDetails(err), "\n")
	assert.Contains(t, details, opts.File)
	assert.Contains(t, details, filepath.ToSlash(recordPath))
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "recorded outputs")
	after, err := os.ReadFile(filepath.Join(plan.Root, recordPath))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.NoFileExists(t, filepath.Join(plan.Root, plan.Manifest))
}

// outcomeHook cancels generation before writes and records the failure hook's outcome.
type outcomeHook struct {
	runnerstep.BaseHandler
	cancel   context.CancelFunc
	outcomes *[]string
}

func (h *outcomeHook) Validate(*schema.WorkflowStep) error { return nil }

func (h *outcomeHook) Execute(_ context.Context, step *schema.WorkflowStep, vars *runnerstep.Variables) (*runnerstep.StepResult, error) {
	if step.Content == "before" {
		if h.cancel != nil {
			h.cancel()
		}
		return runnerstep.NewStepResult("before"), nil
	}
	resolved := vars.Env["ATMOS_HOOK_STATUS"] + "|" + vars.Env["ATMOS_HOOK_ERROR"]
	*h.outcomes = append(*h.outcomes, resolved)
	return runnerstep.NewStepResult(resolved), nil
}

func TestFailureHookReceivesCreationError(t *testing.T) {
	cfg, renderer := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	outcomes := []string{}
	handler := &outcomeHook{
		BaseHandler: runnerstep.NewBaseHandler("stack-create-outcome", runnerstep.CategoryOutput, false),
		cancel:      cancel,
		outcomes:    &outcomes,
	}
	runnerstep.Register(handler)
	definition := strings.Split(customDefinition, "  hooks:\n")[0] + `  hooks:
    before:
      events: [before.scaffold.generate]
      kind: step
      type: stack-create-outcome
      with:
        content: before
    failure:
      events: [after.scaffold.generate]
      when: failure
      kind: step
      type: stack-create-outcome
      with:
        content: after
`
	customFixture(t, definition)
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}, renderer)
	require.NoError(t, err)
	require.ErrorIs(t, plan.Apply(ctx, nil), context.Canceled)
	assert.Equal(t, []string{"failure|context canceled"}, outcomes)
	assert.NoFileExists(t, filepath.Join(plan.Root, plan.Manifest))

	// A successful retry must not invoke failure-only hooks.
	handler.cancel = nil
	outcomes = nil
	require.NoError(t, plan.Apply(context.Background(), nil))
	assert.Empty(t, outcomes)
}

func TestUnmarkedManifestTemplateHintsUseRenderingDelimiters(t *testing.T) {
	for _, tc := range []struct {
		name       string
		definition string
		content    string
		wantHint   bool
	}{
		{name: "default", definition: customDefinition, content: "vars:\n  stage: {{ .Config.environment | quote }}\n", wantHint: true},
		{
			name:       "scaffold delimiters",
			definition: strings.ReplaceAll(strings.ReplaceAll(strings.Replace(customDefinition, "spec:\n", "spec:\n  delimiters: ['[%', '%]']\n", 1), "{{", "[%"), "}}", "%]"),
			content:    "vars:\n  stage: [% .Config.environment | quote %]\n",
			wantHint:   true,
		},
		{
			name:       "file delimiters",
			definition: strings.Replace(customDefinition, "target: '{{ .Config.stack_file }}'", "target: '[% .Config.stack_file %]'\n      delimiters: ['[%', '%]']", 1),
			content:    "vars:\n  stage: [% .Config.environment | quote %]\n",
			wantHint:   true,
		},
		{name: "ordinary invalid YAML", definition: customDefinition, content: "vars: [\n"},
		{name: "foreign template", definition: customDefinition, content: "vars:\n  stage: {{ .Values.environment }}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := fixture(t)
			customFixture(t, tc.definition)
			write(t, "scaffolds/app/stack.yaml", tc.content)
			_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}, renderer)
			require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
			if tc.wantHint {
				require.ErrorIs(t, err, errUtils.ErrUnprocessedTemplate)
				assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "# atmos:template")
				assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), ".tmpl")
			} else {
				assert.NotErrorIs(t, err, errUtils.ErrUnprocessedTemplate)
			}
			assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks"))
		})
	}
}
