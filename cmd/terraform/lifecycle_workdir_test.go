package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

type lifecycleTestRecord struct {
	Event string
	Dir   string
}

// runLifecycleTestProcess is a portable Terraform/hook stand-in. It checks
// that provisioning has finished before the hook and that subsequent init
// and command execution preserve the hook's generated file in the same cwd.
func runLifecycleTestProcess() int {
	event := os.Getenv("_ATMOS_TEST_LIFECYCLE_EVENT")
	if event == "" {
		if len(os.Args) < 2 {
			return 1
		}
		switch os.Args[1] {
		case "version", "-version":
			fmt.Println(`{"terraform_version":"1.9.8"}`)
			return 0
		case "workspace":
			fmt.Println("default")
			return 0
		default:
			event = "terraform-" + os.Args[1]
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return 1
	}
	if !strings.HasPrefix(event, "terraform-") {
		cwdInfo, cwdErr := os.Stat(dir)
		componentInfo, componentErr := os.Stat(os.Getenv("ATMOS_COMPONENT_PATH"))
		if cwdErr != nil || componentErr != nil || !os.SameFile(cwdInfo, componentInfo) {
			fmt.Fprintln(os.Stderr, "hook cwd differs from ATMOS_COMPONENT_PATH")
			return 1
		}
	}
	if event == "before" {
		if _, err := os.Stat("main.tf"); err != nil {
			fmt.Fprintln(os.Stderr, "hook ran before source provisioning:", err)
			return 1
		}
		if err := os.WriteFile("hook-generated.txt", []byte("ready"), 0o600); err != nil {
			return 1
		}
	} else if _, err := os.Stat("hook-generated.txt"); err != nil {
		fmt.Fprintln(os.Stderr, "hook artifact missing from execution directory:", err)
		return 1
	}
	log, err := os.OpenFile(os.Getenv("_ATMOS_TEST_LIFECYCLE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 1
	}
	err = json.NewEncoder(log).Encode(lifecycleTestRecord{Event: event, Dir: dir})
	_ = log.Close()
	if err != nil {
		return 1
	}
	if event == "before" && os.Getenv("_ATMOS_TEST_LIFECYCLE_FAIL_BEFORE") == "true" {
		return 7
	}
	if event == "terraform-"+os.Getenv("_ATMOS_TEST_LIFECYCLE_COMMAND") && os.Getenv("_ATMOS_TEST_LIFECYCLE_FAIL") == "true" {
		return 7
	}
	return 0
}

func writeLifecycleWorkdirFixture(t *testing.T, command string, source bool) string {
	t.Helper()
	root := t.TempDir()
	exe, err := os.Executable()
	require.NoError(t, err)
	module := filepath.Join(root, "components", "terraform", "module")
	if source {
		module = filepath.Join(root, "source")
	}
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("variable \"stage\" { type = string }\n"), 0o600))
	writeYAML := func(path string, data any) {
		t.Helper()
		content, err := yaml.Marshal(data)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, content, 0o600))
	}
	writeYAML(filepath.Join(root, "atmos.yaml"), map[string]any{
		"base_path": root,
		"components": map[string]any{"terraform": map[string]any{
			"base_path": "components/terraform", "command": exe, "auto_generate_backend_file": false,
		}},
		"stacks": map[string]any{"base_path": "stacks", "included_paths": []string{"test.yaml"}, "name_pattern": "{stage}"},
		"logs":   map[string]any{"level": "Off"},
	})
	components := map[string]any{}
	for _, name := range []string{"app1", "app2"} {
		hook := func(event, when string) map[string]any {
			phase := "after"
			if event == "before" {
				phase = "before"
			}
			return map[string]any{
				"kind": "command", "command": exe, "events": []string{phase + ".terraform." + command},
				"when": when, "on_failure": "fail",
				"env": map[string]string{"_ATMOS_TEST_LIFECYCLE_EVENT": event},
			}
		}
		config := map[string]any{
			"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			"hooks": map[string]any{
				"before": hook("before", "success"), "success": hook("success", "success"),
				"failure": hook("failure", "failure"), "always": hook("always", "always"),
			},
		}
		if source {
			config["source"] = map[string]any{"uri": module, "ttl": "0s"}
		} else {
			config["metadata"] = map[string]any{"component": "module"}
		}
		components[name] = config
	}
	writeYAML(filepath.Join(root, "stacks", "test.yaml"), map[string]any{
		"vars": map[string]any{"stage": "test"}, "components": map[string]any{"terraform": components},
	})
	return root
}

type lifecycleTestCommand struct {
	name   string
	cmd    *cobra.Command
	parser *flags.StandardParser
}

func TestTerraformLifecycleWorkdir(t *testing.T) {
	for _, tc := range []lifecycleTestCommand{
		{"destroy", destroyCmd, destroyParser},
		{"init", initCmd, initParser},
		{"test", testCmd, testParser},
	} {
		for _, selection := range []string{"single", "all", "components"} {
			if selection == "all" && tc.name == "test" {
				continue // test supports component selectors, but does not expose --all.
			}
			for _, source := range []bool{false, true} {
				for _, failure := range []string{"none", "command", "before"} {
					t.Run(fmt.Sprintf("%s/%s/source=%t/failure=%s", tc.name, selection, source, failure), func(t *testing.T) {
						testLifecycleWorkdir(t, tc, selection, source, failure)
					})
				}
			}
		}
	}
}

