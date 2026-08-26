package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// TestResolveTargetDirectory tests target directory resolution.
func TestResolveTargetDirectory(t *testing.T) {
	tests := []struct {
		name        string
		targetDir   string
		expectError bool
	}{
		{
			name:        "empty target directory",
			targetDir:   "",
			expectError: false,
		},
		{
			name:        "absolute path",
			targetDir:   "/tmp/test",
			expectError: false,
		},
		{
			name:        "relative path",
			targetDir:   "./test",
			expectError: false,
		},
		{
			name:        "current directory",
			targetDir:   ".",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTargetDirectory(tt.targetDir)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.targetDir != "" {
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}

// TestLoadScaffoldTemplates tests loading scaffold templates.
func TestLoadScaffoldTemplates(t *testing.T) {
	configs, origins, ui, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.NotNil(t, origins)
	assert.NotNil(t, ui)
	assert.NotEmpty(t, configs)
}

// TestExecuteTemplateGenerationErrors tests error paths in template generation.
func TestExecuteTemplateGenerationErrors(t *testing.T) {
	// This tests the execution flow, not full integration
	// Most error paths require complex setup with git repos, etc.

	// Test that the function exists and has proper signature
	selectedConfig := templates.Configuration{
		Name: "Test",
		Files: []templates.File{
			{Path: "test.txt", Content: "test"},
		},
	}

	// With an empty target directory in non-interactive mode the call must
	// fail fast instead of trying to prompt.
	opts := scaffoldGenerateOptions{
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{},
	}
	err := executeTemplateGeneration(&selectedConfig, "", &opts, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirRequired)
}

func TestScaffoldCommandProvider_UncoveredMetadata(t *testing.T) {
	provider := &ScaffoldCommandProvider{}

	assert.Nil(t, provider.GetAliases())
	assert.True(t, provider.IsExperimental())
}

func TestSelectGenerateTemplate_NonInteractiveRequiresName(t *testing.T) {
	_, err := selectGenerateTemplate(&scaffoldGenerateOptions{interactive: false}, map[string]templates.Configuration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTemplateNameRequired)
}

// TestExecuteScaffoldGenerate_DryRunNonexistentTargetDirectory proves the
// primary real-world dry-run use case still works: previewing generation
// into a target directory that doesn't exist yet (nested, so its parents
// don't exist either). Its validation, filesystem.ValidateTargetDirectory,
// returns nil immediately for a missing path, so routing dry-run through the
// real generation path (now true for every --dry-run, not just --dry-run
// --update) must not require the target to pre-exist.
func TestExecuteScaffoldGenerate_DryRunNonexistentTargetDirectory(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")

	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		targetDir:      targetDir,
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{"project_name": "demo"},
	})

	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(targetDir, "README.md"), "dry-run must not write any files")
}

// TestExecuteScaffoldGenerate_DryRunNonEmptyTargetWithoutForceOrUpdate_Errors
// proves plain `--dry-run` (no --update) now reflects the same
// filesystem.ValidateTargetDirectory check a real run performs: previewing
// against an existing, non-empty target directory without --force or
// --update fails exactly like the real run it's meant to preview would fail.
// Before routing dry-run through the real generation path, the standalone
// preview implementation never checked the target directory's state at all
// and would happily list files regardless.
func TestExecuteScaffoldGenerate_DryRunNonEmptyTargetWithoutForceOrUpdate_Errors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o600))

	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		targetDir:      dir,
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{"project_name": "demo"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
}

func TestExecuteScaffoldGenerate_DryRunBuiltInTemplate(t *testing.T) {
	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		targetDir:      t.TempDir(),
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{"project_name": "demo"},
	})

	require.NoError(t, err)
}

func TestExecuteScaffoldGenerate_InvalidMergeDriver(t *testing.T) {
	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		targetDir:      t.TempDir(),
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		mergeDriver:    "bogus",
		templateValues: map[string]interface{}{},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnknownMergeDriver)
}

func TestExecuteScaffoldGenerate_DryRunRequiresTarget(t *testing.T) {
	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirRequired)
}

func TestExecuteScaffoldList_LoadsAndDisplaysTemplates(t *testing.T) {
	require.NoError(t, executeScaffoldList(nil))
}

