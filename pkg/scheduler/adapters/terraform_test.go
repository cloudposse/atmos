package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteTerraformAllUsesGraphBackedSequentialOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc@dev", "database@dev", "app@dev"}, executed)
}

func TestExecuteTerraformComponentsUsesGraphBackedSequentialOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Components: []string{"app", "database", "vpc"},
			Stack:      "dev",
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc@dev", "database@dev", "app@dev"}, executed)
}

func TestExecuteTerraformQueryUsesGraphBackedSequentialOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Query:      ".vars.group == \"selected\"",
			Stack:      "dev",
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc@dev", "database@dev", "app@dev"}, executed)
}

func TestExecuteTerraformDestroyUsesReverseDependencyOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "destroy",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"app@dev", "database@dev", "vpc@dev"}, executed)
}

func TestBuildTerraformGraphPrefersDependenciesComponentsOverSettingsDependsOn(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc":      terraformAdapterComponent("selected", nil, nil),
					"database": terraformAdapterComponent("selected", nil, nil),
					"app": terraformAdapterComponent(
						"selected",
						[]any{map[string]any{"component": "vpc"}},
						[]any{map[string]any{"component": "database"}},
					),
				},
			},
		},
	}

	graph, err := BuildTerraformGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode("app-dev")
	require.True(t, ok)
	require.Equal(t, []string{"vpc-dev"}, app.Dependencies)
}

func TestBuildTerraformGraphFallsBackToSettingsDependsOn(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc": terraformAdapterComponent("selected", nil, nil),
					"app": terraformAdapterComponent(
						"selected",
						nil,
						[]any{map[string]any{"component": "vpc"}},
					),
				},
			},
		},
	}

	graph, err := BuildTerraformGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode("app-dev")
	require.True(t, ok)
	require.Equal(t, []string{"vpc-dev"}, app.Dependencies)
}

func TestExecuteTerraformKeepsIndependentComponentsSequential(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
					"database": terraformAdapterComponentWithPath("selected", terraformAdapterPath("database")),
					"vpc":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("vpc")),
				},
			},
		},
	}

	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, maxActive.Load())
}

func TestExecuteTerraformAllowsParallelPlanForDifferentPhysicalComponentPaths(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
					"database": terraformAdapterComponentWithPath("selected", terraformAdapterPath("database")),
					"vpc":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("vpc")),
				},
			},
		},
	}

	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "plan",
			MaxConcurrency: 3,
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Greater(t, maxActive.Load(), int32(1))
}

func TestExecuteTerraformAllowsParallelPlanForSharedPhysicalComponentPathWhenWorkdirEnabled(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"service-api":    terraformAdapterComponentWithPath("selected", terraformAdapterPath("shared-service")),
					"service-worker": terraformAdapterComponentWithPath("selected", terraformAdapterPath("shared-service")),
					"service-cron":   terraformAdapterComponentWithPath("selected", terraformAdapterPath("shared-service")),
				},
			},
		},
	}

	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "plan",
			MaxConcurrency: 3,
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Greater(t, maxActive.Load(), int32(1))
}

func TestTerraformResourceKeyUsesWorkdirIdentityForWorkdirEnabledAliases(t *testing.T) {
	componentPath := terraformAdapterPath("shared-service")
	serviceAPI := &dependency.Node{
		Component: "service-api",
		Stack:     "dev",
		Metadata:  terraformAdapterComponentWithPath("selected", componentPath),
	}
	serviceWorker := &dependency.Node{
		Component: "service-worker",
		Stack:     "dev",
		Metadata:  terraformAdapterComponentWithPath("selected", componentPath),
	}

	require.Equal(t, "workdir:dev/service-api", terraformResourceKey(serviceAPI))
	require.Equal(t, "workdir:dev/service-worker", terraformResourceKey(serviceWorker))
}

