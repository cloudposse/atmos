package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

func projectFile(t *testing.T, root, path, content string) string {
	t.Helper()
	path = filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o640))
	return path
}

func newProject(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return root
}

// requirePOSIXMode asserts permission bits where the platform enforces them.
// Windows reports synthetic modes (0666/0777) that do not reflect the requested bits.
func requirePOSIXMode(t *testing.T, want, got fs.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	require.Equal(t, want, got)
}

// gitCommand builds a git invocation that never signs, so fixtures commit on machines whose
// global configuration requires an interactive signing agent.
func gitCommand(dir string, args ...string) *exec.Cmd {
	prefix := []string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com", "-C", dir}
	return exec.Command("git", append(prefix, args...)...)
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	return &Engine{BackupDir: func() (string, error) { return t.TempDir(), nil }}
}

func TestFixRuntimeContractsRecoveryAndIdempotence(t *testing.T) {
	for _, value := range []string{"component", "stack"} {
		for _, crlf := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/crlf=%v", value, crlf), func(t *testing.T) {
				root := newProject(t)
				original := "# preserve me\ncommands:\n  - name: sample\n    arguments:\n      - name: target\n        type: " + value + " # semantic\n    flags:\n      - name: target\n        type: string\n        semantic_type: " + value + "\n    steps: [echo hello]\ncomponents:\n  terraform:\n    init_run_reconfigure: false\n"
				if crlf {
					original = strings.ReplaceAll(original, "\n", "\r\n")
				}
				path := projectFile(t, root, "atmos.yaml", original)
				e := testEngine(t)
				report, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
				require.NoError(t, err)
				require.Len(t, report.ChangedFiles, 1, "%+v", report.Findings)
				updated, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Contains(t, string(updated), "provides: "+value)
				require.Contains(t, string(updated), "type: string")
				require.Contains(t, string(updated), "reconfigure: never")
				if crlf {
					require.NotContains(t, string(bytes.ReplaceAll(updated, []byte("\r\n"), nil)), "\n")
				}
				info, err := os.Stat(path)
				require.NoError(t, err)
				requirePOSIXMode(t, fs.FileMode(0o640), info.Mode().Perm())
				manifest, err := os.ReadFile(filepath.Join(report.Backup, "manifest.json"))
				require.NoError(t, err)
				var entries []RecoveryEntry
				require.NoError(t, json.Unmarshal(manifest, &entries))
				require.Len(t, entries, 1)
				backup, err := os.ReadFile(filepath.Join(report.Backup, entries[0].Copy))
				require.NoError(t, err)
				require.Equal(t, original, string(backup))
				require.Equal(t, digest(updated), entries[0].After)
				info, err = os.Stat(report.Backup)
				require.NoError(t, err)
				requirePOSIXMode(t, fs.FileMode(0o700), info.Mode().Perm())
				second, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
				require.NoError(t, err)
				require.Empty(t, second.ChangedFiles)
				require.Empty(t, second.Backup)
			})
		}
	}
}

func TestDryRunAndIncompleteInputsNeverWrite(t *testing.T) {
	for _, tt := range []struct {
		name, extra string
		dry         bool
	}{{"dry", "", true}, {"import", "import: [other.yaml]\n", false}, {"conflict", "", false}} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			original := "commands:\n  - name: sample\n    arguments:\n      - name: target\n        type: stack\n" + tt.extra
			if tt.name == "conflict" {
				original += "        provides: component\n"
			}
			if tt.name == "import" {
				projectFile(t, root, "other.yaml", "vars: {}\n")
			}
			path := projectFile(t, root, "atmos.yaml", original)
			e := &Engine{BackupDir: func() (string, error) { t.Fatal("backup called"); return "", nil }}
			report, err := e.Run(context.Background(), Options{Root: root, Fix: true, DryRun: tt.dry, Offline: true, Only: []string{"deprecations"}})
			require.NoError(t, err)
			require.Empty(t, report.ChangedFiles)
			updated, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, string(updated))
		})
	}
}