func testLifecycleWorkdir(t *testing.T, tc lifecycleTestCommand, selection string, source bool, failure string) {
	t.Helper()
	bulk := selection != "single"
	fail := failure != "none"
	root := writeLifecycleWorkdirFixture(t, tc.name, source)
	t.Chdir(root)
	withoutCIDetection(t)
	resetViperCI(t)
	e.ClearLastAuthContext()
	t.Cleanup(e.ClearLastAuthContext)
	t.Cleanup(func() { wasMultiComponentExecution = false; preResolvedComponent = "" })
	logPath := filepath.Join(root, "events.jsonl")
	t.Setenv("_ATMOS_TEST_LIFECYCLE_LOG", logPath)
	t.Setenv("_ATMOS_TEST_LIFECYCLE_COMMAND", tc.name)
	t.Setenv("_ATMOS_TEST_LIFECYCLE_FAIL", fmt.Sprint(failure == "command"))
	t.Setenv("_ATMOS_TEST_LIFECYCLE_FAIL_BEFORE", fmt.Sprint(failure == "before"))
	t.Setenv("TF_CLI_ARGS_destroy", "-auto-approve")
	cmd := &cobra.Command{Use: tc.name}
	cmd.Flags().String("base-path", "", "")
	cmd.Flags().StringSlice("config", nil, "")
	cmd.Flags().StringSlice("config-path", nil, "")
	cmd.Flags().StringSlice("profile", nil, "")
	terraformParser.RegisterFlags(cmd)
	tc.parser.RegisterFlags(cmd)
	// RunE binds these flags into the global Viper instance. Clear
	// selections afterward so later tests cannot inherit this run.
	t.Cleanup(func() {
		for _, name := range []string{"stack", "components", "all", "max-concurrency"} {
			flag := cmd.Flags().Lookup(name)
			if flag == nil {
				continue
			}
			value := ""
			if name == "all" {
				value = "false"
			}
			if name == "max-concurrency" {
				value = flag.DefValue
			}
			require.NoError(t, flag.Value.Set(value))
			flag.Changed = false
		}
	})
	originalParent := terraformCmd
	terraformCmd = cmd
	t.Cleanup(func() { terraformCmd = originalParent })
	require.NoError(t, cmd.Flags().Set("stack", "test"))
	args := []string{"app1"}
	if bulk {
		args = nil
		if selection == "all" {
			require.NoError(t, cmd.Flags().Set("all", "true"))
		} else {
			// Query dispatch is shared by --components/--query/stack selectors.
			require.NoError(t, cmd.Flags().Set("components", "app1,app2"))
		}
		if !fail && cmd.Flags().Lookup("max-concurrency") != nil {
			require.NoError(t, cmd.Flags().Set("max-concurrency", "2"))
		}
	}
	require.NoError(t, tc.cmd.PreRunE(cmd, args))
	_, err := os.Stat(logPath)
	require.True(t, os.IsNotExist(err), "Cobra must not fire hooks before provisioning")
	err = tc.cmd.RunE(cmd, args)
	if fail {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
		require.NoError(t, tc.cmd.PostRunE(cmd, args))
	}
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	sourceDir := filepath.Join(root, "components", "terraform", "module")
	if source {
		sourceDir = filepath.Join(root, "source")
	}
	assert.NoFileExists(t, filepath.Join(sourceDir, "hook-generated.txt"), "hooks must not modify the shared source directory")
	byDir := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record lifecycleTestRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		byDir[record.Dir] = append(byDir[record.Dir], record.Event)
	}
	if bulk && !fail {
		assert.Len(t, byDir, 2, "each component needs its own workdir and hooks")
	} else {
		require.NotEmpty(t, byDir)
	}
	for dir, events := range byDir {
		assert.Contains(t, filepath.ToSlash(dir), "/.workdir/")
		assert.Equal(t, "before", events[0])
		assert.Equal(t, 1, countLifecycleEvent(events, "before"))
		if failure == "before" {
			for _, event := range events {
				assert.NotContains(t, event, "terraform-", "a failed before-hook must prevent Terraform execution")
			}
		} else {
			assert.Equal(t, 1, countLifecycleEvent(events, "terraform-"+tc.name))
		}
		assert.Equal(t, 1, countLifecycleEvent(events, "always"))
		if fail {
			assert.Equal(t, 1, countLifecycleEvent(events, "failure"))
			assert.NotContains(t, events, "success")
		} else {
			assert.Equal(t, 1, countLifecycleEvent(events, "success"))
			assert.NotContains(t, events, "failure")
		}
	}
}

func countLifecycleEvent(events []string, event string) int {
	count := 0
	for _, candidate := range events {
		if candidate == event {
			count++
		}
	}
	return count
}
