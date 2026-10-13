package doctor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/validation"
)

func TestRemediationNeverSuggestsFixForRepairsThatFixCannotApply(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "import: [extra.yaml]\ncomponents:\n  terraform:\n    init_run_reconfigure: false\n    init:\n      reconfigure: always\ncommands:\n  - name: c\n    arguments:\n      - name: a\n        type: stack\n        provides: component\n      - name: b\n        type: component\n")
	projectFile(t, root, "extra.yaml", "logs:\n  level: Debug\n")
	projectFile(t, root, "stacks/dev.yaml", "hooks:\n  h:\n    events: [after-terraform-apply]\n    command: store\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{categoryDeprecations}})

	statuses := map[string]bool{}
	for i := range r.Findings {
		f := &r.Findings[i]
		statuses[f.Status] = true
		if f.Status == "available" {
			require.Equal(t, remediationFix, f.Remediation)
			continue
		}
		require.NotContains(t, f.Remediation, "--fix", "%s %s: %s", f.RuleID, f.Status, f.Remediation)
	}
	require.True(t, statuses[statusManual], "the fixture must include a conflict")
	require.True(t, statuses[statusVerificationIncomplete], "the fixture must include a blocked repair")
	require.True(t, statuses["available"], "the fixture must include an applicable repair")
}

func TestRejectedRepairsStateTheManualEdit(t *testing.T) {
	tests := []struct {
		name string
		edit edit
		want string
	}{
		{"reconfigure", edit{kind: ruleTerraformReconfigure, value: "never"}, "init.reconfigure: never"},
		{"provides", edit{kind: ruleCommandProvides, old: "semantic_type"}, "Rename semantic_type to provides"},
		{"hook", edit{kind: ruleHookEvent, value: "after.terraform.apply"}, "after.terraform.apply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Findings: []Finding{{Status: "available", Fixable: true, Remediation: remediationFix}}}
			change := tt.edit
			rejectCandidate(r, []edit{change}, errors.New("rejected"))

			require.Equal(t, statusVerificationFailed, r.Findings[0].Status)
			require.False(t, r.Findings[0].Fixable)
			require.NotContains(t, r.Findings[0].Remediation, "--fix")
			require.Contains(t, r.Findings[0].Remediation, tt.want)
		})
	}
}

func TestOnlyTheReplacementKeyConflicts(t *testing.T) {
	tests := []struct {
		name, init string
		status     string
	}{
		{"init without reconfigure", "    init:\n      pass_vars: true\n", "available"},
		{"empty init", "    init: {}\n", "available"},
		{"init.reconfigure present", "    init:\n      reconfigure: auto\n", statusManual},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: false\n"+tt.init)

			r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{ruleTerraformReconfigure}})

			found := ecFindings(&r, ruleTerraformReconfigure)
			require.Len(t, found, 1)
			require.Equal(t, tt.status, found[0].Status, found[0].Reason)
			if tt.status == statusManual {
				require.Contains(t, found[0].Reason, "init.reconfigure is already declared")
				require.Contains(t, found[0].Remediation, "Delete init_run_reconfigure by hand")
			}
		})
	}
}

func TestInertSemanticTypesAreNotMigrated(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "commands:\n  - name: greet\n    arguments:\n      - name: who\n        type: string\n    flags:\n      - name: stack\n        semantic_type: Stack\n      - name: other\n        semantic_type: banana\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

	require.Empty(t, r.ChangedFiles)
	found := ecFindings(&r, ruleDeprecatedField)
	require.Len(t, found, 3)
	byPath := map[string]Finding{}
	for i := range found {
		byPath[found[i].Path] = found[i]
	}
	argument := byPath["commands[0].arguments[0].type"]
	require.Contains(t, argument.Remediation, `Remove it`)
	require.Contains(t, argument.Remediation, `"string" is neither`)
	require.NotContains(t, argument.Remediation, "Use provides")
	mistyped := byPath["commands[0].flags[0].semantic_type"]
	require.Contains(t, mistyped.Remediation, "provides: stack")
	require.Contains(t, mistyped.Remediation, "case-sensitive")
	unknown := byPath["commands[0].flags[1].semantic_type"]
	require.Contains(t, unknown.Remediation, `"banana" is neither`)
}

func TestHookEventFindingsNameTheirEvent(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "hooks:\n  a:\n    events: [after-terraform-apply, before-helmfile-sync, after-terraform-init]\n    command: store\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleHookEvent}})

	found := ecFindings(&r, ruleHookEvent)
	require.Len(t, found, 2)
	require.Equal(t, `Legacy hook-event declaration: "after-terraform-apply" is now "after.terraform.apply"`, found[0].Message)
	require.Equal(t, `Legacy hook-event declaration: "after-terraform-init" is now "after.terraform.init"`, found[1].Message)
	require.NotEqual(t, found[0].Message, found[1].Message)
}

