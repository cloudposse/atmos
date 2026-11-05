package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// Compile-time sentinel so renaming FileSpec.Delimiters fails the build.
var _ = config.FileSpec{Delimiters: []string{"[[", "]]"}}

var defaultTestDelimiters = []string{"{{", "}}"}

// mixedDelimitersScaffoldYAML models the motivating template: ordinary files
// use the spec-level default "{{ }}", Helm chart files (which use "{{ }}"
// themselves) use "[[ ]]", and a workflow file keeps GitHub's "${{ }}"
// literal by using "<< >>".
const mixedDelimitersScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: mixed
spec:
  delimiters: ["{{", "}}"]
  fields:
    - name: name
      type: input
      default: demo
  files:
    - path: "charts/**"
      delimiters: ["[[", "]]"]
    - path: ".github/workflows/ci.yml.tmpl"
      target: ".github/workflows/<< .Config.name >>.yml"
      delimiters: ["<<", ">>"]
`

const (
	mixedHelmContent     = "name: [[ .Config.name ]]\nimage: \"{{ .Values.image }}\"\n"
	mixedPlainContent    = "name: {{ .Config.name }}\n"
	mixedWorkflowContent = "name: << .Config.name >>\nsha: ${{ github.sha }}\n"
)

func mixedDelimitersEmbedsConfig() *templates.Configuration {
	return &templates.Configuration{
		Name: "mixed",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: mixedDelimitersScaffoldYAML, Permissions: 0o644},
			// The discovered path itself is rendered with the entry's pair.
			{Path: "charts/[[ .Config.name ]]/templates/deployment.yaml", Content: mixedHelmContent, IsTemplate: true, Permissions: 0o644},
			{Path: "docs/plain.txt", Content: mixedPlainContent, IsTemplate: true, Permissions: 0o644},
			{Path: ".github/workflows/ci.yml.tmpl", Content: mixedWorkflowContent, IsTemplate: true, Permissions: 0o644},
		},
	}
}

func readFileContent(t *testing.T, elem ...string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(elem...))
	require.NoError(t, err, "expected %s to be generated", filepath.Join(elem...))
	return string(content)
}

// listGeneratedFiles returns every regular file under root (excluding the
// .atmos project record) as a sorted, forward-slash relative path.
func listGeneratedFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		// Skip the project record the real run writes; it is not template output.
		if !strings.HasPrefix(filepath.ToSlash(rel), ".atmos/") {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	}))
	sort.Strings(files)
	return files
}

func TestResolveFileDelimiters(t *testing.T) {
	active := []string{"{{", "}}"}

	tests := []struct {
		name string
		spec config.FileSpec
		want []string
	}{
		{name: "file pair wins", spec: config.FileSpec{Delimiters: []string{"[[", "]]"}}, want: []string{"[[", "]]"}},
		{name: "unset file pair uses the active pair", spec: config.FileSpec{}, want: active},
		{name: "wrong-length file pair uses the active pair", spec: config.FileSpec{Delimiters: []string{"[["}}, want: active},
		{name: "three-element file pair uses the active pair", spec: config.FileSpec{Delimiters: []string{"[[", "]]", "x"}}, want: active},
		{name: "empty-string file pair uses the active pair", spec: config.FileSpec{Delimiters: []string{"", ""}}, want: active},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveFileDelimiters(active, tt.spec))
		})
	}
}

func TestExecuteWithSetup_PerFileDelimitersMixedTemplate(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	err := ui.executeWithSetup(mixedDelimitersEmbedsConfig(), targetDir, false, false, true, "", map[string]interface{}{"name": "demo"}, defaultTestDelimiters)
	require.NoError(t, err)

	// Helm: the discovered path and "[[ ]]" actions render; Helm's own "{{ }}" stays literal.
	assert.Equal(t,
		"name: demo\nimage: \"{{ .Values.image }}\"\n",
		readFileContent(t, targetDir, "charts", "demo", "templates", "deployment.yaml"))
	// Plain: the spec-level default pair.
	assert.Equal(t, "name: demo\n", readFileContent(t, targetDir, "docs", "plain.txt"))
	// Workflow: target: and content use "<< >>"; GitHub's "${{ }}" stays literal.
	assert.Equal(t,
		"name: demo\nsha: ${{ github.sha }}\n",
		readFileContent(t, targetDir, ".github", "workflows", "demo.yml"))

	assert.Equal(t, []string{
		".github/workflows/demo.yml",
		"charts/demo/templates/deployment.yaml",
		"docs/plain.txt",
	}, listGeneratedFiles(t, targetDir))
}

// TestExecuteWithSetup_PerFileDelimitersStandInForCallerDelimiters proves the
// caller-passed delimiters (atmos init) stand in for spec-level ones when the
// scaffold declares none, while a per-file pair still wins for its file.
func TestExecuteWithSetup_PerFileDelimitersStandInForCallerDelimiters(t *testing.T) {
	const scaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: caller
spec:
  fields:
    - name: name
      type: input
      default: demo
  files:
    - path: "charts/**"
      delimiters: ["[[", "]]"]
`
	embeds := &templates.Configuration{
		Name: "caller",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "charts/values.yaml", Content: "a: [[ .Config.name ]]\nb: <% .Config.name %>\n", IsTemplate: true, Permissions: 0o644},
			{Path: "other.txt", Content: "a: [[ .Config.name ]]\nb: <% .Config.name %>\n", IsTemplate: true, Permissions: 0o644},
		},
	}
	ui := createTestUI(t)
	targetDir := t.TempDir()

	err := ui.executeWithSetup(embeds, targetDir, false, false, true, "", map[string]interface{}{"name": "demo"}, []string{"<%", "%>"})
	require.NoError(t, err)

	assert.Equal(t, "a: demo\nb: <% .Config.name %>\n", readFileContent(t, targetDir, "charts", "values.yaml"))
	assert.Equal(t, "a: [[ .Config.name ]]\nb: demo\n", readFileContent(t, targetDir, "other.txt"))
}