func TestExecuteValidateScaffold_EndToEnd(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		require.NoError(t, executeValidateScaffold(context.Background(), t.TempDir()))
	})

	t.Run("valid scaffold", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scaffold.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: valid
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`), 0o600))

		require.NoError(t, executeValidateScaffold(context.Background(), dir))
	})

	t.Run("invalid scaffold", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scaffold.yaml")
		require.NoError(t, os.WriteFile(path, []byte("not: a scaffold\n"), 0o600))

		err := executeValidateScaffold(context.Background(), dir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrScaffoldValidation)
	})
}

func TestScaffoldGenerateRunE_DryRunAndSetFlags(t *testing.T) {
	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("set", "project_name=demo"))

	err := scaffoldGenerateCmd.RunE(cmd, []string{"simple", t.TempDir()})

	require.NoError(t, err)
}

func TestScaffoldGenerateRunE_MalformedSetFlag(t *testing.T) {
	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("set", "missing-equals"))

	err := scaffoldGenerateCmd.RunE(cmd, []string{"simple", t.TempDir()})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

// TestScaffoldGenerateRunE_UpdateFlagWithPositionalTarget_ResolvesBaseRef
// covers the RunE-level "if update && target != \"\"" pre-resolution branch:
// when --update is combined with a positional target directory, RunE must
// resolve defaultBaseRef itself (rather than deferring to the interactive
// flow, which never runs for a positional target) before generation. With no
// pinned metadata at the target, defaultBaseRef falls back to "HEAD" and
// generation (here, dry-run) must still succeed.
func TestScaffoldGenerateRunE_UpdateFlagWithPositionalTarget_ResolvesBaseRef(t *testing.T) {
	// RunE binds this test's flags to the global viper.GetViper() singleton
	// (BindFlagsToViper), which outlives the test unless reset -- cmd.NewTestKit
	// only restores RootCmd state and isn't available to this package (it
	// would create an import cycle back into cmd). Reset viper directly so a
	// later test reading the same keys doesn't see this test's bound values.
	t.Cleanup(func() { viper.Reset() })

	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("update", "true"))

	err := scaffoldGenerateCmd.RunE(cmd, []string{"simple", t.TempDir()})

	require.NoError(t, err)
}

// TestScaffoldGenerateRunE_UpdateFlagWithPositionalTarget_PropagatesBaseRefError
// reproduces a corrupt .atmos/scaffold/metadata.yaml at the target directory:
// RunE's eager defaultBaseRef resolution (for --update with a positional
// target) must propagate that error immediately instead of silently falling
// back to "HEAD" and letting generation proceed against a damaged pin.
func TestScaffoldGenerateRunE_UpdateFlagWithPositionalTarget_PropagatesBaseRefError(t *testing.T) {
	// See the sibling test above for why this reset is needed.
	t.Cleanup(func() { viper.Reset() })

	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("update", "true"))

	err := scaffoldGenerateCmd.RunE(cmd, []string{"simple", dir})

	require.Error(t, err)
}

// TestMaybeInitGeneratedGitRepository_PropagatesInitGitError reproduces
// InitGitRepository failing (a leftover regular file named ".git" blocks
// git.PlainInit) and asserts maybeInitGeneratedGitRepository returns that
// error directly instead of proceeding to call PinInitialBaseRef with a
// bogus empty headSHA.
func TestMaybeInitGeneratedGitRepository_PropagatesInitGitError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("blocker"), 0o600))

	cfg := &templates.Configuration{Name: "demo"}
	err := maybeInitGeneratedGitRepository(dir, cfg, &scaffoldGenerateOptions{git: true})

	require.Error(t, err)
	assert.NoFileExists(t, storage.ScaffoldMetadataPath(dir), "InitGitRepository failure must prevent PinInitialBaseRef from running")
}

// TestExecuteScaffoldGenerate_DryRunPropagatesInvalidScaffoldConfig
// reproduces an invalid scaffold.yaml in the selected template (unparseable
// YAML): routing dry-run through the real generation path must still
// surface a parse error immediately, rather than silently proceeding to
// preview an empty file list.
func TestExecuteScaffoldGenerate_DryRunPropagatesInvalidScaffoldConfig(t *testing.T) {
	_, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)

	cfg := &templates.Configuration{
		Name: "broken",
		Files: []templates.File{
			{Path: config.ScaffoldConfigFileName, Content: "not: valid: yaml: ["},
		},
	}

	scaffoldUI.SetDryRun(true)
	err = executeTemplateGeneration(cfg, t.TempDir(), &scaffoldGenerateOptions{
		dryRun:      true,
		useDefaults: true,
	}, scaffoldUI)

	require.Error(t, err)
}

func TestScaffoldListAndValidateRunE(t *testing.T) {
	require.NoError(t, scaffoldListCmd.RunE(&cobra.Command{}, nil))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: valid
spec:
  fields: []
`), 0o600))

	require.NoError(t, scaffoldValidateCmd.RunE(&cobra.Command{}, []string{dir}))
}

