package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

const ecLegacyReconfigure = "components:\n  terraform:\n    init_run_reconfigure: false\n"

// ecDiscover resolves options and runs discovery the same way a run does.
func ecDiscover(t *testing.T, e *Engine, o *Options) *project {
	t.Helper()
	require.NoError(t, validateOptions(o))
	require.NoError(t, e.configure(o))
	p, err := e.discover(context.Background(), o, &Report{})
	require.NoError(t, err)
	return p
}

func TestEcUnchangedDetectsSourceDrift(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(t *testing.T, root string)
		wantErr error
		message string
	}{
		{"nothing changed", func(*testing.T, string) {}, nil, ""},
		{"content changed", func(t *testing.T, root string) {
			projectFile(t, root, "stacks/dev.yaml", "vars:\n  changed: true\n")
		}, ErrChanged, "stacks/dev.yaml"},
		{"permissions changed", func(t *testing.T, root string) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows does not report POSIX permission bits.")
			}
			require.NoError(t, os.Chmod(filepath.Join(root, "stacks", "dev.yaml"), 0o600))
		}, ErrChanged, "stacks/dev.yaml"},
		{"new source appeared", func(t *testing.T, root string) {
			projectFile(t, root, "stacks/new.yaml", "vars: {}\n")
		}, ErrChanged, "source closure changed"},
		{"source removed", func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, "stacks", "dev.yaml")))
		}, ErrChanged, "source closure changed"},
		{"source replaced by another file of the same count", func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, "stacks", "dev.yaml")))
			projectFile(t, root, "stacks/renamed.yaml", "vars: {}\n")
		}, ErrChanged, "new source stacks/renamed.yaml"},
		// Drift detection compares content and mode only; refusing to write through a link is the job of staging and applying.
		{"source replaced by a symlink to identical content", func(t *testing.T, root string) {
			link := filepath.Join(root, "stacks", "dev.yaml")
			copyPath := projectFile(t, root, "copy/dev.yaml", "vars: {}\n")
			require.NoError(t, os.Remove(link))
			if err := os.Symlink(copyPath, link); err != nil {
				t.Skipf("symlinks are unavailable: %v", err)
			}
		}, nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
			e := testEngine(t)
			o := &Options{Root: root, Offline: true}
			p := ecDiscover(t, e, o)
			tt.mutate(t, root)
			err := e.unchanged(context.Background(), p, o)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			require.Contains(t, filepath.ToSlash(err.Error()), tt.message)
		})
	}
}

func TestEcUnchangedDetectsCustomSchemaDrift(t *testing.T) {
	setup := func(t *testing.T) (root string) {
		root = newProject(t)
		projectFile(t, root, "atmos.yaml", "schemas:\n  custom:\n    schema: schema.json\n    matches: [data.yaml]\n")
		projectFile(t, root, "schema.json", `{"type":"object"}`)
		projectFile(t, root, "data.yaml", "name: value\n")
		return root
	}
	capture := func(t *testing.T, e *Engine, p *project, o *Options) {
		t.Helper()
		require.NoError(t, e.customSchemas(context.Background(), p, o, &Report{}, validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})))
		require.Contains(t, p.extra, filepath.Join(p.root, "schema.json"))
	}

	t.Run("captured schemas that did not change pass", func(t *testing.T) {
		root := setup(t)
		e, o := testEngine(t), &Options{Root: root, Offline: true}
		p := ecDiscover(t, e, o)
		capture(t, e, p, o)
		require.NoError(t, e.unchanged(context.Background(), p, o))
	})

	t.Run("a rewritten schema is a change", func(t *testing.T) {
		root := setup(t)
		e, o := testEngine(t), &Options{Root: root, Offline: true}
		p := ecDiscover(t, e, o)
		capture(t, e, p, o)
		projectFile(t, root, "schema.json", `{"type":"object","required":["name"]}`)
		err := e.unchanged(context.Background(), p, o)
		require.ErrorIs(t, err, ErrChanged)
		require.Contains(t, filepath.ToSlash(err.Error()), "schema.json")
	})

	t.Run("a schema that was not captured is a changed closure", func(t *testing.T) {
		root := setup(t)
		e, o := testEngine(t), &Options{Root: root, Offline: true}
		p := ecDiscover(t, e, o)
		err := e.unchanged(context.Background(), p, o)
		require.ErrorIs(t, err, ErrChanged)
		require.Contains(t, err.Error(), "custom schema source closure changed")
	})
}

