package terraform

// These tests run real subprocesses through the host's process facility. The test binary is
// every executable: it is Terraform (info.Command), each hook, and a toolchain-only tool, so
// nothing platform-specific is needed. A child records its invocation as one JSON line, which
// gives an ordered timeline of everything the pipeline started.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	testEnvHookChild = "_ATMOS_TEST_HOOK_CHILD"
	testEnvHookLog   = "_ATMOS_TEST_HOOK_LOG"
	// The child whose first argument equals this value exits 1.
	testEnvHookFail = "_ATMOS_TEST_HOOK_FAIL"
)

// hookRecord is what a child observed about its own invocation.
type hookRecord struct {
	Executable string            `json:"executable"`
	Arguments  []string          `json:"arguments"`
	Directory  string            `json:"directory"`
	Path       string            `json:"path"`
	Env        map[string]string `json:"env"`
}

// runHookChildForTest records the invocation and then acts on scripted arguments:
// exit=N, stdout=TEXT, stderr=TEXT, sleep=MILLISECONDS.
func runHookChildForTest() int {
	args := os.Args[1:]
	directory, _ := os.Getwd()
	executable, _ := os.Executable()
	record := hookRecord{Executable: executable, Arguments: args, Directory: directory, Path: os.Getenv("PATH"), Env: map[string]string{}}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok && (strings.HasPrefix(key, "TG_CTX_") || strings.HasPrefix(key, "HOOKTEST_")) {
			record.Env[key] = value
		}
	}
	if path := os.Getenv(testEnvHookLog); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 1
		}
		_ = json.NewEncoder(file).Encode(record)
		_ = file.Close()
	}
	if len(args) > 0 && args[0] == os.Getenv(testEnvHookFail) {
		fmt.Fprintf(os.Stderr, "child %s failed with Error: intentional\n", args[0])
		return 1
	}
	code := 0
	for _, token := range args {
		key, value, _ := strings.Cut(token, "=")
		switch key {
		case "exit":
			code, _ = strconv.Atoi(value)
		case "stdout":
			fmt.Fprintln(os.Stdout, value)
		case "stderr":
			fmt.Fprintln(os.Stderr, value)
		case "sleep":
			ms, _ := strconv.Atoi(value)
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
	return code
}

// readHookLogQuietly is for goroutines, which must not fail the test directly.
func readHookLogQuietly(path string) []hookRecord {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var records []hookRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var record hookRecord
		if decoder.Decode(&record) != nil {
			break
		}
		records = append(records, record)
	}
	return records
}

func readHookLog(t *testing.T, path string) []hookRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var records []hookRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var record hookRecord
		require.NoError(t, decoder.Decode(&record))
		records = append(records, record)
	}
	return records
}

// timeline renders each record as its first argument, which names the Terraform phase or hook.
func timeline(records []hookRecord) []string {
	var result []string
	for _, record := range records {
		result = append(result, record.Arguments[0])
	}
	return result
}

// installHookTool places a copy of the test binary as an executable that only the toolchain PATH can reach.
func installHookTool(t *testing.T, dir, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	executable, err := os.Executable()
	require.NoError(t, err)
	target := filepath.Join(dir, name)
	if err := os.Link(executable, target); err == nil {
		return
	}
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, content, 0o755))
}

type processPipeline struct {
	info    *schema.ConfigAndStacksInfo
	execCtx *componentExecContext
	log     string
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	toolDir string
}

// newProcessPipeline builds a component whose Terraform and hooks are all the test binary.
func newProcessPipeline(t *testing.T, execution *schema.ComponentExecution) *processPipeline {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	log := filepath.Join(t.TempDir(), "log.jsonl")
	toolDir := filepath.Join(t.TempDir(), "toolchain", "bin")
	installHookTool(t, toolDir, "hook-tool")
	env := []string{
		testEnvHookChild + "=1", testEnvHookLog + "=" + log,
		// The toolchain directory exists only in the invocation's PATH, never in the ambient one.
		"PATH=" + toolDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOOKTEST_STACK_ENV=from-stack",
	}
	return &processPipeline{
		log: log, toolDir: toolDir, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{},
		info: &schema.ConfigAndStacksInfo{
			SubCommand: "plan", Command: exe, TerraformWorkspace: "dev", ComponentEnvList: env,
			ComponentExecution: execution,
		},
		execCtx: &componentExecContext{componentPath: dir, workingDir: dir, varFile: "vars.tfvars", planFile: "plan.tfplan"},
	}
}