func TestVerifierRejectsMaterialChanges(t *testing.T) {
	before := []byte("commands:\n  - name: run\n    arguments:\n      - name: target\n        type: stack\nvars:\n  untouched: true\n")
	d := &document{path: "atmos.yaml", kind: "config", content: before, schema: configSchema}
	var err error
	d.node, err = parse(before)
	require.NoError(t, err)
	r := Report{}
	edits, err := scanDocument(d, &project{complete: true, configCount: 1}, &Options{}, &r, validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{}))
	require.NoError(t, err)
	require.Len(t, edits, 1)
	after, err := applyEdit(before, &edits[0])
	require.NoError(t, err)
	require.NoError(t, verifyDocument(before, after, edits))
	for _, candidate := range [][]byte{bytes.ReplaceAll(after, []byte("provides: stack"), []byte("provides: component")), bytes.ReplaceAll(after, []byte("untouched: true"), []byte("untouched: false")), bytes.ReplaceAll(after, []byte("untouched: true"), []byte("untouched: 'true'"))} {
		require.ErrorIs(t, verifyDocument(before, candidate, edits), ErrVerification)
	}
}

func TestMalformedYAMLProducesFindings(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "commands: [\n")
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true})
	require.NoError(t, err)
	require.Equal(t, 1, r.ExitCode("error"))
	require.Equal(t, 0, r.ExitCode("none"))
}

type failingFS struct {
	filesystem.FileSystem
	writes, failAt int
	cancel         context.CancelFunc
}

func (f *failingFS) WriteFileAtomic(path string, b []byte, mode fs.FileMode) error {
	f.writes++
	if f.writes == f.failAt {
		return errors.New("injected write failure")
	}
	err := f.FileSystem.WriteFileAtomic(path, b, mode)
	if f.cancel != nil {
		f.cancel()
	}
	return err
}

func TestBackupAndConcurrentEditFailures(t *testing.T) {
	for _, mode := range []string{"backup", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			root := newProject(t)
			original := "components:\n  terraform:\n    init_run_reconfigure: true\n"
			path := projectFile(t, root, "atmos.yaml", original)
			e := testEngine(t)
			if mode == "backup" {
				e.BackupDir = func() (string, error) { return "", errors.New("backup unavailable") }
			} else {
				e.Observe = func(event Event) {
					if event.Phase == "Back up" && event.Done {
						require.NoError(t, os.WriteFile(path, []byte("# concurrent\n"+original), 0o640))
					}
				}
			}
			r, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
			require.Error(t, err)
			require.Empty(t, r.ChangedFiles)
			require.Equal(t, 2, r.ExitCode("none"))
			b, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(b), "init_run_reconfigure")
		})
	}
}

func TestPartialApplicationAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			for _, name := range []string{"a", "b"} {
				projectFile(t, root, "stacks/"+name+".yaml", "hooks:\n  example:\n    events: [before-terraform-apply]\n    command: echo\n")
			}
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			f := &failingFS{FileSystem: filesystem.NewOSFileSystem(), failAt: 2}
			if cancel {
				f.cancel = stop
			}
			e := testEngine(t)
			e.FS = f
			r, err := e.Run(ctx, Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
			require.Error(t, err)
			require.Len(t, r.ChangedFiles, 1, "%+v", r)
			require.NotEmpty(t, r.Backup)
			b, err := os.ReadFile(filepath.Join(root, "stacks", "a.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(b), "before.terraform.apply")
			b, err = os.ReadFile(filepath.Join(root, "stacks", "b.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(b), "before-terraform-apply")
		})
	}
}

type repositoryStub struct {
	calls  []string
	health RepositoryHealth
}

func (s *repositoryStub) Check(_ context.Context, uri string) RepositoryHealth {
	s.calls = append(s.calls, uri)
	return s.health
}

func TestRepositoryHealthIncludesVendorAndSourceProvisioning(t *testing.T) {
	for _, status := range []string{"healthy", "archived", "missing", "unavailable", "inconclusive", "unsupported"} {
		t.Run(status, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "vendor.yaml", "spec:\n  sources:\n    - source: git::https://github.com/example/repo.git//modules/vpc?ref=v1\n")
			projectFile(t, root, "stacks/test.yaml", "components:\n  terraform:\n    vpc:\n      source:\n        uri: git@github.com:example/repo.git\n")
			s := &repositoryStub{health: RepositoryHealth{status, status}}
			e := testEngine(t)
			e.Repositories = s
			r, err := e.Run(context.Background(), Options{Root: root, Only: []string{"sources"}})
			require.NoError(t, err)
			require.Equal(t, []string{"https://github.com/example/repo"}, s.calls)
			if status == "healthy" {
				require.Empty(t, r.Findings)
			} else {
				require.Len(t, r.Findings, 2)
				require.Equal(t, validation.SeverityWarning, r.Findings[0].Severity)
				require.False(t, r.Findings[0].Fixable)
			}
		})
	}
}

