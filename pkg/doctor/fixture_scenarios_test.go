package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/validation"
)

// scenarioRepairRules are the rules whose findings a repair run can fix.
var scenarioRepairRules = []string{ruleTerraformReconfigure, ruleCommandProvides, ruleHookEvent}

// readScenarioFile returns the content of a file in a copied scenario.
func readScenarioFile(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	require.NoError(t, err)
	return string(content)
}

// requireRepairStatus asserts every finding of the given rules has the wanted status and fixable flag.
func requireRepairStatus(t *testing.T, r *Report, rules []string, status string) {
	t.Helper()
	for _, rule := range rules {
		found := ecFindings(r, rule)
		require.NotEmpty(t, found, "the scenario must produce %s findings", rule)
		for i := range found {
			assert.Equal(t, status, found[i].Status, "%s at %s:%d: %s", rule, found[i].File, found[i].Line, found[i].Reason)
		}
	}
}

// TestEligibleScenario pins the baseline: every supported repair is available, then applied and idempotent.
func TestEligibleScenario(t *testing.T) {
	root := copyScenario(t, "eligible")

	diagnosis := ecRun(t, &Options{Root: root, Offline: true})
	requireRepairStatus(t, &diagnosis, scenarioRepairRules, "available")
	for _, rule := range scenarioRepairRules {
		for _, f := range ecFindings(&diagnosis, rule) {
			assert.True(t, f.Fixable, "%s at %s:%d", rule, f.File, f.Line)
		}
	}
	require.Empty(t, diagnosis.ChangedFiles, "diagnosis must not write")

	fixed := ecRun(t, &Options{Root: root, Offline: true, Fix: true})
	requireRepairStatus(t, &fixed, scenarioRepairRules, statusFixed)
	require.ElementsMatch(t, []string{"atmos.yaml", "stacks/deploy/dev.yaml"}, fixed.ChangedFiles)
	require.Equal(t, 0, fixed.ExitCode("error"))
	atmos := readScenarioFile(t, root, "atmos.yaml")
	assert.Contains(t, atmos, "reconfigure: never")
	assert.NotContains(t, atmos, "init_run_reconfigure")
	assert.NotContains(t, atmos, "semantic_type")
	assert.Contains(t, readScenarioFile(t, root, "stacks/deploy/dev.yaml"), "after.terraform.apply")

	again := ecRun(t, &Options{Root: root, Offline: true, Fix: true})
	require.Empty(t, again.ChangedFiles, "a second run must find nothing to repair")
}

// TestLayeredScenario pins that a configuration imported by atmos.yaml blocks the repairs of atmos.yaml,
// names the import, and still leaves the stack file repairable.
func TestLayeredScenario(t *testing.T) {
	root := copyScenario(t, "layered")
	original := readScenarioFile(t, root, "atmos.yaml")

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true})

	for _, rule := range []string{ruleTerraformReconfigure, ruleCommandProvides} {
		found := ecFindings(&r, rule)
		require.NotEmpty(t, found, rule)
		for i := range found {
			assert.Equal(t, statusVerificationIncomplete, found[i].Status, rule)
			assert.False(t, found[i].Fixable, rule)
			assert.Contains(t, found[i].Reason, "Imports layer other files", rule)
			assert.Contains(t, found[i].Reason, "atmos.yaml:5", "the reason must name the import line: %s", rule)
			assert.NotContains(t, found[i].Remediation, "--fix", "a blocked finding must say what to do by hand")
		}
	}
	requireRepairStatus(t, &r, []string{ruleHookEvent}, statusFixed)
	require.Equal(t, []string{"stacks/deploy/dev.yaml"}, r.ChangedFiles)
	assert.Equal(t, original, readScenarioFile(t, root, "atmos.yaml"), "the blocked file must be left untouched")
	assert.Contains(t, readScenarioFile(t, root, "stacks/deploy/dev.yaml"), "after.terraform.apply")
}