// TestExecuteWithSetup_PerFileDelimitersApplyToTargetAndMatrix proves the
// entry's pair drives target: rendering and a Go-template matrix axis
// expression, as well as the file's content.
func TestExecuteWithSetup_PerFileDelimitersApplyToTargetAndMatrix(t *testing.T) {
	const scaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: matrix
spec:
  files:
    - path: deploy.yaml
      delimiters: ["[[", "]]"]
      target: "deploy/[[ .matrix.environment ]].yaml"
      matrix:
        environment: '[[ collectKeys answers.environments ]]'
`
	embeds := &templates.Configuration{
		Name: "matrix",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "deploy.yaml", Content: "env: [[ .matrix.environment ]]\nhelm: {{ .Values.x }}\n", IsTemplate: true, Permissions: 0o644},
		},
	}
	ui := createTestUI(t)
	targetDir := t.TempDir()
	values := map[string]interface{}{
		"environments": map[string]interface{}{"dev": map[string]interface{}{}, "prod": map[string]interface{}{}},
	}

	err := ui.executeWithSetup(embeds, targetDir, false, false, true, "", values, defaultTestDelimiters)
	require.NoError(t, err)

	assert.Equal(t, "env: dev\nhelm: {{ .Values.x }}\n", readFileContent(t, targetDir, "deploy", "dev.yaml"))
	assert.Equal(t, "env: prod\nhelm: {{ .Values.x }}\n", readFileContent(t, targetDir, "deploy", "prod.yaml"))
	assert.Equal(t, []string{"deploy/dev.yaml", "deploy/prod.yaml"}, listGeneratedFiles(t, targetDir))
}

// TestValidateDirectoryMatrixTargetsDifferentiate_UsesPerFileDelimiters proves
// the preflight .file.* check parses each target with its own entry's pair.
func TestValidateDirectoryMatrixTargetsDifferentiate_UsesPerFileDelimiters(t *testing.T) {
	matrix := config.MatrixAxes{"env": []any{"dev", "prod"}}

	tests := []struct {
		name    string
		spec    config.FileSpec
		wantErr bool
	}{
		{
			name: "per-file pair finds the file reference",
			spec: config.FileSpec{Path: "components/**", Matrix: matrix, Delimiters: []string{"[[", "]]"}, Target: "[[ .matrix.env ]]/[[ .file.RelPath ]]"},
		},
		{
			name:    "per-file pair without a file reference collides",
			spec:    config.FileSpec{Path: "components/**", Matrix: matrix, Delimiters: []string{"[[", "]]"}, Target: "[[ .matrix.env ]]/out.txt"},
			wantErr: true,
		},
		{
			// Under the active "{{ }}" pair this would count as a file
			// reference; the entry's "[[ ]]" pair makes it plain text.
			name:    "active pair reference is ignored when the entry overrides it",
			spec:    config.FileSpec{Path: "components/**", Matrix: matrix, Delimiters: []string{"[[", "]]"}, Target: "{{ .file.RelPath }}/[[ .matrix.env ]]"},
			wantErr: true,
		},
		{
			name: "no per-file pair uses the active pair",
			spec: config.FileSpec{Path: "components/**", Matrix: matrix, Target: "{{ .matrix.env }}/{{ .file.RelPath }}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs := map[string]config.FileSpec{"components/a.txt": tt.spec, "components/b.txt": tt.spec}

			err := validateDirectoryMatrixTargetsDifferentiate(specs, defaultTestDelimiters)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestExecuteWithSetup_PerFileDelimitersLastMatchingEntryWins(t *testing.T) {
	const scaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: lastwins
spec:
  fields:
    - name: name
      type: input
      default: demo
  files:
    - path: "app/**"
      delimiters: ["[[", "]]"]
    - path: "app/special.txt"
      delimiters: ["<<", ">>"]
`
	const body = "a: [[ .Config.name ]]\nb: << .Config.name >>\n"
	embeds := &templates.Configuration{
		Name: "lastwins",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "app/other.txt", Content: body, IsTemplate: true, Permissions: 0o644},
			{Path: "app/special.txt", Content: body, IsTemplate: true, Permissions: 0o644},
		},
	}
	ui := createTestUI(t)
	targetDir := t.TempDir()

	err := ui.executeWithSetup(embeds, targetDir, false, false, true, "", map[string]interface{}{"name": "demo"}, defaultTestDelimiters)
	require.NoError(t, err)

	assert.Equal(t, "a: demo\nb: << .Config.name >>\n", readFileContent(t, targetDir, "app", "other.txt"))
	assert.Equal(t, "a: [[ .Config.name ]]\nb: demo\n", readFileContent(t, targetDir, "app", "special.txt"))
}