func (p *processPipeline) run(t *testing.T, opts ...component.ExecutionOption) error {
	t.Helper()
	streams := process.Streams{Stdout: p.stdout, Stderr: p.stderr}
	opts = append([]component.ExecutionOption{component.WithProcessStreams(streams)}, opts...)
	return testExecutor(t).executeCommandPipeline(&schema.AtmosConfiguration{}, p.info, p.execCtx, opts...)
}

func toolHook(name string, commands []string, args ...string) schema.ComponentHook {
	return schema.ComponentHook{Name: name, Commands: commands, Execute: append([]string{"hook-tool", name}, args...)}
}

func TestPipelineRunsHooksAroundEachPhaseSeparately(t *testing.T) {
	scripted := toolHook("plan-before", []string{"plan"}, "stdout=hook says hello")
	pipeline := newProcessPipeline(t, &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{
			{Commands: []string{"init"}, Arguments: []string{"-from-init-extra"}},
			{Commands: []string{"plan"}, Arguments: []string{"-from-plan-extra"}, Environment: map[string]string{"HOOKTEST_PLAN_EXTRA": "plan"}},
		},
		BeforeHooks: []schema.ComponentHook{toolHook("init-before", []string{"init"}), scripted, toolHook("workspace-before", []string{"workspace"})},
		AfterHooks:  []schema.ComponentHook{toolHook("init-after", []string{"init"}), toolHook("plan-after", []string{"plan"})},
	})
	require.NoError(t, pipeline.run(t))

	records := readHookLog(t, pipeline.log)
	require.Equal(t, []string{
		"init-before", "init", "init-after",
		"workspace", // Workspace selection is not a hook phase.
		"plan-before", "plan", "plan-after",
	}, timeline(records))
	require.Equal(t, []string{"init", "-from-init-extra"}, records[1].Arguments[:2])
	require.Equal(t, []string{"plan", "-from-plan-extra"}, records[5].Arguments[:2])

	exe, err := os.Executable()
	require.NoError(t, err)
	initRecord, planBefore, planRecord := records[1], records[4], records[5]
	require.Equal(t, filepath.Base(exe), filepath.Base(initRecord.Executable))
	require.Equal(t, physicalPath(t, pipeline.execCtx.componentPath), physicalPath(t, planBefore.Directory), "hooks run in the module directory by default")
	require.Equal(t, "plan", planBefore.Env["TG_CTX_COMMAND"])
	require.Equal(t, "plan-before", planBefore.Env["TG_CTX_HOOK_NAME"])
	require.Equal(t, exe, planBefore.Env["TG_CTX_TF_PATH"])
	require.Equal(t, "from-stack", planBefore.Env["HOOKTEST_STACK_ENV"], "hooks see the stack and component env")
	require.Equal(t, "plan", planRecord.Env["HOOKTEST_PLAN_EXTRA"])
	require.Equal(t, "init", records[0].Env["TG_CTX_COMMAND"])
	require.Empty(t, records[0].Env["HOOKTEST_PLAN_EXTRA"], "plan env_vars must not apply to the implicit init")
	require.Contains(t, planBefore.Path, pipeline.toolDir, "hooks resolve tools on the invocation's toolchain PATH")

	require.Contains(t, pipeline.stderr.String(), "hook says hello", "hook stdout belongs on the UI channel")
	require.NotContains(t, pipeline.stdout.String(), "hook says hello", "hook stdout must not mix into data stdout")
}

func physicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return filepath.Clean(resolved)
}