// TestTemplatedStepScenario pins that a template in an unrelated custom command step does not block repairs.
func TestTemplatedStepScenario(t *testing.T) {
	root := copyScenario(t, "templated-step")

	diagnosis := ecRun(t, &Options{Root: root, Offline: true})
	requireRepairStatus(t, &diagnosis, scenarioRepairRules, "available")

	fixed := ecRun(t, &Options{Root: root, Offline: true, Fix: true})
	requireRepairStatus(t, &fixed, scenarioRepairRules, statusFixed)
	atmos := readScenarioFile(t, root, "atmos.yaml")
	assert.Contains(t, atmos, `echo "inspect {{ .Arguments.target }}"`, "the template must survive the repair unchanged")
	assert.Contains(t, atmos, "reconfigure: never")
	assert.Equal(t, 0, fixed.ExitCode("error"))
}

// TestSourcesScenarioOffline pins that skipping repository requests adds notices that never count toward a threshold.
func TestSourcesScenarioOffline(t *testing.T) {
	root := copyScenario(t, "sources")

	r := ecRun(t, &Options{Root: root, Offline: true})

	var offline, others []Finding
	for _, f := range ecFindings(&r, ruleSourceRepository) {
		if f.Message == repositoryOfflineMessage {
			offline = append(offline, f)
		} else {
			others = append(others, f)
		}
	}
	require.NotEmpty(t, offline, "declared sources must be reported as skipped")
	for i := range offline {
		assert.Equal(t, validation.SeverityNotice, offline[i].Severity, "%s:%d", offline[i].File, offline[i].Line)
		assert.Equal(t, statusVerificationIncomplete, offline[i].Status)
	}
	// The templated source only templates its ref, so it is checked like the literal ones.
	require.Empty(t, others)

	// Only the offline notices are removed here, so this proves they do not decide the exit code.
	report := Report{Version: 1, Findings: append([]Finding(nil), offline...)}
	assert.Equal(t, 0, report.ExitCode("warning"))
	assert.Equal(t, 0, r.ExitCode("error"))
	assert.Equal(t, 0, r.ExitCode("none"))
	for _, rule := range []string{ruleSchema, ruleDeprecatedField, ruleCommandProvides, ruleHookEvent, ruleTerraformReconfigure} {
		assert.Empty(t, ecFindings(&r, rule), rule)
	}
	moved := ecFindings(&r, ruleComponentMonorepo)
	require.NotEmpty(t, moved, "monorepo sources are reported even when verification is skipped")
	for i := range moved {
		assert.Equal(t, statusVerificationIncomplete, moved[i].Status, "%s:%d", moved[i].File, moved[i].Line)
		assert.Equal(t, monorepoOfflineNote, moved[i].Reason)
	}
}

// TestEdgeScenario pins an empty stack file, a multi-document workflow, and a stack value that
// contains template delimiters.
func TestEdgeScenario(t *testing.T) {
	root := copyScenario(t, "edge")
	empty := readScenarioFile(t, root, "stacks/deploy/empty.yaml")
	dev := readScenarioFile(t, root, "stacks/deploy/dev.yaml")
	require.Contains(t, dev, "!important: rendered by {{ templates }} docs elsewhere")

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true})

	// A multi-document workflow is reported and never edited.
	var workflow []Finding
	for _, f := range ecFindings(&r, ruleSchema) {
		if f.File == ".github/workflows/multi.yaml" {
			workflow = append(workflow, f)
		}
	}
	require.Len(t, workflow, 1)
	assert.Equal(t, validation.SeverityError, workflow[0].Severity)
	assert.False(t, workflow[0].Fixable)

	// Template delimiters in an unrelated value do not block the hook-event repair, and the value is preserved.
	requireRepairStatus(t, &r, scenarioRepairRules, statusFixed)
	assert.ElementsMatch(t, []string{"atmos.yaml", "stacks/deploy/dev.yaml"}, r.ChangedFiles)
	repaired := readScenarioFile(t, root, "stacks/deploy/dev.yaml")
	assert.Contains(t, repaired, "!important: rendered by {{ templates }} docs elsewhere")
	assert.Contains(t, repaired, "after.terraform.apply")

	// An empty stack file is never rewritten by a repair run.
	assert.Equal(t, empty, readScenarioFile(t, root, "stacks/deploy/empty.yaml"))
	assert.NotContains(t, r.ChangedFiles, "stacks/deploy/empty.yaml")
}