func TestEcStagingRejectsUnapplicableEdits(t *testing.T) {
	root := newProject(t)
	doc := &document{path: filepath.Join(root, "x.yaml"), kind: "stack", content: []byte("a: 1\n")}
	p := &project{root: root, docs: []*document{doc}}
	edits := []edit{{doc: doc, path: []string{"a"}, kind: "unsupported-kind", finding: 0}}
	newReport := func() *Report {
		return &Report{Findings: []Finding{{RuleID: "unsupported-kind", Status: "available", Fixable: true}}}
	}

	t.Run("the finding records why the repair was refused", func(t *testing.T) {
		var events []Event
		e := &Engine{Observe: func(event Event) { events = append(events, event) }}
		r := newReport()
		candidates, err := e.stageCandidates(context.Background(), p, r, edits)
		require.NoError(t, err)
		require.Empty(t, candidates)
		require.Equal(t, statusVerificationFailed, r.Findings[0].Status)
		require.False(t, r.Findings[0].Fixable)
		require.Contains(t, r.Findings[0].Reason, ErrVerification.Error())
		require.Equal(t, statusVerificationFailed, events[len(events)-1].Status)
	})

	t.Run("cancellation stops staging", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := newReport()
		_, err := (&Engine{}).stageCandidates(ctx, p, r, edits)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, "available", r.Findings[0].Status, "an unstaged edit must keep its decision")
	})
}

func TestEcVerifySchemaRejectsOnlyNewErrors(t *testing.T) {
	root := newProject(t)
	source := projectFile(t, root, "schema.json", `{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}}}`)
	v := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})

	for _, tt := range []struct {
		name          string
		source        string
		before, after string
		wantErr       bool
		message       string
	}{
		{"no schema accepts any change", "", "a: 1\n", "a: x\n", false, ""},
		{"valid to valid", source, "a: 1\n", "a: 2\n", false, ""},
		{"invalid to valid", source, "a: x\n", "a: 1\n", false, ""},
		{"valid to invalid", source, "a: 1\n", "a: x\n", true, "new schema error"},
		{"baseline error preserved while another value changes", source, "a: x\n", "a: x\nb: 1\n", false, ""},
		{"baseline error preserved while a second error appears", source, "a: x\n", "a: x\nb: y\n", true, "new schema error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySchema(v, tt.source, []byte(tt.before), []byte(tt.after))
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrVerification)
			require.Contains(t, err.Error(), tt.message)
		})
	}

	t.Run("an unreadable schema is an error rather than a pass", func(t *testing.T) {
		require.Error(t, verifySchema(v, filepath.Join(root, "missing.json"), []byte("a: 1\n"), []byte("a: 1\n")))
	})
}

func TestEcBackupFailuresStopBeforeAnyWrite(t *testing.T) {
	root := newProject(t)
	blocker := projectFile(t, root, "blocker", "not a directory")
	doc := &document{path: filepath.Join(root, "x.yaml"), content: []byte("a: 1\n"), mode: 0o640}
	candidates := []candidate{{doc: doc, content: []byte("a: 2\n")}}

	t.Run("backup directory cannot be created", func(t *testing.T) {
		e := &Engine{BackupDir: func() (string, error) { return filepath.Join(blocker, "backups"), nil }}
		r := &Report{}
		require.Error(t, e.backup(candidates, r))
		require.Empty(t, r.Backup)
	})

	t.Run("backup records exact originals and digests", func(t *testing.T) {
		base := t.TempDir()
		e := &Engine{BackupDir: func() (string, error) { return base, nil }}
		r := &Report{}
		require.NoError(t, e.backup(candidates, r))
		require.Equal(t, base, filepath.Dir(r.Backup))
		require.NotEmpty(t, r.Recovery)
		var entries []RecoveryEntry
		manifest, err := os.ReadFile(filepath.Join(r.Backup, "manifest.json"))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(manifest, &entries))
		require.Len(t, entries, 1)
		require.Equal(t, doc.path, entries[0].Path)
		require.Equal(t, digest([]byte("a: 1\n")), entries[0].Before)
		require.Equal(t, digest([]byte("a: 2\n")), entries[0].After)
		require.Equal(t, RecoveryMode(0o640), entries[0].Mode)
		copied, err := os.ReadFile(filepath.Join(r.Backup, entries[0].Copy))
		require.NoError(t, err)
		require.Equal(t, "a: 1\n", string(copied))
	})
}