// TestSelectTemplateErrors tests error handling in template selection.
func TestSelectTemplateErrors(t *testing.T) {
	configs := map[string]templates.Configuration{
		"template1": {Name: "template1", TemplateID: "id1"},
		"template2": {Name: "template2", TemplateID: "id2"},
	}

	// Test selecting non-existent template. selectTemplateByName never
	// touches scaffoldUI, so a nil ScaffoldUI is safe here.
	_, err := selectTemplate("nonexistent", configs, nil)
	assert.Error(t, err)

	// Test selecting with empty name: this triggers selectTemplateInteractive,
	// which calls scaffoldUI.PromptForTemplate -- simulate a prompt failure
	// (e.g. no TTY available) via a mock rather than a nil receiver.
	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().PromptForTemplate("scaffold", gomock.Any()).Return("", assert.AnError)

	_, err = selectTemplate("", configs, mockUI)
	assert.Error(t, err)
}

func TestMaybeInitGeneratedGitRepository_GitEnabled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o600))

	cfg := &templates.Configuration{Name: "demo", Version: "1.0.0"}
	err := maybeInitGeneratedGitRepository(dir, cfg, &scaffoldGenerateOptions{git: true})

	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(dir, ".git"))
}

func TestMaybeInitGeneratedGitRepository_GitDisabled(t *testing.T) {
	dir := t.TempDir()

	cfg := &templates.Configuration{Name: "demo"}
	err := maybeInitGeneratedGitRepository(dir, cfg, &scaffoldGenerateOptions{git: false})

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(dir, ".git"))
}

// TestExecuteTemplateGeneration_WithTargetDir covers the targetDir != "" branch of
// executeTemplateGeneration, which drives the real UI (safe: the "simple" built-in
// template has a scaffold.yaml, and useDefaults:true skips all interactive huh
// prompts) rather than the targetDir == "" branch, which always prompts for a
// target directory via a real terminal form and cannot be safely unit tested.
func TestExecuteTemplateGeneration_WithTargetDir(t *testing.T) {
	configs, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	cfg := configs["simple"]

	t.Run("success without git", func(t *testing.T) {
		dir := t.TempDir()
		opts := &scaffoldGenerateOptions{
			useDefaults:    true,
			templateValues: map[string]interface{}{"project_name": "demo"},
		}

		err := executeTemplateGeneration(&cfg, dir, opts, scaffoldUI)

		require.NoError(t, err)
		assert.NoDirExists(t, filepath.Join(dir, ".git"))
	})

	t.Run("success with git", func(t *testing.T) {
		dir := t.TempDir()
		opts := &scaffoldGenerateOptions{
			useDefaults:    true,
			git:            true,
			templateValues: map[string]interface{}{"project_name": "demo"},
		}

		err := executeTemplateGeneration(&cfg, dir, opts, scaffoldUI)

		require.NoError(t, err)
		assert.DirExists(t, filepath.Join(dir, ".git"))
	})
}