func TestTerraformResourceKeyUsesSharedPathWithoutWorkdir(t *testing.T) {
	componentPath := terraformAdapterPath("shared-service")
	serviceAPI := &dependency.Node{
		Component: "service-api",
		Stack:     "dev",
		Metadata:  terraformAdapterComponentWithPathNoWorkdir("selected", componentPath),
	}
	serviceWorker := &dependency.Node{
		Component: "service-worker",
		Stack:     "dev",
		Metadata:  terraformAdapterComponentWithPathNoWorkdir("selected", componentPath),
	}

	require.Equal(t, "path:"+componentPath, terraformResourceKey(serviceAPI))
	require.Equal(t, terraformResourceKey(serviceAPI), terraformResourceKey(serviceWorker))
}

func TestTerraformResourceKeyFallbacks(t *testing.T) {
	require.Empty(t, terraformResourceKey(nil))

	require.Equal(t, "component:from-field", terraformResourceKey(&dependency.Node{
		Component: "node-component",
		Metadata: map[string]any{
			cfg.ComponentSectionName: "from-field",
		},
	}))

	require.Equal(t, "component:from-metadata", terraformResourceKey(&dependency.Node{
		Component: "node-component",
		Metadata: map[string]any{
			cfg.MetadataSectionName: map[string]any{
				cfg.ComponentSectionName: "from-metadata",
			},
		},
	}))

	require.Equal(t, "component:node-component", terraformResourceKey(&dependency.Node{
		Component: "node-component",
		Metadata:  map[string]any{},
	}))
}

func TestTerraformExecutionResultCombinedOutput(t *testing.T) {
	require.Equal(t, "stdout", TerraformExecutionResult{Stdout: "stdout"}.CombinedOutput())
	require.Equal(t, "stderr", TerraformExecutionResult{Stderr: "stderr"}.CombinedOutput())
	require.Equal(t, "stdout\nstderr", TerraformExecutionResult{Stdout: "stdout", Stderr: "stderr"}.CombinedOutput())
}

func TestTerraformOutputConfiguration(t *testing.T) {
	output, err := newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{}, 1)
	require.NoError(t, err)
	require.Nil(t, output)

	output, err = newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{
		TerraformPlanHideNoChanges: true,
	}, 1)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.True(t, output.captureOutput())
	require.Equal(t, terraformPlanLogOrderGrouped, output.logOrder)

	_, err = newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{
		TerraformPlanLogOrder: "invalid",
	}, 2)
	require.Error(t, err)
}

func TestTerraformOutputNodeWritersWriteGroupedLogFiles(t *testing.T) {
	tmpDir := t.TempDir()
	output, err := newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: tmpDir}, &schema.ConfigAndStacksInfo{
		TerraformPlanLogOrder: terraformPlanLogOrderGrouped,
	}, 2)
	require.NoError(t, err)

	stdout, stderr, flush, logFiles := output.nodeWriters(&dependency.Node{
		Component: "app",
		Stack:     "dev",
	})
	require.NotNil(t, stdout)
	require.NotNil(t, stderr)
	require.NotNil(t, flush)
	require.Contains(t, logFiles, "stdout")
	require.Contains(t, logFiles, "stderr")

	_, err = stdout.Write([]byte("stdout"))
	require.NoError(t, err)
	_, err = stderr.Write([]byte("stderr"))
	require.NoError(t, err)
	require.NoError(t, flush())

	stdoutContent, err := os.ReadFile(logFiles["stdout"])
	require.NoError(t, err)
	require.Equal(t, "stdout", string(stdoutContent))
	stderrContent, err := os.ReadFile(logFiles["stderr"])
	require.NoError(t, err)
	require.Equal(t, "stderr", string(stderrContent))
}

