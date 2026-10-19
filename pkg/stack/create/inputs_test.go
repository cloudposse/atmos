package create

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

func TestReservedInputsMustBeConsumedByTheTemplate(t *testing.T) {
	values := map[string]any{"environment": "dev"}
	for _, tc := range []struct {
		name    string
		options Options
		flag    string
	}{
		{name: "name", options: Options{Name: "development"}, flag: "--name"},
		{name: "import", options: Options{Imports: []string{"catalog/defaults"}}, flag: "--import"},
		{name: "component", options: Options{Components: []string{"terraform:other"}}, flag: "--component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := fixture(t)
			customFixture(t, customDefinition)
			tc.options.File, tc.options.Template, tc.options.Defaults, tc.options.Values = "stacks/deploy/dev.yaml", "application-stack", true, values
			_, err := Prepare(cfg, &tc.options, renderer)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			assert.Contains(t, err.Error(), `template "application-stack" does not use `+tc.flag)
			assert.Contains(t, hints(err), "remove `"+tc.flag+"`")
			assertNothingWritten(t, cfg, tc.options.File)
		})
	}
	t.Run("a component the template already declares is accepted", func(t *testing.T) {
		cfg, renderer := fixture(t)
		customFixture(t, customDefinition)
		_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: values, Components: []string{"terraform:network"}}, renderer)
		require.NoError(t, err)
	})
}

func TestRefIsRejectedForNonGitScaffolds(t *testing.T) {
	cfg, renderer := fixture(t)
	customFixture(t, customDefinition)
	for name, tc := range map[string]struct {
		template, want string
	}{
		"embedded":         {template: "", want: "is built into Atmos"},
		"configured local": {template: "application-stack", want: "is not a Git source (./scaffolds/app)"},
		"local path":       {template: "./scaffolds/app", want: "is not a Git source (./scaffolds/app)"},
	} {
		t.Run(name, func(t *testing.T) {
			opts := &Options{File: "stacks/deploy/dev.yaml", Template: tc.template, Ref: "v1", Defaults: true, Values: map[string]any{"environment": "dev"}}
			_, err := Prepare(cfg, opts, renderer)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			assert.Contains(t, err.Error(), "--ref=v1")
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, hints(err), "Git repository")
		})
	}
}

func TestRefIsAcceptedForGitSourcesAndWhenUnset(t *testing.T) {
	git := &templates.Configuration{Name: "remote", Source: "github.com/cloudposse/atmos//examples/stack?ref=v1"}
	require.NoError(t, rejectRefForNonGit(git, "v1"))
	require.NoError(t, rejectRefForNonGit(&templates.Configuration{Name: "local", Source: "./scaffolds/app"}, ""))
}

