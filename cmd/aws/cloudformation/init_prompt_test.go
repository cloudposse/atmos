package cloudformation

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestDiscoverTemplates(t *testing.T) {
	tree := fstest.MapFS{}
	for _, path := range []string{"b.json", "nested/c.TEMPLATE", "a.yml", ".cache/skip.yaml", "vendor/skip.yml", "node_modules/cdk/template.json", "cdk.out/template.cfn", "unrelated.txt"} {
		tree[path] = &fstest.MapFile{Data: []byte(`{"Resources":{}}`)}
	}
	tree["good.yaml"] = &fstest.MapFile{Data: []byte("Resources:\n  Queue:\n    Type: AWS::SQS::Queue\n    Properties: {QueueName: !Sub '${AWS::StackName}-queue'}\n")}
	tree["stack.yaml"] = &fstest.MapFile{Data: []byte("components: {terraform: {}}")}
	tree["broken.yaml"] = &fstest.MapFile{Data: []byte("Resources: [")}
	tree["scalar.yml"] = &fstest.MapFile{Data: []byte("Resources: S3")}
	tree["too-big.json"] = &fstest.MapFile{Data: []byte(`{"Resources":{}}` + strings.Repeat(" ", maxDiscoveryBytes))}
	tree["link.yaml"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("good.yaml")}
	paths, err := discoverTemplates(t.Context(), tree)
	require.NoError(t, err)
	require.Equal(t, []string{"a.yml", "b.json", "cdk.out/template.cfn", "good.yaml", "nested/c.TEMPLATE"}, paths)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = discoverTemplates(ctx, tree)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, hasTemplateResources(tree, "missing"))
}

func guidedInitCommand(t *testing.T) *cobra.Command {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_BASE_PATH", "")
	enableInitInteractive(t)
	cmd := newInitCommand()
	cmd.Flags().StringP("stack", "s", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	return cmd
}

func enableInitInteractive(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "true")
	t.Setenv("ATMOS_CI_WORKER_MODE", "false")
	env := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(env) })
	prev := viper.Get("interactive")
	viper.Set("interactive", true)
	t.Cleanup(func() { viper.Set("interactive", prev) })
}

func TestGuidedInitCommand(t *testing.T) {
	for _, tc := range []struct {
		name             string
		args             []string
		input, stack     string
		names            []string
		prompts, omitted []string
	}{
		{"discover and name twice", nil, "2\n2bad\nblue, green\n../bad\nprod\n", "prod", []string{"blue", "green"}, []string{"CloudFormation template", "AWS stack name(s)", "Atmos stack", "invalid AWS stack name", "invalid Atmos stack name"}, nil},
		{"accept defaults", []string{"queue.yaml"}, "\n\n", "dev", []string{"queue"}, []string{"AWS stack name(s)", "Atmos stack"}, []string{"CloudFormation template"}},
		{"explicit stack", []string{"queue.yaml", "-s", "prod"}, "green\n", "prod", []string{"green"}, []string{"AWS stack name(s)"}, []string{"Atmos stack", "CloudFormation template"}},
		{"explicit name", []string{"queue.yaml", "--name", "blue"}, "qa\n", "qa", []string{"blue"}, []string{"Atmos stack"}, []string{"AWS stack name(s)", "CloudFormation template"}},
		{"all explicit", []string{"queue.yaml", "--name", "blue", "-s", "qa"}, "", "qa", []string{"blue"}, nil, []string{"CloudFormation template", "AWS stack name(s)", "Atmos stack"}},
		{"ci uses defaults", []string{"queue.yaml", "--ci"}, "", "dev", []string{"queue"}, nil, []string{"AWS stack name(s)", "Atmos stack"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := guidedInitCommand(t)
			for _, path := range []string{"other.yaml", "queue.yaml"} {
				require.NoError(t, os.WriteFile(path, []byte("Resources: {}"), 0o600))
			}
			var out, ui bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&ui)
			cmd.SetIn(iotest.OneByteReader(strings.NewReader(tc.input)))
			prev := runInitForm
			runInitForm = func(c *cobra.Command, form *huh.Form) error { return prev(c, form.WithAccessible(true)) }
			t.Cleanup(func() { runInitForm = prev })
			cmd.SetArgs(append([]string{}, tc.args...))
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			data, err := os.ReadFile(filepath.Join("stacks", tc.stack+".yaml"))
			require.NoError(t, err)
			require.Contains(t, string(data), "queue.yaml")
			for _, name := range tc.names {
				require.Contains(t, out.String(), "AWS stack: "+name)
			}
			for _, prompt := range tc.prompts {
				require.Contains(t, ui.String(), prompt)
			}
			for _, prompt := range tc.omitted {
				require.NotContains(t, ui.String(), prompt)
			}
		})
	}
}

func TestGuidedInitErrorsWriteNothing(t *testing.T) {
	cmd := guidedInitCommand(t)
	cmd.SetContext(t.Context())
	_, err := selectInitTemplate(cmd, nil, false)
	require.ErrorContains(t, err, "provide a template path")
	_, err = selectInitTemplate(cmd, nil, true)
	require.ErrorContains(t, err, "no CloudFormation templates found")
	require.NoError(t, os.WriteFile("queue.yaml", []byte("Resources: {}"), 0o600))
	prev := runInitForm
	t.Cleanup(func() { runInitForm = prev })
	for _, args := range [][]string{nil, {"queue.yaml"}} {
		runInitForm = func(*cobra.Command, *huh.Form) error { return huh.ErrUserAborted }
		cmd.SetArgs(append([]string{}, args...))
		require.ErrorIs(t, cmd.ExecuteContext(t.Context()), errUtils.ErrUserAborted)
	}
	failure := errors.New("terminal closed")
	runInitForm = func(*cobra.Command, *huh.Form) error { return failure }
	cmd.SetArgs([]string{"queue.yaml"})
	require.ErrorIs(t, cmd.ExecuteContext(t.Context()), failure)
	require.NoFileExists(t, "atmos.yaml")
	require.NoDirExists(t, "stacks")
}

func TestExplicitInitTemplateValidation(t *testing.T) {
	cmd := guidedInitCommand(t)
	for file, body := range map[string]string{"pod.yaml": "apiVersion: v1", "broken.yaml": "Resources: [", "empty.json": "{}", "wrong.cfn": "Resources: []"} {
		require.NoError(t, os.WriteFile(file, []byte(body), 0o600))
		cmd.SetArgs([]string{file, "--ci"})
		err := cmd.ExecuteContext(t.Context())
		require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
		require.ErrorContains(t, err, file)
		require.ErrorContains(t, err, "top-level Resources mapping")
		require.NoFileExists(t, "atmos.yaml")
		require.NoDirExists(t, "stacks")
	}
	for file, body := range map[string]string{"large.txt": "Resources: {}\n#" + strings.Repeat("x", maxDiscoveryBytes), "intrinsics.cfn": "Resources: {Queue: {Type: 'AWS::SQS::Queue', Properties: {QueueName: !Sub '${AWS::StackName}-queue'}}}", "queue.json": `{"Resources":{"S":{"Type":"AWS::SQS::Queue"}}}`} {
		path := filepath.Join(t.TempDir(), file)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		cmd.SetArgs([]string{path, "--name", "queue", "--ci", "--dry-run"})
		require.NoError(t, cmd.ExecuteContext(t.Context()))
	}
}