func TestTerraformOutputNodeWritersStreamMode(t *testing.T) {
	tmpDir := t.TempDir()
	output, err := newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: tmpDir}, &schema.ConfigAndStacksInfo{
		TerraformPlanLogOrder: terraformPlanLogOrderStream,
	}, 2)
	require.NoError(t, err)

	stdout, stderr, flush, logFiles := output.nodeWriters(&dependency.Node{
		Component: "app",
		Stack:     "dev",
	})
	require.NotNil(t, stdout)
	require.NotNil(t, stderr)
	require.NotNil(t, flush)
	require.Contains(t, logFiles, "stdout")
	require.Contains(t, logFiles, "stderr")
	require.NoError(t, flush())
}

func TestTerraformOutputFinishNodeGroupedOutput(t *testing.T) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)
	stderrReader, stderrWriter, err := os.Pipe()
	require.NoError(t, err)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	output := &terraformOutput{logOrder: terraformPlanLogOrderGrouped}
	output.finishNode(&dependency.Node{Component: "app", Stack: "dev"}, TerraformExecutionResult{
		Stdout: "stdout",
		Stderr: "stderr",
	}, nil)

	require.NoError(t, stdoutWriter.Close())
	require.NoError(t, stderrWriter.Close())
	stdoutBytes, err := io.ReadAll(stdoutReader)
	require.NoError(t, err)
	stderrBytes, err := io.ReadAll(stderrReader)
	require.NoError(t, err)

	require.Contains(t, string(stdoutBytes), "stdout")
	require.Contains(t, string(stderrBytes), "terraform plan succeeded")
	require.Contains(t, string(stderrBytes), "stderr")
	require.Contains(t, string(stderrBytes), "end terraform plan output")
}

func TestTerraformOutputFinishNodeSkipsNoChangesWhenHidden(t *testing.T) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)
	stderrReader, stderrWriter, err := os.Pipe()
	require.NoError(t, err)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	output := &terraformOutput{
		logOrder:      terraformPlanLogOrderGrouped,
		hideNoChanges: true,
	}
	output.finishNode(&dependency.Node{Component: "app", Stack: "dev"}, TerraformExecutionResult{
		Stdout: "No changes. Your infrastructure matches the configuration.\n",
	}, nil)

	require.NoError(t, stdoutWriter.Close())
	require.NoError(t, stderrWriter.Close())
	stdoutBytes, err := io.ReadAll(stdoutReader)
	require.NoError(t, err)
	stderrBytes, err := io.ReadAll(stderrReader)
	require.NoError(t, err)
	require.Empty(t, stdoutBytes)
	require.Empty(t, stderrBytes)
}

func TestTerraformOutputHelpers(t *testing.T) {
	require.Equal(t, "", terraformLogDir(nil))
	require.Equal(t, "", terraformLogDir(&schema.AtmosConfiguration{}))
	require.Equal(t, filepath.Join("base", ".atmos", "logs", "terraform", "plan"), terraformLogDir(&schema.AtmosConfiguration{BasePath: "base"}))
	require.Equal(t, "terraform", safeTerraformLogName(" "))
	require.Equal(t, "dev__app_bad_name", safeTerraformLogName("dev/app:bad name"))

	var out bytes.Buffer
	replayGroupedOutput(&out, "")
	require.Empty(t, out.String())
	replayGroupedOutput(&out, "line")
	require.Equal(t, "line\n", out.String())

	var combined bytes.Buffer
	writer := combineWriters(nil, &combined)
	_, err := writer.Write([]byte("secondary"))
	require.NoError(t, err)
	require.Equal(t, "secondary", combined.String())

	require.NoError(t, closeTerraformLogFiles(nil)())
}

