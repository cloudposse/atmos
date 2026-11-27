package cloudformation

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// init --apply re-executes the current binary. Record the arguments in that
// child process so this test never invokes a CloudFormation deployment.
func TestMain(m *testing.M) {
	if len(os.Args) >= 5 && slices.Equal(os.Args[1:4], []string{"aws", "cfn", "apply"}) {
		if err := json.NewEncoder(os.Stdout).Encode(os.Args[1:]); err != nil || os.Args[4] == "fail" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestInitTemplateCommand(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_BASE_PATH", "")
	require.NoError(t, os.WriteFile("queue.yaml", []byte("Resources: {}"), 0o600))
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := newInitCommand()
		cmd.Flags().StringP("stack", "s", "", "")
		cmd.Flags().Bool("dry-run", false, "")
		cmd.Flags().StringArray("profile", nil, "")
		cmd.SetArgs(args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.ExecuteContext(t.Context())
		return out.String(), err
	}
	for _, args := range [][]string{{"."}, {"missing"}, {"queue.yaml", "--name=2bad"}, {"queue.yaml", "-s", "../dev"}} {
		_, err := run(args...)
		require.Error(t, err)
	}
	out, err := run("queue.yaml", "--dry-run", "--apply")
	require.NoError(t, err)
	require.Contains(t, out, "AWS stack: queue")
	require.NoFileExists(t, "atmos.yaml")
	out, err = run("queue.yaml", "--apply", "--name=blue", "--name=green", "--name=blue", "-s", "dev", "--identity=ci")
	require.NoError(t, err)
	for _, name := range []string{"blue", "green"} {
		require.Contains(t, out, `["aws","cfn","apply","`+name+`","-s","dev","--identity=ci"]`)
	}
	require.FileExists(t, filepath.Join("stacks", "dev.yaml"))
	out, err = run("queue.yaml", "--name=blue", "-s", "dev")
	require.NoError(t, err)
	require.Contains(t, out, "Already configured.")
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.Flags().StringArray("profile", nil, "")
	require.NoError(t, cmd.Flags().Set("profile", "one"))
	require.NoError(t, cmd.Flags().Set("profile", "two"))
	var child bytes.Buffer
	cmd.SetOut(&child)
	require.ErrorContains(t, applyInitialized(cmd, "dev", []string{"blue", "fail", "later"}), "apply fail")
	require.Contains(t, child.String(), `"--profile=one","--profile=two"`)
	require.NotContains(t, child.String(), "later")
}

func TestInitManifestConflicts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dev.yaml")
	c := &schema.AtmosConfiguration{BasePathAbsolute: dir, CloudFormationDirAbsolutePath: dir, StacksBaseAbsolutePath: dir}
	c.Stacks.IncludedPaths = []string{"*.yaml"}
	info := schema.ConfigAndStacksInfo{Stack: "dev"}
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	register := func(path string, fresh bool) error {
		return registerTemplate(cmd, c, fresh, &info, templateSource{path, []string{"queue"}})
	}
	require.ErrorContains(t, register(p, false), "different files")
	c.Stacks.ExcludedPaths = []string{"dev.yaml"}
	require.ErrorContains(t, register("queue.yaml", false), "unique manifest")
	c.Stacks.ExcludedPaths = nil
	write := func(path, body string) { t.Helper(); require.NoError(t, os.WriteFile(path, []byte(body), 0o600)) }
	write(p, "name: elsewhere")
	_, err := initManifest(c, "dev")
	require.ErrorContains(t, err, "declares stack")
	write(p, "name: [")
	_, err = initManifest(c, "dev")
	require.Error(t, err)
	write(p, "vars: {}")
	c.Stacks.NamePattern = "{stage}"
	_, err = initManifest(c, "dev")
	require.ErrorContains(t, err, "naming convention")
	require.NoError(t, os.Remove(p))
	tpl := filepath.Join(dir, "queue.json")
	write(filepath.Join(dir, "atmos.yaml"), "# existing")
	require.ErrorContains(t, register(tpl, true), "existing project configuration")
	require.NoError(t, register(tpl, false))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Contains(t, string(data), "name: dev")
	c.Stacks.IncludedPaths = []string{"*.yml", "*.yaml"}
	write(filepath.Join(dir, "dev.yml"), "{}")
	_, err = initManifest(c, "dev")
	require.ErrorContains(t, err, "unique manifest")
}

func TestInitInheritedManifest(t *testing.T) {
	dir := t.TempDir()
	c := &schema.AtmosConfiguration{CloudFormationDirAbsolutePath: dir, StacksBaseAbsolutePath: dir, IncludeStackAbsolutePaths: []string{filepath.Join(dir, "*.yaml")}}
	path := filepath.Join(dir, "west.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: dev"), 0o600))
	orig := cfnDescribeStacks
	t.Cleanup(func() { cfnDescribeStacks = orig })
	components := map[string]any{}
	var failure error
	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return map[string]any{"dev": map[string]any{"components": components}}, failure
	}
	components[cfg.CloudFormationComponentType] = map[string]any{"inherited": map[string]any{"atmos_stack_file": "west"}, "incomplete": map[string]any{}}
	file, names, err := existingManifest(c, "dev")
	require.NoError(t, err)
	require.Equal(t, path, file)
	require.True(t, names["inherited"])
	cmd := &cobra.Command{}
	require.ErrorContains(t, registerTemplate(cmd, c, false, &schema.ConfigAndStacksInfo{Stack: "dev"}, templateSource{filepath.Join(dir, "queue.yaml"), []string{"inherited"}}), "defined by an import")
	components["terraform"] = map[string]any{"other": map[string]any{"atmos_stack_file": "east"}}
	_, _, err = existingManifest(c, "dev")
	require.ErrorContains(t, err, "multiple manifests")
	clear(components)
	file, _, err = existingManifest(c, "dev")
	require.NoError(t, err)
	require.Empty(t, file)
	failure = errors.New("inventory unavailable")
	_, _, err = existingManifest(c, "dev")
	require.ErrorIs(t, err, failure)
}