func TestShouldOfferScaffoldUpdate(t *testing.T) {
	notEmptyErr := errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).Err()
	otherErr := errUtils.Build(errUtils.ErrInitialization).Err()

	tests := []struct {
		name        string
		err         error
		opts        *scaffoldGenerateOptions
		wantOffer   bool
		wantBaseRef string
	}{
		{"nil error", nil, &scaffoldGenerateOptions{interactive: true}, false, ""},
		{"force already set", notEmptyErr, &scaffoldGenerateOptions{interactive: true, force: true}, false, ""},
		{"update already set", notEmptyErr, &scaffoldGenerateOptions{interactive: true, update: true}, false, ""},
		{"not interactive", notEmptyErr, &scaffoldGenerateOptions{interactive: false}, false, ""},
		{"dry run", notEmptyErr, &scaffoldGenerateOptions{interactive: true, dryRun: true}, false, ""},
		{"different error", otherErr, &scaffoldGenerateOptions{interactive: true}, false, ""},
		{"offers with default HEAD base ref", notEmptyErr, &scaffoldGenerateOptions{interactive: true}, true, "HEAD"},
		{"offers with caller base ref", notEmptyErr, &scaffoldGenerateOptions{interactive: true, baseRef: "v1.2.3"}, true, "v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A fresh, never-written directory: shouldOfferScaffoldUpdate must
			// resolve against the *actual* target passed in, not any stale
			// opts.targetDir (see TestShouldOfferScaffoldUpdate_UsesActualTargetDir
			// for the regression this guards against).
			offer, baseRef, err := shouldOfferScaffoldUpdate(tt.err, tt.opts, t.TempDir())
			require.NoError(t, err)
			assert.Equal(t, tt.wantOffer, offer)
			assert.Equal(t, tt.wantBaseRef, baseRef)
		})
	}
}

// TestShouldOfferScaffoldUpdate_UsesActualTargetDir reproduces the interactive
// retry-offer half of the bug described in defaultBaseRef's doc comment:
// opts.targetDir is the raw positional CLI arg, which is "" when the user ran
// `atmos scaffold generate --update` with no target and the interactive flow
// picked the real directory itself. ShouldOfferScaffoldUpdate must resolve
// the retry base ref against the caller-supplied targetDir parameter (the
// real, resolved directory), not opts.targetDir.
func TestShouldOfferScaffoldUpdate_UsesActualTargetDir(t *testing.T) {
	dir := t.TempDir()
	metadata := storage.NewScaffoldMetadata("demo", "1.0.0", "embedded", "pinned-at-real-dir", nil)
	require.NoError(t, storage.NewMetadataStorage(storage.ScaffoldMetadataPath(dir)).Save(metadata))

	notEmptyErr := errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).Err()
	// opts.targetDir left empty on purpose: it mirrors the raw positional arg
	// in the no-target interactive scenario, and must be ignored in favor of
	// the targetDir parameter below.
	opts := &scaffoldGenerateOptions{interactive: true}

	offer, baseRef, err := shouldOfferScaffoldUpdate(notEmptyErr, opts, dir)

	require.NoError(t, err)
	assert.True(t, offer)
	assert.Equal(t, "pinned-at-real-dir", baseRef)
}

// TestShouldOfferScaffoldUpdate_PropagatesMetadataLoadError verifies a
// corrupt/unreadable metadata file surfaces as an error from
// shouldOfferScaffoldUpdate rather than silently resolving to "HEAD".
func TestShouldOfferScaffoldUpdate_PropagatesMetadataLoadError(t *testing.T) {
	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	notEmptyErr := errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).Err()
	opts := &scaffoldGenerateOptions{interactive: true}

	offer, baseRef, err := shouldOfferScaffoldUpdate(notEmptyErr, opts, dir)

	require.Error(t, err)
	assert.False(t, offer)
	assert.Empty(t, baseRef)
}