func TestTerraformSummaryHelperBranches(t *testing.T) {
	require.Equal(t, "terraform", terraformNodeLabel(nil))
	require.Equal(t, "app", terraformNodeLabel(&dependency.Node{Component: "app"}))
	require.Equal(t, "dev/app", terraformNodeLabel(&dependency.Node{Stack: "dev", Component: "app"}))

	require.Equal(t, 1, effectiveTerraformMaxConcurrency(nil))
	require.Equal(t, 1, effectiveTerraformMaxConcurrency(&schema.ConfigAndStacksInfo{SubCommand: "apply", MaxConcurrency: 4}))
	require.Equal(t, 1, effectiveTerraformMaxConcurrency(&schema.ConfigAndStacksInfo{SubCommand: "plan", MaxConcurrency: 1}))
	require.Equal(t, 4, effectiveTerraformMaxConcurrency(&schema.ConfigAndStacksInfo{SubCommand: "plan", MaxConcurrency: 4}))

	require.Equal(t, 0, terraformExitCode(nil))
	require.Equal(t, 2, terraformExitCode(errUtils.ExitCodeError{Code: 2}))
	require.Equal(t, 1, terraformExitCode(errors.New("failed")))

	require.False(t, terraformPlanChangedError(nil, errUtils.ExitCodeError{Code: 2}))
	require.False(t, terraformPlanChangedError(&schema.ConfigAndStacksInfo{SubCommand: "apply"}, errUtils.ExitCodeError{Code: 2}))
	require.False(t, terraformPlanChangedError(&schema.ConfigAndStacksInfo{SubCommand: "plan"}, errors.New("failed")))
	require.True(t, terraformPlanChangedError(&schema.ConfigAndStacksInfo{SubCommand: "plan"}, errUtils.ExitCodeError{Code: 2}))

	require.Equal(t, 0, processedCount(nil))
	require.Equal(t, 1, processedCount(&scheduler.AggregateResult{Results: []scheduler.Result{{Value: true}}}))
	require.False(t, terraformPlanChanged(nil))
	require.True(t, terraformPlanChanged(&scheduler.AggregateResult{Results: []scheduler.Result{{Value: TerraformNodeOutcome{Changed: true}}}}))
}

func TestExecuteTerraformIgnoresMaxConcurrencyForNonPlan(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
					"database": terraformAdapterComponentWithPath("selected", terraformAdapterPath("database")),
					"vpc":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("vpc")),
				},
			},
		},
	}

	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "apply",
			MaxConcurrency: 3,
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, maxActive.Load())
}

func TestExecuteTerraformConcurrentStreamLogOrderInjectsStreams(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app": terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
				},
			},
		},
	}

	var sawStreams bool
	var sawCapture bool
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                   true,
			SubCommand:            "plan",
			MaxConcurrency:        2,
			TerraformPlanLogOrder: terraformPlanLogOrderStream,
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			sawStreams = execution.Stdout != nil && execution.Stderr != nil && execution.Flush != nil
			sawCapture = execution.CaptureOutput
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.True(t, sawStreams)
	require.False(t, sawCapture)
}

func TestExecuteTerraformConcurrentGroupedLogOrderCapturesOutput(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app": terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
				},
			},
		},
	}

	var sawCapture bool
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                   true,
			SubCommand:            "plan",
			MaxConcurrency:        2,
			TerraformPlanLogOrder: terraformPlanLogOrderGrouped,
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			sawCapture = execution.CaptureOutput
			return TerraformExecutionResult{Stdout: "planned\n"}, nil
		},
	})

	require.NoError(t, err)
	require.True(t, sawCapture)
}

func TestExecuteTerraformRejectsUnsupportedLogOrder(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app": terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
				},
			},
		},
	}
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                   true,
			SubCommand:            "plan",
			MaxConcurrency:        2,
			TerraformPlanLogOrder: "unknown",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported Terraform plan log order")
}