func TestComponentDeclarationsAreTrimmedAndValidated(t *testing.T) {
	cfg, renderer := fixture(t)
	for _, tc := range []struct {
		name       string
		components []string
		want       string
	}{
		{name: "duplicate after trimming", components: []string{"terraform:a", "terraform:a "}, want: `duplicate component "terraform:a"`},
		{name: "duplicate through kind whitespace", components: []string{"terraform:a", " terraform :a"}, want: "duplicate"},
		{name: "whitespace inside the name", components: []string{"terraform:a b"}, want: "cannot contain whitespace"},
		{name: "blank name", components: []string{"terraform:  "}, want: "supported kind:name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Components: tc.components, Defaults: true}, renderer)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Name: "dev", Components: []string{" terraform : network ", "helmfile:ingress"}, Defaults: true}, renderer)
	require.NoError(t, err)
	var manifest struct {
		Components map[string]map[string]any `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(plan.Generation.Files[0].Content), &manifest))
	assert.Contains(t, manifest.Components["terraform"], "network")
	assert.Contains(t, manifest.Components["helmfile"], "ingress")
}

func TestImportsAreTrimmed(t *testing.T) {
	cfg, renderer := fixture(t)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "catalog", "defaults.yaml"), "vars:\n  region: us-east-1\n")
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Name: "dev", Imports: []string{"  catalog/defaults "}, Defaults: true}, renderer)
	require.NoError(t, err)
	assert.Contains(t, plan.Generation.Files[0].Content, `"catalog/defaults"`)
}

func TestExistingManifestIsReportedWithoutOverwriteGuidance(t *testing.T) {
	cfg, renderer := fixture(t)
	opts := &Options{File: "stacks/deploy/dev.yaml", Name: "dev", Defaults: true}
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.NoError(t, plan.Apply(context.Background(), nil))
	before, err := os.ReadFile(opts.File)
	require.NoError(t, err)
	_, err = Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrFileExists)
	assert.Contains(t, err.Error(), "stacks/deploy/dev.yaml already exists; stack create never overwrites files")
	assert.Contains(t, hints(err), "atmos stack config set")
	assert.Contains(t, hints(err), "delete")
	for _, text := range []string{err.Error(), hints(err)} {
		assert.NotContains(t, text, "--force")
		assert.NotContains(t, text, "--update")
		assert.NotContains(t, text, "{{")
	}
	after, err := os.ReadFile(opts.File)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestExistingSharedOutputIsReportedWithoutOverwriteGuidance(t *testing.T) {
	cfg, renderer := fixture(t)
	customFixture(t, customDefinition)
	write(t, "components/terraform/network/main.tf", "# hand written\n")
	opts := &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev", "sources": true}}
	_, err := Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrFileExists)
	assert.Contains(t, err.Error(), "components/terraform/network/main.tf already exists; stack create never overwrites files")
	for _, text := range []string{err.Error(), hints(err)} {
		assert.NotContains(t, text, "--force")
		assert.NotContains(t, text, "--update")
	}
	assertNothingWritten(t, cfg, opts.File)
}

func recordFiles(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, recordDirectory))
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(root, recordDirectory, entry.Name()))
	}
	return paths
}

func TestStaleGenerationRecordIsReplacedAfterTheManifestIsDeleted(t *testing.T) {
	cfg, renderer := fixture(t)
	root := cfg.BasePathAbsolute
	opts := &Options{File: "stacks/deploy/dev.yaml", Name: "first-name", Defaults: true}
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.NoError(t, plan.Apply(context.Background(), nil))
	records := recordFiles(t, root)
	require.Len(t, records, 1)
	first, err := os.ReadFile(records[0])
	require.NoError(t, err)
	assert.Contains(t, string(first), "first-name")

	// While the manifest exists the record is not touched: creation is refused instead.
	opts.Name = "second-name"
	_, err = Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrFileExists)
	unchanged, err := os.ReadFile(records[0])
	require.NoError(t, err)
	assert.Equal(t, first, unchanged)

	// Once the manifest is gone the record is stale and creation replaces it.
	require.NoError(t, os.Remove(opts.File))
	plan, err = Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	// Planning is read-only, and an aborted apply keeps the old record.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, plan.Apply(ctx, nil), context.Canceled)
	kept, err := os.ReadFile(records[0])
	require.NoError(t, err)
	assert.Equal(t, first, kept)
	require.NoError(t, plan.Apply(context.Background(), nil))
	assert.Equal(t, records, recordFiles(t, root))
	replaced, err := os.ReadFile(records[0])
	require.NoError(t, err)
	assert.Contains(t, string(replaced), "second-name")
	assert.NotContains(t, string(replaced), "first-name")
	manifest, err := os.ReadFile(opts.File)
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "second-name")
	assert.False(t, strings.Contains(string(manifest), "first-name"))
}

func TestUnmarkedTemplateInManifestIsExplained(t *testing.T) {
	cfg, renderer := fixture(t)
	customFixture(t, customDefinition)
	// Without the `# atmos:template` marker the action is left in the output and the YAML no longer parses.
	write(t, "scaffolds/app/stack.yaml", "vars:\n  stage: {{ .Config.environment }}\n")
	opts := &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}
	_, err := Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrUnprocessedTemplate)
	assert.Contains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), "not marked as a template")
	assertNothingWritten(t, cfg, opts.File)
}

func TestTranslateRenderErrorKeepsUnrelatedErrorsAndGeneralizesUnnamedConflicts(t *testing.T) {
	unrelated := errors.New("renderer exploded")
	assert.Same(t, unrelated, translateRenderError(unrelated))

	// A conflict whose destination the renderer did not name still loses the --force/--update guidance.
	err := translateRenderError(fmt.Errorf("%w: something is in the way", errUtils.ErrFileExists))
	require.ErrorIs(t, err, errUtils.ErrFileExists)
	assert.Contains(t, err.Error(), "a file the scaffold writes already exists; stack create never overwrites files")
	assert.Contains(t, hints(err), "choose a scaffold that writes elsewhere")
	assert.NotContains(t, err.Error()+hints(err), "--force")

	// A destination that is itself an unrendered template is not echoed back as a path.
	templated := cockroach.WithDetail(fmt.Errorf("%w: conflict", errUtils.ErrFileExists), "File already exists: `{{ .Config.stack_file }}`")
	err = translateRenderError(templated)
	assert.NotContains(t, err.Error(), "{{")
	assert.Contains(t, err.Error(), "a file the scaffold writes already exists")

	// A named destination is reported as such.
	named := cockroach.WithDetail(fmt.Errorf("%w: conflict", errUtils.ErrFileExists), "File already exists: `components/vpc/main.tf`")
	assert.Contains(t, translateRenderError(named).Error(), "components/vpc/main.tf already exists; stack create never overwrites files")
}

func TestImportedPathsAcceptPlainAndMappedEntries(t *testing.T) {
	document := map[string]any{"import": []any{
		"catalog/plain",
		map[string]any{"path": " catalog/mapped ", "context": map[string]any{"region": "us-east-1"}},
		map[string]any{"context": map[string]any{}}, // No path: ignored.
		42, // Unsupported entry: ignored.
	}}
	require.NoError(t, verifyReservedInputs("stack", &Options{Imports: []string{"catalog/plain", "catalog/mapped"}}, nil, document))

	err := verifyReservedInputs("stack", &Options{Imports: []string{"catalog/other"}}, nil, document)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), `--import "catalog/other" was not written`)
	assert.Equal(t, map[string]bool{"catalog/plain": true, "catalog/mapped": true}, importedPaths(document["import"]))
}