func TestEcPrepareWritesHonorsCancellationBeforeBackup(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	e := &Engine{BackupDir: func() (string, error) { t.Fatal("a cancelled run must not create a backup"); return "", nil }}
	o := &Options{Root: root, Offline: true}
	p := ecDiscover(t, e, o)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &Report{}
	err := e.prepareWrites(ctx, p, o, r, []candidate{{doc: p.docs[0], content: []byte("{}\n")}})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, r.Backup)
}

func TestEcGitWarningsReportUnreadableStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	root := newProject(t)
	path := projectFile(t, root, "x.yaml", "a: 1\n")
	output, err := exec.Command("git", "-C", root, "init", "--quiet").CombinedOutput()
	require.NoError(t, err, string(output))
	// A corrupt index keeps the repository discoverable but makes status fail.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "index"), []byte("corrupt index"), 0o600))

	e := &Engine{}
	r := &Report{}
	e.gitWarnings(context.Background(), &project{root: root}, []candidate{{doc: &document{path: path}}}, r)
	require.Len(t, r.Findings, 1)
	require.Equal(t, "recovery", r.Findings[0].RuleID)
	require.Equal(t, "x.yaml", r.Findings[0].File)
	require.Equal(t, "Unable to inspect Git status for x.yaml", r.Findings[0].Message)
	require.Equal(t, statusManual, r.Findings[0].Status)
}

// A rejected candidate keeps its verification decision across the post-write recheck, so a
// repair that was refused is not silently re-offered as available.
func TestEcRecheckRetainsRejectedDecisions(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: stack-schema.json\n"+ecLegacyReconfigure)
	projectFile(t, root, "stack-schema.json", `{"properties":{"hooks":{"properties":{"audit":{"properties":{"events":{"items":{"const":"before-terraform-plan"}}}}}}}}`)
	stack := "hooks:\n  audit:\n    events: [before-terraform-plan]\n    command: echo\n"
	stackPath := projectFile(t, root, "stacks/dev.yaml", stack)

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)

	hooks := ecFindings(&r, ruleHookEvent)
	require.Len(t, hooks, 1)
	require.Equal(t, statusVerificationFailed, hooks[0].Status)
	require.False(t, hooks[0].Fixable)
	require.Contains(t, hooks[0].Reason, "new schema error")

	reconfigure := ecFindings(&r, ruleTerraformReconfigure)
	require.Len(t, reconfigure, 1)
	require.Equal(t, statusFixed, reconfigure[0].Status)

	content, err := os.ReadFile(stackPath)
	require.NoError(t, err)
	require.Equal(t, stack, string(content))
	require.NotNil(t, ecCheck(&r, "semantic-verification"))
}

func TestEcRecheckStopsWhenSourcesChangeAfterWriting(t *testing.T) {
	root := newProject(t)
	path := projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	e := testEngine(t)
	e.Observe = func(event Event) {
		if event.Phase == "Recheck" && !event.Done {
			projectFile(t, root, "atmos.d/late.yaml", "{}\n")
		}
	}
	r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.ErrorIs(t, err, ErrChanged)
	// The write itself already happened and is reported for recovery.
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)
	require.NotEmpty(t, r.Backup)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.NotContains(t, string(content), "init_run_reconfigure: false")
}

var errEcInjectedRead = errors.New("injected read failure")

// ecFailingReads fails every read after the first `skip` once armed.
type ecFailingReads struct {
	filesystem.FileSystem
	armed bool
	skip  int
}

func (f *ecFailingReads) ReadFile(path string) ([]byte, error) {
	if f.armed {
		if f.skip == 0 {
			return nil, errEcInjectedRead
		}
		f.skip--
	}
	return f.FileSystem.ReadFile(path)
}

func TestEcRecheckPropagatesRerunFailures(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	reads := &ecFailingReads{FileSystem: filesystem.NewOSFileSystem(), skip: 2}
	e := testEngine(t)
	e.FS = reads
	// The source-drift check reads the single document twice, so the third read belongs
	// to the diagnostic rerun.
	e.Observe = func(event Event) {
		if event.Phase == "Recheck" && !event.Done {
			reads.armed = true
		}
	}
	r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.ErrorIs(t, err, errEcInjectedRead)
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)
	require.Equal(t, 2, r.ExitCode("none"))
}