func TestTerraformPlanHasNoChanges(t *testing.T) {
	require.True(t, terraformPlanHasNoChanges(TerraformExecutionResult{}, errUtils.ExitCodeError{Code: 0}))
	require.True(t, terraformPlanHasNoChanges(TerraformExecutionResult{
		Stdout: "No changes. Your infrastructure matches the configuration.\n",
	}, nil))
	require.True(t, terraformPlanHasNoChanges(TerraformExecutionResult{
		Stdout: "\x1b[0m\x1b[1mNo changes. Your infrastructure matches the configuration.\x1b[0m\n",
	}, nil))
	require.True(t, terraformPlanHasNoChanges(TerraformExecutionResult{
		Stdout: "No changes. Infrastructure is up-to-date.\n",
	}, nil))
	require.False(t, terraformPlanHasNoChanges(TerraformExecutionResult{
		Stdout: "Plan: 1 to add, 0 to change, 0 to destroy.\n",
	}, nil))
	require.False(t, terraformPlanHasNoChanges(TerraformExecutionResult{}, errUtils.ExitCodeError{Code: 2}))
	require.False(t, terraformPlanHasNoChanges(TerraformExecutionResult{}, errors.New("terraform failed")))
}

func TestTerraformHideNoChangesForcesGroupedLogOrder(t *testing.T) {
	output, err := newTerraformOutput(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{
		SubCommand:                 "plan",
		MaxConcurrency:             2,
		TerraformPlanLogOrder:      terraformPlanLogOrderStream,
		TerraformPlanHideNoChanges: true,
	}, 2)

	require.NoError(t, err)
	require.Equal(t, terraformPlanLogOrderGrouped, output.logOrder)
	require.True(t, output.hideNoChanges)
	require.True(t, output.captureOutput())
}

func TestExecuteTerraformTreatsPlanExitTwoAsChangedSuccess(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app":      terraformAdapterComponentWithPath("selected", terraformAdapterPath("app")),
					"database": terraformAdapterComponentWithPath("selected", terraformAdapterPath("database")),
				},
			},
		},
	}

	var executed []string
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "plan",
			MaxConcurrency: 2,
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, execution.Info.Component)
			if execution.Info.Component == "app" {
				return TerraformExecutionResult{}, errUtils.ExitCodeError{Code: 2}
			}
			return TerraformExecutionResult{}, nil
		},
	})

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 2, exitErr.Code)
	require.ElementsMatch(t, []string{"app", "database"}, executed)
}

func TestValidateTerraformConcurrentPlanRequiresWorkdir(t *testing.T) {
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "plan",
			MaxConcurrency: 2,
		},
		Stacks: terraformAdapterTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "requires provision.workdir.enabled=true")
}

func TestWriteTerraformSummaryUsesDeterministicResultOrder(t *testing.T) {
	tmpDir := t.TempDir()
	summaryFile := filepath.Join(tmpDir, "summary.json")

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{BasePathAbsolute: tmpDir},
		Info: &schema.ConfigAndStacksInfo{
			All:                      true,
			SubCommand:               "plan",
			MaxConcurrency:           1,
			TerraformPlanSummaryFile: summaryFile,
		},
		Stacks: terraformAdapterTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	data, err := os.ReadFile(summaryFile)
	require.NoError(t, err)
	require.Contains(t, string(data), `"node_id": "vpc-dev"`)
	require.Less(t, stringIndex(string(data), `"node_id": "vpc-dev"`), stringIndex(string(data), `"node_id": "database-dev"`))
	require.Less(t, stringIndex(string(data), `"node_id": "database-dev"`), stringIndex(string(data), `"node_id": "app-dev"`))
}

func TestWriteTerraformSummaryIncludesNodeTimings(t *testing.T) {
	tmpDir := t.TempDir()
	summaryFile := filepath.Join(tmpDir, "summary.json")

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{BasePathAbsolute: tmpDir},
		Info: &schema.ConfigAndStacksInfo{
			Components:               []string{"vpc"},
			Stack:                    "dev",
			SubCommand:               "plan",
			MaxConcurrency:           1,
			TerraformPlanSummaryFile: summaryFile,
		},
		Stacks: terraformAdapterTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			time.Sleep(2 * time.Millisecond)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	data, err := os.ReadFile(summaryFile)
	require.NoError(t, err)
	require.Contains(t, string(data), `"started_at":`)
	require.Contains(t, string(data), `"finished_at":`)
	require.Contains(t, string(data), `"duration_ms":`)

	var summary terraformSummary
	require.NoError(t, json.Unmarshal(data, &summary))
	require.Len(t, summary.Results, 1)
	require.NotEmpty(t, summary.Results[0].StartedAt)
	require.NotEmpty(t, summary.Results[0].FinishedAt)
	require.GreaterOrEqual(t, summary.Results[0].DurationMS, int64(1))
}