func TestGitWarningsOnlyConcernEditedFiles(t *testing.T) {
	for _, state := range []string{"clean", "unrelated", "unstaged", "staged", "untracked", "ignored", "no-git"} {
		t.Run(state, func(t *testing.T) {
			root := newProject(t)
			git := func(args ...string) string {
				t.Helper()
				out, err := gitCommand(root, args...).CombinedOutput()
				require.NoError(t, err, string(out))
				return string(out)
			}
			original := "components:\n  terraform:\n    init_run_reconfigure: false\n"
			projectFile(t, root, "atmos.yaml", original)
			//nolint:nestif // This fixture intentionally exercises distinct Git states.
			if state != "no-git" {
				git("init", "--quiet")
				git("config", "user.email", "doctor@example.invalid")
				git("config", "user.name", "Doctor Test")
				projectFile(t, root, "unrelated.txt", "original\n")
				if state == "ignored" {
					projectFile(t, root, ".gitignore", "atmos.yaml\n")
				}
				if state != "untracked" && state != "ignored" {
					git("add", "atmos.yaml")
				}
				git("add", "unrelated.txt")
				git("commit", "--quiet", "-m", "fixture")
				if state == "unrelated" {
					projectFile(t, root, "unrelated.txt", "changed\n")
				}
				if state == "unstaged" || state == "staged" {
					projectFile(t, root, "atmos.yaml", "# existing change\n"+original)
				}
				if state == "staged" {
					git("add", "atmos.yaml")
				}
			}
			indexBefore := ""
			if state != "no-git" {
				indexBefore = git("ls-files", "--stage")
			}
			r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
			require.NoError(t, err)
			require.Len(t, r.ChangedFiles, 1)
			warnings := 0
			for _, f := range r.Findings {
				if f.RuleID == "recovery" {
					warnings++
					require.Equal(t, validation.SeverityNotice, f.Severity, "recovery guidance must not trip --fail-on=warning")
					require.Empty(t, f.File)
				}
			}
			require.Zero(t, r.ExitCode("warning"))
			if state == "clean" || state == "unrelated" {
				require.Zero(t, warnings)
			} else {
				require.Equal(t, 1, warnings)
			}
			if state != "no-git" {
				require.Equal(t, indexBefore, git("ls-files", "--stage"), "doctor must not change the Git index")
			}
		})
	}
}

func TestNewSourceDuringBackupStopsWrites(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: true\n")
	e := testEngine(t)
	e.Observe = func(event Event) {
		if event.Phase == "Back up" && event.Done {
			projectFile(t, root, "atmos.d/new.yaml", "components:\n  terraform:\n    init:\n      reconfigure: always\n")
		}
	}
	r, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.ErrorIs(t, err, ErrChanged)
	require.Empty(t, r.ChangedFiles)
}

func TestCustomSchemaRejectsCandidate(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "schemas:\n  custom:\n    schema: schema.json\n    matches: [atmos.yaml]\ncomponents:\n  terraform:\n    init_run_reconfigure: false\n")
	projectFile(t, root, "schema.json", `{"type":"object","properties":{"components":{"properties":{"terraform":{"required":["init_run_reconfigure"]}}}}}`)
	e := testEngine(t)
	r, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Empty(t, r.ChangedFiles)
	require.Empty(t, r.Backup)
	found := false
	for _, f := range r.Findings {
		found = found || f.Status == "verification-failed"
	}
	require.True(t, found)
}

func TestDependencyMigrationNeedsProductionValidator(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	content := "components:\n  terraform:\n    app:\n      settings:\n        depends_on:\n          - component: missing\n"
	path := projectFile(t, root, "stacks/dev.yaml", content)
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Empty(t, r.ChangedFiles)
	found := false
	for _, f := range r.Findings {
		if strings.Contains(f.Path, "depends_on") {
			found = true
			require.False(t, f.Fixable)
			require.Contains(t, f.Reason, "missing targets")
		}
	}
	require.True(t, found)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(after))
}

