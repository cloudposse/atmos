package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeModuleFiles gives the fixture module the files a real Terraform module has. Terragrunt marks
// them as read without opening them, so they reach the sensitivity checks unread.
func writeModuleFiles(t *testing.T, root string) {
	t.Helper()
	module := filepath.Join(root, "module")
	files := map[string]string{
		"main.tf":         "variable \"password\" {\n  type      = string\n  sensitive = true\n}\n",
		"outputs.tf.json": "{}\n",
		"versions.hcl":    "# not configuration\n",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(module, name), []byte(content), 0o644))
	}
}

// Only the input the unit declares sensitive is reported, even though Terragrunt lists the module's
// Terraform files among the files it read. Before the fix, those unread files made every input sensitive.
func TestResolveReportsOnlyDeclaredSensitiveInputsWithModuleFiles(t *testing.T) {
	request := nativeUnit(t, "locals {\n  password = sensitive(\"not-a-real-password\")\n}\n"+
		"inputs = {\n  large = 9007199254740993\n  name = \"visible\"\n  object = { name = \"x\", count = 3 }\n  password = local.password\n}\n")
	writeModuleFiles(t, request.BasePath)

	for _, command := range []string{"apply", "plan", "output"} {
		t.Run(command, func(t *testing.T) {
			request.Command = command
			sink := NewLogSink("trace", request.BasePath)

			result, err := Resolve(WithLogSink(t.Context(), sink), request)

			require.NoError(t, err)
			assert.Equal(t, [][]string{{"password"}}, result.SensitiveInputs)
			entries := sink.Entries()
			require.NotEmpty(t, entries)
			for _, entry := range entries {
				assert.NotContains(t, entry.Message, "withheld", "unread module files must not withhold the logs")
				assert.NotContains(t, entry.Message, "not-a-real-password")
			}
		})
	}
}

// Included files and files read with read_terragrunt_config are opened while the unit is evaluated, so
// their sensitive declarations are still found once unread files are skipped.
func TestResolveFindsSensitiveInputsOfIncludedAndReadConfiguration(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "root.hcl"), []byte("inputs = { parent_token = sensitive(\"parent-secret-1\") }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "shared.hcl"), []byte("locals { token = sensitive(\"shared-secret-2\") }\n"), 0o644))
	unit := filepath.Join(root, "unit")
	require.NoError(t, os.MkdirAll(unit, 0o755))
	request := nativeUnitAt(t, unit, "include \"root\" {\n  path = find_in_parent_folders(\"root.hcl\")\n}\n"+
		"locals {\n  shared = read_terragrunt_config(\"${get_terragrunt_dir()}/../shared.hcl\")\n}\n"+
		"inputs = {\n  shared_token = local.shared.locals.token\n  name = \"visible\"\n}\n")
	request.BasePath = root
	writeModuleFiles(t, unit)

	result, err := Resolve(t.Context(), request)

	require.NoError(t, err)
	assert.ElementsMatch(t, [][]string{{"parent_token"}, {"shared_token"}}, result.SensitiveInputs)
}

// A file Terragrunt only marked as read was never opened, so it declares nothing. A file it opened
// but did not finish reading still cannot be vouched for, and neither can the unit itself.
func TestEvaluatedViewSkipsFilesThatWereNeverOpened(t *testing.T) {
	disk := afero.NewMemMapFs()
	unit, module, partial := "/repo/unit.hcl", "/repo/module/.terraform.lock.hcl", "/repo/partial.hcl"
	writeFiles(t, disk, map[string]string{unit: "inputs = { token = sensitive(\"abc-secret\"), name = \"visible\" }\n", module: "provider \"x\" {}\n", partial: "abcdef"})
	recording := newEvaluatedFS(disk)
	readThrough(t, recording, unit)
	opened, err := recording.Open(partial)
	require.NoError(t, err)
	_, err = opened.Read(make([]byte, 2))
	require.NoError(t, err)
	require.NoError(t, opened.Close())

	view := recording.Evaluated()

	_, err = view.Stat(module)
	require.ErrorIs(t, err, errNotRead)
	require.ErrorIs(t, err, os.ErrNotExist, "an unread file reads as missing to every check")
	_, err = view.Stat(partial)
	require.ErrorIs(t, err, errNotEvaluated, "an unfinished read stays unverifiable")
	inputs := map[string]any{"token": "abc-secret", "name": "visible"}
	assert.Equal(t, [][]string{{"token"}}, sensitiveInputs(view, []string{module, unit}, inputs, unit, nil))
	assert.False(t, unredactedSensitivity(view, []string{module, unit}, unit, true))
	assert.Contains(t, sensitiveInputs(view, []string{partial, unit}, inputs, unit, nil), []string(nil), "an unfinished read protects every input")

	// The unit is always opened by its evaluation, so an unread unit still protects every input.
	unread := newEvaluatedFS(disk).Evaluated()
	assert.True(t, unverifiable(unread, unit))
	assert.Equal(t, [][]string{nil}, sensitiveInputs(unread, []string{unit}, inputs, unit, nil))
	assert.True(t, unredactedSensitivity(unread, []string{unit}, unit, true))
}