func TestEcUnverifiableProjectsNeverWrite(t *testing.T) {
	for _, tt := range []struct {
		name       string
		config     string
		profiles   []string
		wantReason string
		wantCheck  bool
	}{
		// Detected during discovery: the repair is never offered as fixable.
		{"profiles", ecLegacyReconfigure, []string{"ci"}, "Not fixed automatically because Configuration is layered: profiles/ci/atmos.yaml is also merged at runtime", false},
		// Detected after planning: the planned repair is withdrawn and the gap is reported.
		{"remote custom schema", ecLegacyReconfigure + "schemas:\n  custom:\n    schema: https://example.invalid/schema.json\n    matches: [atmos.yaml]\n", nil, "Not fixed automatically because schema custom: Remote schema cannot be included in the static source snapshot", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			path := projectFile(t, root, "atmos.yaml", tt.config)
			if len(tt.profiles) > 0 {
				projectFile(t, root, "profiles/ci/atmos.yaml", "logs:\n  level: Debug\n")
			}
			e := &Engine{BackupDir: func() (string, error) { t.Fatal("an unverifiable project must not be backed up"); return "", nil }}
			r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Profiles: tt.profiles, Only: []string{categoryDeprecations}})
			require.NoError(t, err)
			require.Empty(t, r.ChangedFiles)
			found := ecFindings(&r, ruleTerraformReconfigure)
			require.Len(t, found, 1)
			require.Equal(t, statusVerificationIncomplete, found[0].Status)
			require.False(t, found[0].Fixable)
			require.Equal(t, tt.wantReason, found[0].Reason)
			check := ecCheck(&r, "semantic-verification")
			require.Equal(t, tt.wantCheck, check != nil)
			if tt.wantCheck {
				require.Equal(t, "incomplete", check.Status)
			}
			content, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, tt.config, string(content))
		})
	}
}

func TestEcDryRunLeavesEveryEligibleRepairUnapplied(t *testing.T) {
	root := newProject(t)
	path := projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	e := &Engine{BackupDir: func() (string, error) { t.Fatal("a dry run must not back up"); return "", nil }}
	r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{categoryDeprecations}})
	require.NoError(t, err)
	require.True(t, r.DryRun)
	require.Empty(t, r.ChangedFiles)
	found := ecFindings(&r, ruleTerraformReconfigure)
	require.Len(t, found, 1)
	require.Equal(t, "Candidate verified; dry run left the source unchanged", found[0].Reason)
	require.NotEqual(t, statusFixed, found[0].Status)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, ecLegacyReconfigure, string(content))
}

func TestEcApplyStopsWhenASourceChangesBetweenWrites(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	for _, name := range []string{"a", "b"} {
		projectFile(t, root, "stacks/"+name+".yaml", "hooks:\n  example:\n    events: [before-terraform-apply]\n    command: echo\n")
	}
	e := testEngine(t)
	applied := 0
	e.Observe = func(event Event) {
		if event.Phase == "Apply" && event.Done {
			applied++
			// Modify the second stack after the first write completes.
			projectFile(t, root, "stacks/b.yaml", "# edited concurrently\nhooks:\n  example:\n    events: [before-terraform-apply]\n    command: echo\n")
		}
	}
	r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.ErrorIs(t, err, ErrChanged)
	require.Equal(t, 1, applied)
	require.Equal(t, []string{"stacks/a.yaml"}, r.ChangedFiles)
	content, readErr := os.ReadFile(filepath.Join(root, "stacks", "b.yaml"))
	require.NoError(t, readErr)
	require.Contains(t, string(content), "# edited concurrently")
	require.Contains(t, string(content), "before-terraform-apply")
}