func FuzzCommandMigrationContract(f *testing.F) {
	f.Add("target", "component", "keep", true)
	f.Add("stack-name", "stack", "001", false)
	f.Fuzz(func(t *testing.T, name, value, extra string, flag bool) {
		if value != "stack" && value != "component" {
			t.Skipf("Only recognized semantic values belong to this migration contract")
		}
		old, kind := "type", "arguments"
		if flag {
			old, kind = "semantic_type", "flags"
		}
		// JSON scalar quoting is valid YAML and handles arbitrary generated strings.
		quote := func(s string) string { b, _ := json.Marshal(s); return string(b) }
		before := []byte("commands:\n  - name: run\n    " + kind + ":\n      - name: " + quote(name) + "\n        " + old + ": " + value + "\nvars:\n  untouched: " + quote(extra) + "\n")
		d := &document{path: "atmos.yaml"}
		change := edit{doc: d, path: []string{"commands", "0", kind, "0"}, kind: "command-provides", old: old, value: value}
		after, err := applyEdit(before, &change)
		require.NoError(t, err)
		require.NoError(t, verifyDocument(before, after, []edit{change}))
		other := "stack"
		if value == "stack" {
			other = "component"
		}
		bad := bytes.Replace(after, []byte("provides: "+value), []byte("provides: "+other), 1)
		require.Error(t, verifyDocument(before, bad, []edit{change}))
	})
}

func TestHookMigrationProductionConsumerContract(t *testing.T) {
	events := []hooks.HookEvent{hooks.BeforeTerraformInit, hooks.AfterTerraformInit, hooks.BeforeTerraformPlan, hooks.AfterTerraformPlan, hooks.BeforeTerraformApply, hooks.AfterTerraformApply, hooks.BeforeTerraformDeploy, hooks.AfterTerraformDeploy, hooks.BeforeTerraformOutput, hooks.AfterTerraformOutput, hooks.BeforeTerraformTest, hooks.AfterTerraformTest, hooks.BeforeHelmfileApply, hooks.AfterHelmfileApply}
	for _, event := range events {
		t.Run(string(event), func(t *testing.T) {
			before := []byte("hooks:\n  audit:\n    events: [" + strings.ReplaceAll(string(event), ".", "-") + "]\n    command: echo\n")
			node, err := parse(before)
			require.NoError(t, err)
			d := &document{path: "stack.yaml", kind: "stack", node: node, content: before}
			report := Report{}
			edits, err := scanDocument(d, &project{complete: true, configCount: 1}, &Options{}, &report, validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{}))
			require.NoError(t, err)
			require.Len(t, edits, 1)
			after, err := applyEdit(before, &edits[0])
			require.NoError(t, err)
			require.NoError(t, verifyDocument(before, after, edits))
			originalHook := hooks.Hook{Events: []string{strings.ReplaceAll(string(event), ".", "-")}}
			migratedHook := hooks.Hook{Events: []string{edits[0].value}}
			for _, observed := range append(events, hooks.HookEvent("unrecognized.event")) {
				require.Equal(t, originalHook.MatchesEvent(observed), migratedHook.MatchesEvent(observed))
			}
			materiallyChanged := bytes.ReplaceAll(after, []byte(edits[0].value), []byte("unrecognized.event"))
			require.ErrorIs(t, verifyDocument(before, materiallyChanged, edits), ErrVerification)
		})
	}
}

func TestUncapturedSchemaPreventsRepairs(t *testing.T) {
	for _, only := range [][]string{{"deprecations"}, {"command-provides"}, {}} {
		for _, source := range []string{"https://example.invalid/schema.json", "local.json"} {
			t.Run(fmt.Sprint(only, source), func(t *testing.T) {
				root := newProject(t)
				original := "commands:\n  - name: demo\n    arguments:\n      - name: target\n        type: stack\nschemas:\n  custom:\n    schema: " + source + "\n    matches: ['atmos.yaml']\n"
				path := projectFile(t, root, "atmos.yaml", original)
				projectFile(t, root, "local.json", `{"$ref":"https://example.invalid/schema.json"}`)
				engine := &Engine{BackupDir: func() (string, error) { t.Fatal("uncaptured schema must prevent writes"); return "", nil }}
				report, err := engine.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: only})
				require.NoError(t, err)
				require.Empty(t, report.ChangedFiles)
				updated, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, original, string(updated))
				require.Contains(t, fmt.Sprint(report.Checks), "incomplete")
			})
		}
	}
}