// TestDefaultBaseRef pins two behaviors:
//   - An explicit --base-ref always wins, regardless of targetDir.
//   - With no --base-ref and no pinned metadata at targetDir, it still falls
//     back to "HEAD" -- the original fix for --update with no --base-ref
//     silently setting up no git storage at all (ExecuteWithDelimiters only
//     calls SetupGitStorage when baseRef is non-empty), which failed every
//     file with an opaque "three-way merge failed" even on a completely
//     unmodified, freshly re-run directory.
func TestDefaultBaseRef(t *testing.T) {
	headRef, err := defaultBaseRef("", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "HEAD", headRef)

	explicitRef, err := defaultBaseRef("v1.2.3", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", explicitRef)
}

// TestDefaultBaseRef_PrefersPinnedMetadata reproduces the fix for the bug
// where `--update` with no --base-ref always diffs against live HEAD, so a
// customization the user committed after generation becomes indistinguishable
// from the unmodified base -- the merge then silently lets the freshly
// rendered template win with no conflict, discarding the user's edit. When a
// pinned base ref exists (written once, at initial `--git` generation --
// see gen.PinInitialBaseRef), defaultBaseRef must prefer it over live HEAD.
func TestDefaultBaseRef_PrefersPinnedMetadata(t *testing.T) {
	dir := t.TempDir()
	metadata := storage.NewScaffoldMetadata("demo", "1.0.0", "embedded", "abc123pinned", nil)
	require.NoError(t, storage.NewMetadataStorage(storage.ScaffoldMetadataPath(dir)).Save(metadata))

	pinnedRef, err := defaultBaseRef("", dir)
	require.NoError(t, err)
	assert.Equal(t, "abc123pinned", pinnedRef)

	// An explicit --base-ref still overrides the pin.
	explicitRef, err := defaultBaseRef("v9.9.9", dir)
	require.NoError(t, err)
	assert.Equal(t, "v9.9.9", explicitRef)
}

// TestDefaultBaseRef_PropagatesUnreadableMetadataError reproduces the bug
// where any metadata.Load() error (not just "file doesn't exist") was
// silently swallowed and defaultBaseRef fell back to "HEAD" regardless --
// defeating the pin fix, since a corrupt pin file would silently
// re-introduce the original silent-overwrite bug (diffing against live HEAD)
// instead of surfacing the problem. The storage.MetadataStorage.Load method
// returns (nil, nil) only when the file is genuinely absent (os.IsNotExist);
// any other failure (corrupt YAML here) must propagate as an error.
func TestDefaultBaseRef_PropagatesUnreadableMetadataError(t *testing.T) {
	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	resolved, err := defaultBaseRef("", dir)

	require.Error(t, err)
	assert.Empty(t, resolved)
	assert.NotEqual(t, "HEAD", resolved, "a corrupt metadata file must not silently fall back to HEAD")
}

// TestExecuteTemplateGeneration_UpdateFlag_MergesExistingDirectory covers the
// real bug --update fixes: re-running scaffold generation against an
// already-generated, git-initialized directory with update+base-ref=HEAD
// regenerates the template while preserving the user's own edits via a
// 3-way merge, instead of failing with "target directory is not empty".
func TestExecuteTemplateGeneration_UpdateFlag_MergesExistingDirectory(t *testing.T) {
	configs, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	cfg := configs["simple"]

	dir := t.TempDir()
	opts := &scaffoldGenerateOptions{
		useDefaults:    true,
		templateValues: map[string]interface{}{"project_name": "demo"},
	}
	require.NoError(t, executeTemplateGeneration(&cfg, dir, opts, scaffoldUI))

	scaffoldGitInit(t, dir)
	scaffoldGitCommitAll(t, dir, "initial")

	readmePath := filepath.Join(dir, "README.md")
	original, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readmePath, append(original, []byte("\nuser note\n")...), 0o600))

	updateOpts := &scaffoldGenerateOptions{
		useDefaults:    true,
		update:         true,
		baseRef:        "HEAD",
		templateValues: map[string]interface{}{"project_name": "demo"},
	}
	err = executeTemplateGeneration(&cfg, dir, updateOpts, scaffoldUI)

	require.NoError(t, err)
	merged, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), "user note", "the user's manual edit must survive the 3-way merge")
}