func TestExitCodeForRequestedRepairs(t *testing.T) {
	failed := Finding{Severity: validation.SeverityWarning, Status: statusVerificationFailed}
	fixed := Finding{Severity: validation.SeverityWarning, Status: statusFixed}
	warning := Finding{Severity: validation.SeverityWarning, Status: statusManual}
	tests := []struct {
		name      string
		report    Report
		threshold string
		want      int
	}{
		{"a failed repair exits 1 below the error threshold", Report{Fix: true, Findings: []Finding{failed}}, "error", 1},
		{"a failed repair exits 1 even when findings are ignored", Report{Fix: true, Findings: []Finding{failed}}, "none", 1},
		{"a failed repair among fixed ones exits 1", Report{Fix: true, Findings: []Finding{fixed, failed}}, "error", 1},
		{"a dry run only applies the threshold", Report{Fix: true, DryRun: true, Findings: []Finding{failed}}, "error", 0},
		{"a dry run still honors a warning threshold", Report{Fix: true, DryRun: true, Findings: []Finding{failed}}, "warning", 1},
		{"fixed repairs exit 0", Report{Findings: []Finding{fixed}}, "warning", 0},
		{"unfixed warnings stay below the error threshold", Report{Findings: []Finding{warning}}, "error", 0},
		{"operational failures exit 2 over a failed repair", Report{Fix: true, OperationalError: "write failed", Findings: []Finding{failed}}, "error", 2},
		{"without --fix a failed verification is only a finding", Report{Findings: []Finding{failed}}, "error", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.report.ExitCode(tt.threshold))
		})
	}
}

func TestRejectedRepairExitsOneAfterFix(t *testing.T) {
	setup := func(t *testing.T) string {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: stack-schema.json\n"+ecLegacyReconfigure)
		projectFile(t, root, "stack-schema.json", `{"properties":{"hooks":{"properties":{"audit":{"properties":{"events":{"items":{"const":"before-terraform-plan"}}}}}}}}`)
		projectFile(t, root, "stacks/dev.yaml", "hooks:\n  audit:\n    events: [before-terraform-plan]\n    command: echo\n")
		return root
	}

	t.Run("a fix run with a rejected repair exits 1 whatever the threshold", func(t *testing.T) {
		r := ecRun(t, &Options{Root: setup(t), Offline: true, Fix: true, Only: []string{categoryDeprecations}})

		require.Equal(t, statusVerificationFailed, ecFindings(&r, ruleHookEvent)[0].Status)
		require.Equal(t, 1, r.ExitCode("error"))
		require.Equal(t, 1, r.ExitCode("none"))
	})

	t.Run("a dry run keeps the threshold semantics", func(t *testing.T) {
		r := ecRun(t, &Options{Root: setup(t), Offline: true, Fix: true, DryRun: true, Only: []string{categoryDeprecations}})

		require.Equal(t, statusVerificationFailed, ecFindings(&r, ruleHookEvent)[0].Status)
		require.Equal(t, 0, r.ExitCode("error"))
	})

	t.Run("a fix run that applies everything exits 0", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)

		r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

		require.Equal(t, 0, r.ExitCode("error"))
	})
}

func TestRecoveryModeIsAnOctalString(t *testing.T) {
	data, err := json.Marshal(RecoveryEntry{Path: "p", Mode: RecoveryMode(0o644)})
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"p","copy":"","mode":"0644","before_sha256":"","after_sha256":""}`, string(data))

	tests := []struct {
		name, mode string
		want       RecoveryMode
		wantErr    bool
	}{
		{"octal string", `"0640"`, 0o640, false},
		{"private octal string", `"0600"`, 0o600, false},
		{"decimal number from older manifests", `420`, 0o644, false},
		{"non-octal string", `"0988"`, 0, true},
		{"unsupported value", `true`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry RecoveryEntry
			err := json.Unmarshal([]byte(`{"path":"p","mode":`+tt.mode+`}`), &entry)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrRecoveryManifest)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, entry.Mode)
		})
	}
}

func TestBackupManifestRecordsOctalMode(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: false\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

	raw, err := os.ReadFile(filepath.Join(r.Backup, "manifest.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"mode": "0640"`)
	var entries []RecoveryEntry
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Equal(t, RecoveryMode(0o640), entries[0].Mode)
}