// TestExecuteWithSetup_PerFileDelimitersDryRunMatchesRealRun proves the
// dry-run preview lists exactly the paths a real run writes, with no raw
// delimiter markers left in any previewed path, and writes nothing itself.
func TestExecuteWithSetup_PerFileDelimitersDryRunMatchesRealRun(t *testing.T) {
	values := map[string]interface{}{"name": "demo"}

	realDir := t.TempDir()
	realUI := createTestUI(t)
	require.NoError(t, realUI.executeWithSetup(mixedDelimitersEmbedsConfig(), realDir, false, false, true, "", values, defaultTestDelimiters))
	realFiles := listGeneratedFiles(t, realDir)
	require.NotEmpty(t, realFiles)

	// executeWithSetup flushes its output buffer before returning, so the
	// preview lines are captured by driving processFileEntry directly with
	// the same inputs executeWithSetup uses.
	dryDir := t.TempDir()
	dryUI := createTestUI(t)
	dryUI.SetDryRun(true)
	embeds := mixedDelimitersEmbedsConfig()
	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(mixedDelimitersScaffoldYAML)
	require.NoError(t, err)
	fileSpecs := FileSpecByPath(scaffoldConfig, embeds.Files)
	seen := make(map[string]string)
	expansions := make(map[string]matrixExpansionResult)
	for _, file := range embeds.Files {
		if file.Path == config.ScaffoldConfigFileName {
			continue
		}
		success, failures, _, entryErr := dryUI.processFileEntry(file, fileSpecs[file.Path], dryDir, false, false, scaffoldConfig, values, defaultTestDelimiters, seen, expansions)
		require.NoError(t, entryErr)
		assert.Equal(t, 1, success)
		assert.Equal(t, 0, failures)
	}

	output := ansi.Strip(dryUI.output.String())
	for _, want := range realFiles {
		assert.Contains(t, output, want+" "+dryRunCreateStatus)
	}
	assert.Equal(t, len(realFiles), strings.Count(output, dryRunCreateStatus))
	assert.NotContains(t, output, "[[")
	assert.NotContains(t, output, "<<")
	assert.Empty(t, listGeneratedFiles(t, dryDir), "dry-run must not write anything")

	// The full dry-run entry point also succeeds and stays write-free.
	fullDryDir := t.TempDir()
	fullDryUI := createTestUI(t)
	fullDryUI.SetDryRun(true)
	require.NoError(t, fullDryUI.executeWithSetup(mixedDelimitersEmbedsConfig(), fullDryDir, false, false, true, "", values, defaultTestDelimiters))
	assert.Empty(t, listGeneratedFiles(t, fullDryDir))
}

