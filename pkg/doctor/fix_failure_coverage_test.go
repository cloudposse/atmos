package doctor

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

const ecHookStack = "hooks:\n  example:\n    events: [before-terraform-apply]\n    command: echo\n"

// ecFailingStat fails every Stat after the first `skip` once armed.
type ecFailingStat struct {
	filesystem.FileSystem
	armed bool
	skip  int
}

func (f *ecFailingStat) Stat(path string) (fs.FileInfo, error) {
	if f.armed {
		if f.skip == 0 {
			return nil, errEcInjectedRead
		}
		f.skip--
	}
	return f.FileSystem.Stat(path)
}

// Every failure between verification and the first write must leave the project untouched.
func TestEcLateFailuresNeverLeaveWrittenFiles(t *testing.T) {
	t.Run("a captured schema disappearing before repairs", func(t *testing.T) {
		root := newProject(t)
		original := ecLegacyReconfigure + "schemas:\n  custom:\n    schema: schema.json\n    matches: [atmos.yaml]\n"
		path := projectFile(t, root, "atmos.yaml", original)
		schemaPath := projectFile(t, root, "schema.json", `{"type":"object"}`)
		projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: github.com/example/repo.git\n")
		e := testEngine(t)
		e.Observe = func(event Event) {
			if event.Phase == "Check" && event.Label == "Repository metadata" && event.Done {
				require.NoError(t, os.Remove(schemaPath))
			}
		}
		r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations, "sources"}})
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Empty(t, r.ChangedFiles)
		require.Empty(t, r.Backup)
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, original, string(content))
	})

	t.Run("cancellation while staging", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		projectFile(t, root, "stacks/a.yaml", ecHookStack)
		projectFile(t, root, "stacks/b.yaml", ecHookStack)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		e := testEngine(t)
		e.Observe = func(event Event) {
			if event.Phase == "Stage fixes" && !event.Done {
				cancel()
			}
		}
		r, err := e.Run(ctx, Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, r.ChangedFiles)
		require.Empty(t, r.Backup)
	})

	t.Run("cancellation while verifying the candidate project", func(t *testing.T) {
		root := newProject(t)
		path := projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		e := testEngine(t)
		e.Observe = func(event Event) {
			if event.Phase == "Verify" && event.Label == "Complete candidate project" && !event.Done {
				cancel()
			}
		}
		r, err := e.Run(ctx, Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, r.ChangedFiles)
		require.Empty(t, r.Backup)
		for _, f := range ecFindings(&r, ruleTerraformReconfigure) {
			require.NotEqual(t, statusVerificationFailed, f.Status, "cancellation is not a verification verdict")
		}
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, ecLegacyReconfigure, string(content))
	})

	t.Run("a source edited after verification", func(t *testing.T) {
		root := newProject(t)
		path := projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
		const edited = "# edited after verification\n" + ecLegacyReconfigure
		e := testEngine(t)
		e.Observe = func(event Event) {
			if event.Phase == "Verify" && event.Label == "Complete candidate project" && event.Done {
				require.NoError(t, os.WriteFile(path, []byte(edited), 0o640))
			}
		}
		r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
		require.ErrorIs(t, err, ErrChanged)
		require.Empty(t, r.ChangedFiles)
		require.Empty(t, r.Backup)
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, edited, string(content))
	})

	t.Run("a read failure while applying", func(t *testing.T) {
		root := newProject(t)
		path := projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
		reads := &ecFailingReads{FileSystem: filesystem.NewOSFileSystem()}
		e := testEngine(t)
		e.FS = reads
		e.Observe = func(event Event) {
			if event.Phase == "Apply" && !event.Done {
				reads.armed = true
			}
		}
		r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
		require.ErrorIs(t, err, errEcInjectedRead)
		require.Empty(t, r.ChangedFiles)
		require.NotEmpty(t, r.Backup, "the backup precedes the guarded write")
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, ecLegacyReconfigure, string(content))
	})
}

