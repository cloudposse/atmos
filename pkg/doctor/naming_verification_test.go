package doctor

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	stackexec "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNamingDocumentVerificationRejectsUnexpectedChanges(t *testing.T) {
	change := edit{kind: ruleLegacyNaming, path: []string{"stacks"}, old: "name_pattern", target: "name_template", from: "{stage}", value: "{{ .vars.stage }}"}
	const before = "stacks:\n  name_pattern: '{stage}'\n"
	const after = "stacks:\n  name_template: '{{ .vars.stage }}'\n"
	require.NoError(t, verifyDocument([]byte(before), []byte(after), []edit{change}))
	for _, tc := range []struct{ name, before, after string }{
		{"both fields", before, "stacks:\n  name_pattern: '{stage}'\n  name_template: '{{ .vars.stage }}'\n"},
		{"neither field", before, "stacks: {}\n"},
		{"old value changed", "stacks:\n  name_pattern: '{region}'\n", after},
		{"replacement changed", before, "stacks:\n  name_template: '{{ .vars.region }}'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, verifyDocument([]byte(tc.before), []byte(tc.after), []edit{change}), ErrVerification)
		})
	}
}

func TestNamingTemplateValueVerification(t *testing.T) {
	change := edit{kind: ruleLegacyNaming, path: []string{"metadata"}, old: "terraform_workspace_template", target: "terraform_workspace_template", from: "{component}", value: "{{ .atmos_component }}"}
	const before = "metadata:\n  terraform_workspace_template: '{component}' # retained\n"
	const after = "metadata:\n  terraform_workspace_template: '{{ .atmos_component }}' # retained\n"
	// This stage verifies only the authored replacement; runtime equivalence is a separate gate.
	require.NoError(t, verifyDocument([]byte(before), []byte(after), []edit{change}))
	for _, content := range []string{"metadata: {}\n", "metadata:\n  terraform_workspace_template: unexpected\n"} {
		node, err := parse([]byte(content))
		require.NoError(t, err)
		require.ErrorIs(t, normalizeNaming(nodeAt(rootNode(node), change.path), &change), ErrVerification)
	}
}

// failingNamingReporter uses production resolution until a later verification becomes unavailable.
type failingNamingReporter struct {
	vtProductionValidator
	calls, failAt int
	err           error
}

func (r *failingNamingReporter) NamingResults(ctx context.Context, config *schema.AtmosConfiguration, inputs StackInputs) (map[naming.Identity]naming.Result, error) {
	r.calls++
	if r.calls >= r.failAt {
		return nil, r.err
	}
	return stackexec.StaticNamingResults(ctx, config, stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader})
}

func TestNamingFixRechecksEquivalenceBeforeWriting(t *testing.T) {
	root := newProject(t)
	const original = "stacks:\n  base_path: stacks\n  name_pattern: '{stage}'\n"
	path := projectFile(t, root, "atmos.yaml", original)
	projectFile(t, root, "stacks/dev.yaml", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    app: {}\n")
	failure := errors.New("naming inputs became unavailable")
	reporter := &failingNamingReporter{failAt: 4, err: failure}
	engine := vtStacksEngine(t, reporter)
	report, err := engine.Run(context.Background(), Options{Root: root, Only: []string{ruleLegacyNaming}, Offline: true, Fix: true})
	require.NoError(t, err)
	require.Equal(t, 4, reporter.calls, "diagnostic equivalence must be checked again on the final candidate")
	require.Empty(t, report.ChangedFiles)
	require.Empty(t, report.Backup)
	findings := ecFindings(&report, ruleLegacyNaming)
	require.Len(t, findings, 1)
	require.Equal(t, statusVerificationFailed, findings[0].Status)
	require.Contains(t, findings[0].Reason, failure.Error())
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(content))
}

func TestNamingAssessmentCancellationAndMalformedCandidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := testEngine(t)
	ready, err := e.assessNaming(ctx, &Options{}, &Report{}, []edit{{kind: ruleHookEvent}})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, ready)

	change := edit{kind: ruleLegacyNaming, doc: &document{path: "atmos.yaml", content: []byte("stacks: [\n")}, path: []string{"stacks"}, old: "name_pattern", target: "name_template", value: "{{ .vars.stage }}"}
	report := Report{Findings: []Finding{{RuleID: ruleLegacyNaming, Fixable: true}}}
	ready, err = e.assessNaming(context.Background(), &Options{}, &report, []edit{change})
	require.NoError(t, err)
	require.Empty(t, ready)
	require.False(t, report.Findings[0].Fixable)
	require.Equal(t, statusVerificationIncomplete, report.Findings[0].Status)
	require.NotEmpty(t, report.Findings[0].Reason)
}

func TestNamingSnapshotCancellation(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks:\n  base_path: stacks\n")
	e := vtStacksEngine(t, &failingNamingReporter{failAt: 1})
	options := Options{Root: root, Offline: true}
	require.NoError(t, e.configure(&options))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := e.namingSnapshot(ctx, &options, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, results)
}