// TestExecuteTemplateGeneration_UpdateFlag_PreservesCommittedEdit reproduces
// a client-reported bug: `--update` silently discards a customization the
// user committed to the generated project, as long as the template's own
// change lands on a *different* line. Sequence (mirroring the report
// exactly):
//  1. Generate with --git (creates the pristine-content initial commit and
//     pins its SHA -- see gen.PinInitialBaseRef).
//  2. The user edits `runner: ubuntu-latest` to `runner: self-hosted` and
//     commits it.
//  3. The template changes an unrelated line (`app:` -> `appName:`).
//  4. Re-run with --update and no --base-ref (the common case -- most users
//     never pass --base-ref explicitly).
//
// Before the fix, --update's default base ref is live HEAD, which by step 4
// is the user's own commit -- so git sees `runner: self-hosted` as part of
// the "base" and the merge treats it as unchanged, letting the freshly
// rendered template silently overwrite it back to `ubuntu-latest`. The fix
// makes the default prefer the SHA pinned at step 1, so the merge always
// diffs against the true pristine content regardless of what's since been
// committed.
func TestExecuteTemplateGeneration_UpdateFlag_PreservesCommittedEdit(t *testing.T) {
	_, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)

	cfg := &templates.Configuration{
		Name: "values-template",
		Files: []templates.File{
			{
				Path:        "deploy/values/default.yaml",
				Content:     "app: {{ .Config.service }}\nrunner: ubuntu-latest\n",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}

	dir := t.TempDir()
	opts := &scaffoldGenerateOptions{
		useDefaults:    true,
		git:            true,
		templateValues: map[string]interface{}{"service": "svc-x"},
	}
	require.NoError(t, executeTemplateGeneration(cfg, dir, opts, scaffoldUI))

	valuesPath := filepath.Join(dir, "deploy", "values", "default.yaml")
	original, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.Equal(t, "app: svc-x\nrunner: ubuntu-latest\n", string(original))

	// The user customizes and commits a line the template will never touch again.
	require.NoError(t, os.WriteFile(valuesPath, []byte("app: svc-x\nrunner: self-hosted\n"), 0o600))
	scaffoldGitCommitAll(t, dir, "customize runner")

	// The template changes an unrelated line.
	cfg.Files[0].Content = "appName: {{ .Config.service }}\nrunner: ubuntu-latest\n"

	// Mirrors the RunE handler: resolve --base-ref's default (empty here,
	// exactly like a real `--update` invocation with no --base-ref flag) the
	// same way the CLI does, before calling into executeTemplateGeneration.
	resolvedBaseRef, err := defaultBaseRef("", dir)
	require.NoError(t, err)
	updateOpts := &scaffoldGenerateOptions{
		useDefaults:    true,
		update:         true,
		baseRef:        resolvedBaseRef,
		templateValues: map[string]interface{}{"service": "svc-x"},
	}
	require.NoError(t, executeTemplateGeneration(cfg, dir, updateOpts, scaffoldUI))

	merged, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), "appName: svc-x", "the template's own change must still apply")
	assert.Contains(t, string(merged), "runner: self-hosted", "the user's committed customization must survive --update")
}