func TestEcReportExitCodeAndSorting(t *testing.T) {
	finding := func(severity validation.Severity, status string) Finding {
		return Finding{Severity: severity, Status: status}
	}
	for _, tt := range []struct {
		name      string
		report    Report
		threshold string
		want      int
	}{
		{"no findings", Report{}, "error", 0},
		{"error at error threshold", Report{Findings: []Finding{finding(validation.SeverityError, statusManual)}}, "error", 1},
		{"warning below error threshold", Report{Findings: []Finding{finding(validation.SeverityWarning, statusManual)}}, "error", 0},
		{"warning at warning threshold", Report{Findings: []Finding{finding(validation.SeverityWarning, statusManual)}}, "warning", 1},
		{"error at warning threshold", Report{Findings: []Finding{finding(validation.SeverityError, statusManual)}}, "warning", 1},
		{"none threshold ignores findings", Report{Findings: []Finding{finding(validation.SeverityError, statusManual)}}, "none", 0},
		{"fixed findings are excluded", Report{Findings: []Finding{finding(validation.SeverityError, statusFixed), finding(validation.SeverityWarning, statusFixed)}}, "warning", 0},
		{"a later unfixed finding still counts", Report{Findings: []Finding{finding(validation.SeverityError, statusFixed), finding(validation.SeverityError, statusVerificationFailed)}}, "error", 1},
		{"operational errors cannot be suppressed", Report{OperationalError: "boom"}, "none", 2},
		{"operational errors outrank findings", Report{OperationalError: "boom", Findings: []Finding{finding(validation.SeverityError, statusManual)}}, "error", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.report.ExitCode(tt.threshold))
		})
	}

	t.Run("findings sort by file, line, rule, then path and keep ties stable", func(t *testing.T) {
		r := Report{Findings: []Finding{
			{File: "b.yaml", Message: "last"},
			{File: "a.yaml", Line: 2, Message: "line two"},
			{File: "a.yaml", Line: 1, RuleID: "z", Message: "rule z"},
			{File: "a.yaml", Line: 1, RuleID: "y", Path: "p2", Message: "path p2"},
			{File: "a.yaml", Line: 1, RuleID: "y", Path: "p1", Message: "path p1"},
			{File: "a.yaml", Line: 1, RuleID: "y", Path: "p1", Message: "tie second"},
		}}
		// The tie shares every sort key with "path p1" and must keep insertion order after it.
		r.sort()
		var messages []string
		for i := range r.Findings {
			messages = append(messages, r.Findings[i].Message)
		}
		require.Equal(t, []string{"path p1", "tie second", "path p2", "rule z", "line two", "last"}, messages)
	})
}

func TestEcValidationReportComposesMessages(t *testing.T) {
	r := Report{
		OperationalError: "backup unavailable",
		Findings: []Finding{
			{RuleID: "r1", Severity: validation.SeverityWarning, File: "a.yaml", Line: 3, Column: 5, Message: "base", Reason: "because", Remediation: "do this", Status: statusManual},
			{RuleID: "r2", Severity: validation.SeverityError, File: "b.yaml", Message: "only remediation", Remediation: "fix it", Status: statusManual},
			{RuleID: "r3", Severity: validation.SeverityError, File: "c.yaml", Message: "reason only", Reason: "why", Status: statusVerificationFailed},
			{RuleID: "r4", Severity: validation.SeverityError, File: "d.yaml", Message: "already repaired", Status: statusFixed},
		},
	}
	report := r.ValidationReport()
	require.Len(t, report.Diagnostics, 4)

	operational := report.Diagnostics[0]
	require.Equal(t, "doctor", operational.Source)
	require.Equal(t, "operational", operational.RuleID)
	require.Equal(t, validation.SeverityError, operational.Severity)
	require.Equal(t, "backup unavailable", operational.Message)

	first := report.Diagnostics[1]
	require.Equal(t, "base: because. do this", first.Message)
	require.Equal(t, "r1", first.RuleID)
	require.Equal(t, validation.SeverityWarning, first.Severity)
	require.Equal(t, "a.yaml", first.File)
	require.Equal(t, 3, first.Line)
	require.Equal(t, 5, first.Column)

	require.Equal(t, "only remediation. fix it", report.Diagnostics[2].Message)
	require.Equal(t, "reason only: why", report.Diagnostics[3].Message)

	t.Run("no operational diagnostic without an operational error", func(t *testing.T) {
		clean := Report{Findings: []Finding{{RuleID: "r", Message: "m", Status: statusManual}}}
		diagnostics := clean.ValidationReport().Diagnostics
		require.Len(t, diagnostics, 1)
		require.Equal(t, "m", diagnostics[0].Message)
	})
}