// commitAll stages and commits everything under dir so a later --update has
// a real tracked base ref ("HEAD") to three-way merge against.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, worktree.AddWithOptions(&git.AddOptions{All: true}))
	_, err = worktree.Commit("generated", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)
}

// TestExecuteWithDelimiters_UpdatePerFileDelimitersPreservesUserEdit proves a
// tracked --update on a file with a per-file pair keeps the user's edit,
// applies the template's change, and keeps Helm's "{{ }}" literal.
func TestExecuteWithDelimiters_UpdatePerFileDelimitersPreservesUserEdit(t *testing.T) {
	values := map[string]interface{}{"name": "demo"}
	targetDir := t.TempDir()

	first := createTestUI(t)
	require.NoError(t, first.executeWithSetup(mixedDelimitersEmbedsConfig(), targetDir, false, false, true, "", values, defaultTestDelimiters))
	commitAll(t, targetDir)

	helmPath := filepath.Join(targetDir, "charts", "demo", "templates", "deployment.yaml")
	require.NoError(t, os.WriteFile(helmPath, []byte("name: demo\nimage: \"{{ .Values.image }}\"\nreplicas: 3\n"), 0o644))

	updated := mixedDelimitersEmbedsConfig()
	for i := range updated.Files {
		if strings.HasPrefix(updated.Files[i].Path, "charts/") {
			updated.Files[i].Content = "kind: [[ .Config.name ]]\n" + mixedHelmContent
		}
	}

	second := createTestUI(t)
	require.NoError(t, second.ExecuteWithDelimiters(updated, targetDir, false, true, true, "HEAD", values, defaultTestDelimiters))

	merged := readFileContent(t, helmPath)
	assert.Contains(t, merged, "kind: demo", "the template's change must be applied")
	assert.Contains(t, merged, "replicas: 3", "the user's edit must be preserved")
	assert.Contains(t, merged, `image: "{{ .Values.image }}"`, "Helm's own actions must stay literal")
	assert.NotContains(t, merged, "[[", "no raw delimiter may leak into the merge result")
}

// TestRenderPristineBase_HonorsPerFileDelimiters proves the pristine
// re-render used as the --update-strategy=rendered merge base resolves each
// file's own pair from the OLD scaffold config.
func TestRenderPristineBase_HonorsPerFileDelimiters(t *testing.T) {
	ui := createTestUI(t)
	oldConfig := mixedDelimitersEmbedsConfig()

	tempDir, cleanup, err := ui.renderPristineBase(oldConfig, map[string]interface{}{"name": "old"}, defaultTestDelimiters)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	assert.Equal(t,
		"name: old\nimage: \"{{ .Values.image }}\"\n",
		readFileContent(t, tempDir, "charts", "old", "templates", "deployment.yaml"))
	assert.Equal(t, "name: old\n", readFileContent(t, tempDir, "docs", "plain.txt"))
	assert.Equal(t,
		"name: old\nsha: ${{ github.sha }}\n",
		readFileContent(t, tempDir, ".github", "workflows", "old.yml"))
}