func TestTerraformNodeTimingsHandlesNilInputs(t *testing.T) {
	var nilTimings *terraformNodeTimings
	_, ok := nilTimings.Get("missing")
	require.False(t, ok)
	nilTimings.Start(&dependency.Node{ID: "ignored"})
	nilTimings.Complete(&dependency.Node{ID: "ignored"}, scheduler.Result{})

	timings := newTerraformNodeTimings()
	timings.Start(nil)
	timings.Complete(nil, scheduler.Result{})
	_, ok = timings.Get("missing")
	require.False(t, ok)
}

func TestWriteTerraformSummaryHandlesNoopAndWriteError(t *testing.T) {
	require.NoError(t, writeTerraformSummary(nil, nil, nil))
	require.NoError(t, writeTerraformSummary(&schema.ConfigAndStacksInfo{}, nil, nil))

	tmpDir := t.TempDir()
	err := writeTerraformSummary(
		&schema.ConfigAndStacksInfo{TerraformPlanSummaryFile: tmpDir},
		&scheduler.AggregateResult{
			Results: []scheduler.Result{
				{
					NodeID: "vpc-dev",
					Node: dependency.Node{
						ID:        "vpc-dev",
						Component: "vpc",
						Stack:     "dev",
					},
					Status: scheduler.StatusSucceeded,
					Value:  TerraformNodeOutcome{Processed: true},
				},
			},
		},
		nil,
	)
	require.Error(t, err)
}

func updateMaxActive(maxActive *atomic.Int32, current int32) {
	for {
		previous := maxActive.Load()
		if current <= previous {
			return
		}
		if maxActive.CompareAndSwap(previous, current) {
			return
		}
	}
}

func stringIndex(value, substr string) int {
	idx := strings.Index(value, substr)
	if idx < 0 {
		return len(value)
	}
	return idx
}

func terraformAdapterTestStacks() map[string]any {
	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app": terraformAdapterComponent(
						"selected",
						[]any{map[string]any{"component": "database"}},
						nil,
					),
					"database": terraformAdapterComponent(
						"selected",
						[]any{map[string]any{"component": "vpc"}},
						nil,
					),
					"vpc": terraformAdapterComponent("selected", nil, nil),
				},
			},
		},
	}
}

func terraformAdapterComponent(group string, dependenciesComponents, settingsDependsOn []any) map[string]any {
	component := map[string]any{
		cfg.MetadataSectionName: map[string]any{
			"component": "mock",
		},
		"vars": map[string]any{
			"group": group,
		},
	}
	if dependenciesComponents != nil {
		component[cfg.DependenciesSectionName] = map[string]any{
			"components": dependenciesComponents,
		}
	}
	if settingsDependsOn != nil {
		component[cfg.SettingsSectionName] = map[string]any{
			"depends_on": settingsDependsOn,
		}
	}
	return component
}

func terraformAdapterComponentWithPath(group, componentPath string) map[string]any {
	component := terraformAdapterComponentWithPathNoWorkdir(group, componentPath)
	component["provision"] = map[string]any{
		"workdir": map[string]any{
			"enabled": true,
		},
	}
	return component
}

func terraformAdapterPath(parts ...string) string {
	segments := append([]string{"components", "terraform"}, parts...)
	return filepath.Join(segments...)
}

func terraformAdapterComponentWithPathNoWorkdir(group, componentPath string) map[string]any {
	component := terraformAdapterComponent(group, nil, nil)
	component["component_info"] = map[string]any{
		cfg.ComponentPathSectionName: componentPath,
	}
	return component
}