func TestSourcePathsExcludeArbitraryVariables(t *testing.T) {
	require.True(t, sourcePath("stack", []string{"components", "terraform", "app", "source", "uri"}))
	require.True(t, sourcePath("stack", []string{"components", "terraform", "app", "source"}))
	require.False(t, sourcePath("stack", []string{"components", "terraform", "app", "vars", "source"}))
	require.False(t, sourcePath("stack", []string{"components", "terraform", "app", "vars", "source", "uri"}))
}

func TestReconfigureBooleanContracts(t *testing.T) {
	for _, value := range []string{"true", "True", "false", "False"} {
		t.Run(value, func(t *testing.T) {
			root := newProject(t)
			path := projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: "+value+"\n")
			report, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
			require.NoError(t, err)
			require.Len(t, report.ChangedFiles, 1)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			var config schema.AtmosConfiguration
			require.NoError(t, yaml.Unmarshal(content, &config))
			original := schema.Terraform{InitRunReconfigure: strings.EqualFold(value, "true")}
			require.Equal(t, original.EffectiveInitReconfigure(), config.Components.Terraform.EffectiveInitReconfigure())
		})
	}
}

func TestAnchorsPreservedAndSymlinksUnchanged(t *testing.T) {
	for _, kind := range []string{"anchor", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := newProject(t)
			original := "components:\n  terraform:\n    init_run_reconfigure: false\n"
			if kind == "anchor" {
				original += "vars:\n  first: &shared {value: true}\n  second: *shared\n"
			}
			path := projectFile(t, root, "atmos.yaml", original)
			if kind == "symlink" {
				target := filepath.Join(root, "original.yaml")
				require.NoError(t, os.Rename(path, target))
				require.NoError(t, os.Symlink(target, path))
			}
			report, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
			require.NoError(t, err)
			if kind == "symlink" {
				require.Empty(t, report.ChangedFiles)
				require.Empty(t, report.Backup)
			} else {
				require.Len(t, report.ChangedFiles, 1)
			}
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			if kind == "symlink" {
				require.Equal(t, original, string(content))
			} else {
				require.Contains(t, string(content), "&shared")
				require.Contains(t, string(content), "*shared")
				var before, after map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(original), &before))
				require.NoError(t, yaml.Unmarshal(content, &after))
				require.Equal(t, before["vars"], after["vars"])
			}
		})
	}
}

func TestGitWorktreeRecovery(t *testing.T) {
	repository := newProject(t)
	git := func(args ...string) {
		t.Helper()
		output, err := gitCommand(repository, args...).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	projectFile(t, repository, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: false\n")
	git("init", "--quiet")
	git("config", "user.email", "doctor@example.invalid")
	git("config", "user.name", "Doctor Test")
	git("add", "atmos.yaml")
	git("commit", "--quiet", "-m", "fixture")
	root := newProject(t)
	git("worktree", "add", "--detach", root)
	report, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Len(t, report.ChangedFiles, 1)
	require.NotEmpty(t, report.Backup)
	for _, finding := range report.Findings {
		require.NotEqual(t, "recovery", finding.RuleID)
	}
}

func TestGitUnavailableWarnsWithoutPreventingBackup(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: false\n")
	t.Setenv("PATH", t.TempDir())
	report, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Len(t, report.ChangedFiles, 1)
	require.NotEmpty(t, report.Backup)
	require.True(t, checkHasFindings(&report, "recovery"))
	for _, f := range report.Findings {
		if f.RuleID == "recovery" {
			require.Equal(t, validation.SeverityNotice, f.Severity)
		}
	}
	require.Zero(t, report.ExitCode("warning"))
}

func TestConfiguredStackSchemaRejectsRepair(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: stack-schema.json\n")
	original := "hooks:\n  audit:\n    events: [before-terraform-plan]\n    command: echo\n"
	path := projectFile(t, root, "stacks/dev.yaml", original)
	projectFile(t, root, "stack-schema.json", `{"properties":{"hooks":{"properties":{"audit":{"properties":{"events":{"items":{"const":"before-terraform-plan"}}}}}}}}`)
	report, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"hook-event"}})
	require.NoError(t, err)
	require.Empty(t, report.ChangedFiles)
	require.Empty(t, report.Backup)
	require.Contains(t, fmt.Sprint(report.Findings), "verification-failed")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(content))
}