// Terraform and OpenTofu files are module sources, never Terragrunt configuration: even when Terragrunt
// opened them, for example while copying the module, a JSON or sensitive() module file changes nothing.
func TestSensitivitySkipsTerraformSourceFiles(t *testing.T) {
	disk := afero.NewMemMapFs()
	unit := "/repo/unit.hcl"
	files := map[string]string{
		unit:                       "inputs = { token = sensitive(\"abc-secret\"), name = \"visible\" }\n",
		"/repo/module/main.tf":     "output \"x\" { value = sensitive(var.y) }\n",
		"/repo/module/out.TF.JSON": "{}\n",
		"/repo/module/main.tofu":   "output \"x\" { value = sensitive(var.y) }\n",
	}
	writeFiles(t, disk, files)
	recording := newEvaluatedFS(disk)
	for name := range files {
		readThrough(t, recording, name)
	}
	view := recording.Evaluated()
	read := []string{"/repo/module/main.tf", "/repo/module/out.TF.JSON", "/repo/module/main.tofu", unit}

	assert.Equal(t, [][]string{{"token"}}, sensitiveInputs(view, read, map[string]any{"token": "abc-secret", "name": "visible"}, unit, nil))
	assert.False(t, unredactedSensitivity(view, read, unit, true))
	assert.False(t, moduleSourceFile("/repo/unit.tf", "/repo/unit.tf"), "the unit is configuration whatever its name")
	assert.True(t, moduleSourceFile("/repo/module/main.tf", ""))
	assert.False(t, moduleSourceFile("/repo/root.hcl", unit))
}

// An included file evaluates find_in_parent_folders from the unit's directory, as Terragrunt does, so a
// read_terragrunt_config of a plain file found that way is not sensitive. Before the fix, the search started
// above the included file, found nothing, and every value read from the file was masked.
func TestSensitiveInputsResolveParentSearchesFromTheUnit(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "live", "dev", "network", "terragrunt.hcl")
	parent := filepath.Join(root, "root.hcl")
	env := filepath.Join(root, "env.hcl")
	fsys := afero.NewMemMapFs()
	writeFiles(t, fsys, map[string]string{
		unit:   "include \"root\" {\n  path = find_in_parent_folders(\"root.hcl\")\n}\n",
		parent: "locals {\n  environment = read_terragrunt_config(find_in_parent_folders(\"env.hcl\"))\n}\ninputs = { region = local.environment.locals.region }\n",
		env:    "locals {\n  region = \"us-east-2\"\n}\n",
	})
	inputs := map[string]any{"region": "us-east-2"}

	assert.Empty(t, sensitiveInputs(fsys, []string{env, parent, unit}, inputs, unit, nil))

	writeFiles(t, fsys, map[string]string{env: "locals {\n  region = sensitive(\"us-east-2\")\n}\n"})
	assert.Equal(t, [][]string{{"region"}}, sensitiveInputs(fsys, []string{env, parent, unit}, inputs, unit, nil), "a sensitive local found from the unit is still sensitive")
	assert.Equal(t, [][]string{{"region"}}, sensitiveInputs(fsys, []string{parent, unit}, inputs, unit, nil), "a file that cannot be found fails closed")
}