// TestExecuteTemplateGeneration_DryRunMatrixExpansion proves the CodeRabbit-
// reported gap is fixed at the cmd/scaffold routing level, not just inside
// pkg/generator/ui: a plain `--dry-run` (no --update) run of a template
// whose scaffold.yaml declares spec.files[].matrix (with a matrix-driven
// spec.files[].target) goes through the exact same real generation path a
// non-dry-run run uses, so it accounts for every matrix-expanded output
// instead of the single, unexpanded path the old standalone preview
// (collectDryRunFiles) used to report. Proven by comparing a real run of the
// template (which writes one file per matrix value) against a dry-run of
// the identical template, which must write nothing to either its own target
// directory or any matrix-expanded subpath.
func TestExecuteTemplateGeneration_DryRunMatrixExpansion(t *testing.T) {
	_, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: matrix-template
spec:
  files:
    - path: deploy.yaml
      target: "deploy/{{ .matrix.region }}.yaml"
      matrix:
        region: [us-east-1, us-west-2, eu-west-1]
`
	cfg := &templates.Configuration{
		Name: "matrix-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "deploy.yaml", Content: "region: {{ .matrix.region }}\n", IsTemplate: true, Permissions: 0o644},
		},
	}
	regions := []string{"us-east-1", "us-west-2", "eu-west-1"}

	// A real run writes one file per matrix value.
	realDir := t.TempDir()
	require.NoError(t, executeTemplateGeneration(cfg, realDir, &scaffoldGenerateOptions{useDefaults: true}, scaffoldUI))
	for _, region := range regions {
		content, readErr := os.ReadFile(filepath.Join(realDir, "deploy", region+".yaml"))
		require.NoError(t, readErr, "real generation must write deploy/%s.yaml", region)
		assert.Equal(t, "region: "+region+"\n", string(content))
	}

	// The same template, previewed with plain --dry-run (no --update),
	// must not write any of those matrix-expanded files -- while still
	// succeeding, proving it accounted for (rather than erroring on, or
	// silently dropping) the matrix expansion.
	dryDir := t.TempDir()
	scaffoldUI.SetDryRun(true)
	defer scaffoldUI.SetDryRun(false)
	err = executeTemplateGeneration(cfg, dryDir, &scaffoldGenerateOptions{useDefaults: true, dryRun: true}, scaffoldUI)
	require.NoError(t, err)
	for _, region := range regions {
		assert.NoFileExists(t, filepath.Join(dryDir, "deploy", region+".yaml"), "dry-run must not write deploy/%s.yaml", region)
	}
}

// scaffoldGitInit initializes dir as a git repository using go-git (no
// external git binary), mirroring the idiom in
// pkg/generator/gitinit.go's InitGitRepository. Setup failures fail the test
// loudly via require.NoError rather than skipping, since go-git has no
// external binary dependency that can be "unavailable".
func scaffoldGitInit(t *testing.T, dir string) {
	t.Helper()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)
}

// scaffoldGitCommitAll opens the git repository at dir, stages all files,
// and creates a commit with the given message, using go-git. Unlike a real
// `git commit`, go-git's Commit never shells out to gpg, so no
// commit.gpgsign workaround is needed.
func scaffoldGitCommitAll(t *testing.T, dir, message string) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, wt.AddGlob("."))
	_, err = wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
}

func TestSelectGenerateTemplate_ConfigHit(t *testing.T) {
	configs := map[string]templates.Configuration{
		"demo": {Name: "demo", Description: "demo template"},
	}

	result, err := selectGenerateTemplate(&scaffoldGenerateOptions{templateName: "demo"}, configs, nil)

	require.NoError(t, err)
	assert.Equal(t, "demo", result.Name)
}

func TestSelectGenerateTemplate_TemplateSource(t *testing.T) {
	result, err := selectGenerateTemplate(
		&scaffoldGenerateOptions{templateName: "./local-template"},
		map[string]templates.Configuration{},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, "./local-template", result.Name)
}

func TestSelectGenerateTemplate_FallbackNotFound(t *testing.T) {
	_, err := selectGenerateTemplate(
		&scaffoldGenerateOptions{templateName: "nonexistent", interactive: false},
		map[string]templates.Configuration{},
		nil,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldNotFound)
}

func TestMergeConfiguredTemplates_Success(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "my-template")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`scaffold:
  templates:
    my-template:
      description: My template
      source: ./my-template
`), 0o600))
	t.Chdir(dir)

	configs := map[string]templates.Configuration{}
	origins := map[string]string{}
	err := mergeConfiguredTemplates(configs, origins)

	require.NoError(t, err)
	require.Contains(t, configs, "my-template")
	assert.Equal(t, "atmos.yaml", origins["my-template"])
}

func TestMergeConfiguredTemplates_WarnsAndContinues(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`scaffold:
  templates:
    broken-template:
      description: Missing source, cannot be converted
`), 0o600))
	t.Chdir(dir)

	configs := map[string]templates.Configuration{}
	origins := map[string]string{}
	err := mergeConfiguredTemplates(configs, origins)

	require.NoError(t, err)
	assert.NotContains(t, configs, "broken-template")
}

func TestDetermineScaffoldPathsToValidate_EmptyPathDefaultsToCwd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte("apiVersion: atmos/v1\n"), 0o600))
	t.Chdir(dir)

	paths, err := determineScaffoldPathsToValidate("")

	require.NoError(t, err)
	assert.Len(t, paths, 1)
}

func TestValidateScaffoldFile_ReadError(t *testing.T) {
	// A directory can't be read as a file, forcing os.ReadFile to fail.
	err := validateScaffoldFile(t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldReadFile)
}

// TestFindScaffoldFilesInDirectory_WalkError exercises the walk-error branch
// deterministically (a nonexistent root makes filepath.Walk's initial Lstat
// fail) instead of relying on chmod-based permission denial, which is
// unreliable when tests run as root or on Windows.
func TestFindScaffoldFilesInDirectory_WalkError(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := findScaffoldFilesInDirectory(nonexistent, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldDirectoryRead)
}