func TestPipelineSuppressStdoutKeepsHookOutputOffEveryTerminalStream(t *testing.T) {
	hook := toolHook("quiet", []string{"plan"}, "stdout=secret-looking-output")
	hook.SuppressStdout = true
	pipeline := newProcessPipeline(t, &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{hook}})
	require.NoError(t, pipeline.run(t))
	require.NotContains(t, pipeline.stdout.String(), "secret-looking-output")
	require.NotContains(t, pipeline.stderr.String(), "secret-looking-output")
}

func TestPipelineFailingBeforeHookAbortsWithItsExitCode(t *testing.T) {
	pipeline := newProcessPipeline(t, &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{toolHook("plan-before", []string{"plan"}, "exit=23", "stderr=hook failed loudly")},
		AfterHooks:  []schema.ComponentHook{toolHook("plan-after", []string{"plan"})},
	})
	err := pipeline.run(t)
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	require.Equal(t, 23, resolveExitCode(err))
	require.Equal(t, []string{"init", "workspace", "plan-before"}, timeline(readHookLog(t, pipeline.log)))
	require.Contains(t, pipeline.stderr.String(), "hook failed loudly")
}

func TestPipelineErrorHookRunsOnMatchingFailureOnly(t *testing.T) {
	for _, test := range []struct {
		name     string
		pattern  string
		wantHook bool
	}{
		{"matching failure text", "intentional", true},
		{"non matching failure text", "something else entirely", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pipeline := newProcessPipeline(t, &schema.ComponentExecution{
				AfterHooks: []schema.ComponentHook{toolHook("skipped-after", []string{"plan"})},
				ErrorHooks: []schema.ComponentHook{
					{Name: "recover", Commands: []string{"plan"}, Execute: []string{"hook-tool", "recover"}, OnErrors: []string{test.pattern}},
					{Name: "other-command", Commands: []string{"apply"}, Execute: []string{"hook-tool", "other"}, OnErrors: []string{".*"}},
				},
			})
			pipeline.info.ComponentEnvList = append(pipeline.info.ComponentEnvList, testEnvHookFail+"=plan")
			err := pipeline.run(t)
			require.Equal(t, 1, resolveExitCode(err), "the command's own exit code survives")
			names := timeline(readHookLog(t, pipeline.log))
			if test.wantHook {
				require.Equal(t, []string{"init", "workspace", "plan", "recover"}, names)
			} else {
				require.Equal(t, []string{"init", "workspace", "plan"}, names)
			}
		})
	}
}

func TestPipelineDryRunNeverStartsAProcess(t *testing.T) {
	pipeline := newProcessPipeline(t, &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{toolHook("plan-before", []string{"plan"})},
		AfterHooks:  []schema.ComponentHook{toolHook("plan-after", []string{"plan"})},
	})
	pipeline.info.DryRun = true
	require.NoError(t, pipeline.run(t))
	require.Empty(t, readHookLog(t, pipeline.log), "dry-run must not run hooks (or Terraform)")
}

func TestPipelineCancellationStopsAHookAndEverythingAfterIt(t *testing.T) {
	pipeline := newProcessPipeline(t, &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{toolHook("slow", []string{"plan"}, "sleep=60000"), toolHook("never", []string{"plan"})},
		AfterHooks:  []schema.ComponentHook{{Name: "cleanup", Commands: []string{"plan"}, Execute: []string{"hook-tool", "cleanup"}, RunOnError: true}},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	started := time.Now()
	go func() {
		// Cancel as soon as the slow hook has started; the safety deadline above only bounds a broken run.
		for ctx.Err() == nil {
			if slices.Contains(timeline(readHookLogQuietly(pipeline.log)), "slow") {
				cancel()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	err := pipeline.run(t, component.WithProcessContext(ctx))
	require.Error(t, err)
	require.Less(t, time.Since(started), 50*time.Second, "the sleeping hook must be stopped, not waited for")
	names := timeline(readHookLog(t, pipeline.log))
	require.Equal(t, "slow", names[len(names)-1], "nothing may start after the canceled hook: %v", names)
}