func TestEcUnchangedSurfacesFilesystemFailures(t *testing.T) {
	setup := func(t *testing.T, fsys filesystem.FileSystem) (*Engine, *Options, *project) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		e := testEngine(t)
		e.FS = fsys
		o := &Options{Root: root, Offline: true}
		return e, o, ecDiscover(t, e, o)
	}

	t.Run("read failure while comparing a document", func(t *testing.T) {
		reads := &ecFailingReads{FileSystem: filesystem.NewOSFileSystem(), skip: 1}
		e, o, p := setup(t, reads)
		reads.armed = true
		require.ErrorIs(t, e.unchanged(context.Background(), p, o), errEcInjectedRead)
	})

	t.Run("stat failure while comparing a document", func(t *testing.T) {
		stats := &ecFailingStat{FileSystem: filesystem.NewOSFileSystem(), skip: 1}
		e, o, p := setup(t, stats)
		stats.armed = true
		require.ErrorIs(t, e.unchanged(context.Background(), p, o), errEcInjectedRead)
	})

	t.Run("removed custom schema while comparing", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "schemas:\n  custom:\n    schema: schema.json\n    matches: [data.yaml]\n")
		schemaPath := projectFile(t, root, "schema.json", `{"type":"object"}`)
		projectFile(t, root, "data.yaml", "name: value\n")
		e, o := testEngine(t), &Options{Root: root, Offline: true}
		p := ecDiscover(t, e, o)
		require.NoError(t, e.customSchemas(context.Background(), p, o, &Report{}, validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})))
		require.NoError(t, os.Remove(schemaPath))
		require.ErrorIs(t, e.unchanged(context.Background(), p, o), fs.ErrNotExist)
	})
}

func TestEcRepairsProceedDespiteUnrelatedExistingErrors(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	const invalid = "components: 5\n"
	projectFile(t, root, "stacks/invalid.yaml", invalid)
	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations, ruleSchema}})
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)
	var invalidFiles []string
	for _, f := range ecFindings(&r, ruleSchema) {
		if f.Severity == validation.SeverityError {
			invalidFiles = append(invalidFiles, f.File)
		}
	}
	require.NotEmpty(t, invalidFiles, "the pre-existing schema error stays reported")
	for _, file := range invalidFiles {
		require.Equal(t, "stacks/invalid.yaml", file)
	}
	content, err := os.ReadFile(filepath.Join(root, "stacks", "invalid.yaml"))
	require.NoError(t, err)
	require.Equal(t, invalid, string(content))
}

func TestEcDefaultBackupLocationFollowsTheDataDirectory(t *testing.T) {
	data := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", data)
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	r, err := (&Engine{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.NoError(t, err)
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)
	require.Equal(t, filepath.Join(data, "atmos", "doctor", "backups"), filepath.Dir(r.Backup))
	original, err := os.ReadFile(filepath.Join(r.Backup, "000000.original"))
	require.NoError(t, err)
	require.Equal(t, ecLegacyReconfigure, string(original))
}

func TestEcUnreadableAndInvalidSourceLocationsAreOperationalErrors(t *testing.T) {
	t.Run("vendor base path below a regular file", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows reports a missing path instead of a path-type error.")
		}
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "vendor:\n  base_path: blocker.txt/vendor\n")
		projectFile(t, root, "blocker.txt", "not a directory")
		r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true})
		require.Error(t, err)
		require.NotErrorIs(t, err, fs.ErrNotExist)
		require.Equal(t, 2, r.ExitCode("none"))
	})

	for _, dir := range []string{"atmos.d", "stacks"} {
		t.Run("unreadable "+dir, func(t *testing.T) {
			if runtime.GOOS == "windows" || os.Geteuid() == 0 {
				t.Skip("Directory permission bits are not enforced for this user or platform.")
			}
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, dir+"/inside.yaml", "{}\n")
			blocked := filepath.Join(root, dir)
			require.NoError(t, os.Chmod(blocked, 0o000))
			t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
			r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true})
			require.ErrorIs(t, err, fs.ErrPermission)
			require.Equal(t, 2, r.ExitCode("none"))
		})
	}
}

func TestEcDocumentsWithNonMappingRootsDeclareNoImports(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "- import\n- other.yaml\n")
	projectFile(t, root, "other.yaml", ecBroken)
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleImports}})
	require.Empty(t, ecLoaded(&r), "list entries are not import declarations")
	require.Empty(t, ecFindings(&r, ruleImports))
}
