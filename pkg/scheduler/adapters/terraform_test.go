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

func TestExecuteTerraformAffectedSelectionUsesGraphBackedPath(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Affected:   true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
		Selection: &TerraformSelection{
			NodeIDs: []string{"database-dev"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"database@dev"}, executed)
}

func TestExecuteTerraformAffectedSelectionIncludesDependentsWhenRequested(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Affected:   true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
		Selection: &TerraformSelection{
			NodeIDs:           []string{"database-dev"},
			IncludeDependents: true,
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"database@dev", "app@dev"}, executed)
}

func TestFilterTerraformGraphBySelectionDoesNotTreatDuplicatesAsAllNodes(t *testing.T) {
	graph, err := BuildTerraformGraph(terraformAdapterTestStacks())
	require.NoError(t, err)

	filtered := filterTerraformGraphBySelection(graph, &TerraformSelection{
		NodeIDs: []string{"database-dev", "database-dev", "missing-dev"},
	})

	require.Equal(t, 1, filtered.Size())
	_, ok := filtered.GetNode("database-dev")
	require.True(t, ok)
}

func TestFilterTerraformGraphBySelectionEdgeCases(t *testing.T) {
	graph, err := BuildTerraformGraph(terraformAdapterTestStacks())
	require.NoError(t, err)

	t.Run("nil graph returns empty graph", func(t *testing.T) {
		filtered := filterTerraformGraphBySelection(nil, &TerraformSelection{NodeIDs: []string{"database-dev"}})
		require.NotNil(t, filtered)
		require.Equal(t, 0, filtered.Size())
	})

	t.Run("nil selection returns empty graph", func(t *testing.T) {
		filtered := filterTerraformGraphBySelection(graph, nil)
		require.NotNil(t, filtered)
		require.Equal(t, 0, filtered.Size())
	})

	t.Run("all valid nodes without closure returns original graph", func(t *testing.T) {
		filtered := filterTerraformGraphBySelection(graph, &TerraformSelection{
			NodeIDs: []string{"app-dev", "database-dev", "vpc-dev"},
		})
		require.Same(t, graph, filtered)
	})

	t.Run("dependencies closure includes prerequisites", func(t *testing.T) {
		filtered := filterTerraformGraphBySelection(graph, &TerraformSelection{
			NodeIDs:             []string{"app-dev"},
			IncludeDependencies: true,
		})
		require.Equal(t, 3, filtered.Size())
		for _, id := range []string{"app-dev", "database-dev", "vpc-dev"} {
			_, ok := filtered.GetNode(id)
			require.True(t, ok, "expected node %s", id)
		}
	})

	t.Run("dependents closure includes downstream nodes", func(t *testing.T) {
		filtered := filterTerraformGraphBySelection(graph, &TerraformSelection{
			NodeIDs:           []string{"database-dev"},
			IncludeDependents: true,
		})
		require.Equal(t, 2, filtered.Size())
		for _, id := range []string{"database-dev", "app-dev"} {
			_, ok := filtered.GetNode(id)
			require.True(t, ok, "expected node %s", id)
		}
	})
}

func TestExecuteTerraformAffectedDestroyReversesSelectedDependents(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Affected:               true,
			SubCommand:             "destroy",
			MaxConcurrency:         2,
			AdditionalArgsAndFlags: []string{"-auto-approve"},
		},
		Stacks: stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
		Selection: &TerraformSelection{
			NodeIDs:           []string{"database-dev"},
			IncludeDependents: true,
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"app@dev", "database@dev"}, executed)
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
					"app":      terraformAdapterComponentWithPathNoWorkdir("selected", terraformAdapterPath("app")),
					"database": terraformAdapterComponentWithPathNoWorkdir("selected", terraformAdapterPath("database")),
					"vpc":      terraformAdapterComponentWithPathNoWorkdir("selected", terraformAdapterPath("vpc")),
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

func TestExecuteTerraformSerializesAliasesForSharedPhysicalComponentPathWithoutWorkdir(t *testing.T) {
	for _, subcommand := range []string{"plan", "apply", "destroy"} {
		t.Run(subcommand, func(t *testing.T) {
			stacks := map[string]any{
				"dev": map[string]any{
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformSectionName: map[string]any{
							"service-api":    terraformAdapterComponentWithPathNoWorkdir("selected", terraformAdapterPath("shared-service")),
							"service-worker": terraformAdapterComponentWithPathNoWorkdir("selected", terraformAdapterPath("shared-service")),
							"service-cron":   terraformAdapterComponentWithPathNoWorkdir("selected", terraformAdapterPath("shared-service")),
						},
					},
				},
			}

			var active atomic.Int32
			var maxActive atomic.Int32
			info := &schema.ConfigAndStacksInfo{
				All:            true,
				SubCommand:     subcommand,
				MaxConcurrency: 3,
			}
			if requiresTerraformAutoApprove(info) {
				info.AdditionalArgsAndFlags = []string{"-auto-approve"}
			}

			err := ExecuteTerraform(context.Background(), TerraformOptions{
				AtmosConfig: &schema.AtmosConfiguration{},
				Info:        info,
				Stacks:      stacks,
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
		})
	}
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

	require.Equal(t, "path:"+filepath.ToSlash(componentPath), terraformResourceKey(serviceAPI))
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

func TestTerraformExecutionErrorIncludesCapturedOutputDetail(t *testing.T) {
	err := terraformExecutionError(
		&dependency.Node{Component: "clickhouse-keeper-vm", Stack: "fuecoco-stg"},
		TerraformExecutionResult{
			Stderr: strings.Join([]string{
				"Initializing the backend...",
				"Error: Error acquiring the state lock",
				`writing "gs://nxtfwd-tf-state/clickhouse-keeper-vm/fuecoco-stg.tflock" failed`,
				"Lock Info:",
				"  ID: 1780498246824576",
			}, "\n"),
		},
		errors.New("subcommand exited with code 1"),
	)

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrTerraformExecFailed)
	require.Contains(t, err.Error(), "component=clickhouse-keeper-vm stack=fuecoco-stg")
	require.Contains(t, err.Error(), "subcommand exited with code 1")
	require.Contains(t, err.Error(), "terraform output:")
	require.Contains(t, err.Error(), "```text")
	require.Contains(t, err.Error(), "Error acquiring the state lock")
	require.Contains(t, err.Error(), "gs://nxtfwd-tf-state/clickhouse-keeper-vm/fuecoco-stg.tflock")
}

func TestTerraformOutputConfiguration(t *testing.T) {
	output, err := newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{}, 1)
	require.NoError(t, err)
	require.Nil(t, output)

	output, err = newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{
		SubCommand:                 "plan",
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

	output, err = newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{
		SubCommand:        "plan",
		TerraformPlanHide: []string{terraformPlanHideNoChanges},
	}, 1)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.True(t, output.hideNoChanges)
	require.Equal(t, terraformPlanLogOrderGrouped, output.logOrder)

	_, err = newTerraformOutput(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, &schema.ConfigAndStacksInfo{
		SubCommand:        "plan",
		TerraformPlanHide: []string{"refresh"},
	}, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported Terraform plan hide option")
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

	output := &terraformOutput{command: "plan", logOrder: terraformPlanLogOrderGrouped}
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
	require.Contains(t, string(stderrBytes), "plan output succeeded")
	require.Contains(t, string(stderrBytes), "stderr")
	require.Contains(t, string(stderrBytes), "end plan output")
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
	require.Equal(t, "", terraformLogDir(nil, "plan"))
	require.Equal(t, "", terraformLogDir(&schema.AtmosConfiguration{}, "plan"))
	require.Equal(t, filepath.Join("base", ".atmos", "logs", "terraform", "plan"), terraformLogDir(&schema.AtmosConfiguration{BasePath: "base"}, "plan"))
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
	require.Equal(t, 4, effectiveTerraformMaxConcurrency(&schema.ConfigAndStacksInfo{SubCommand: "apply", MaxConcurrency: 4}))
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

func TestExecuteTerraformAllowsParallelApplyWithAutoApprove(t *testing.T) {
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
			All:                    true,
			SubCommand:             "apply",
			MaxConcurrency:         3,
			AdditionalArgsAndFlags: []string{"-auto-approve"},
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

func TestExecuteTerraformRejectsConcurrentApplyWithoutAutoApprove(t *testing.T) {
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "apply",
			MaxConcurrency: 2,
		},
		Stacks: terraformAdapterTestStacksWithWorkdir(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "requires -auto-approve")
}

func TestExecuteTerraformFailsFastByDefault(t *testing.T) {
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "plan",
			MaxConcurrency: 1,
		},
		Stacks: terraformAdapterFailureModeStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, execution.Info.Component+"@"+execution.Info.Stack)
			return TerraformExecutionResult{}, errors.New("planned failure")
		},
	})

	require.Error(t, err)
	require.Equal(t, []string{"a-fail@dev"}, executed)
	require.Contains(t, err.Error(), "planned failure")
	require.Contains(t, err.Error(), "fail-fast after a-fail-dev failed")
}

func TestExecuteTerraformKeepGoingRunsIndependentNodes(t *testing.T) {
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                  true,
			SubCommand:           "plan",
			MaxConcurrency:       1,
			TerraformFailureMode: terraformFailureModeKeepGoing,
		},
		Stacks: terraformAdapterFailureModeStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, execution.Info.Component+"@"+execution.Info.Stack)
			if execution.Info.Component == "a-fail" {
				return TerraformExecutionResult{}, errors.New("planned failure")
			}
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Equal(t, []string{"a-fail@dev", "c-independent@dev"}, executed)
	require.Contains(t, err.Error(), "planned failure")
	require.Contains(t, err.Error(), "dependency a-fail-dev failed")
}

func TestExecuteTerraformRejectsConflictingFailureModes(t *testing.T) {
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
			FailFast:   true,
			KeepGoing:  true,
		},
		Stacks: terraformAdapterFailureModeStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), `failure mode cannot be both "fail-fast" and "keep-going"`)
}

func TestExecuteTerraformRejectsUnsupportedFailureMode(t *testing.T) {
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                  true,
			SubCommand:           "plan",
			TerraformFailureMode: "eventually",
		},
		Stacks: terraformAdapterFailureModeStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), `unsupported Terraform failure mode "eventually"`)
}

func TestTerraformFailureModeHelpers(t *testing.T) {
	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		wantMode      string
		wantFailFast  bool
		wantErrSubstr string
	}{
		{
			name:         "nil defaults to fail fast",
			info:         nil,
			wantMode:     terraformFailureModeFailFast,
			wantFailFast: true,
		},
		{
			name: "explicit keep going",
			info: &schema.ConfigAndStacksInfo{
				TerraformFailureMode: terraformFailureModeKeepGoing,
			},
			wantMode:     terraformFailureModeKeepGoing,
			wantFailFast: false,
		},
		{
			name: "legacy keep going fallback",
			info: &schema.ConfigAndStacksInfo{
				KeepGoing: true,
			},
			wantMode:     terraformFailureModeKeepGoing,
			wantFailFast: false,
		},
		{
			name: "explicit mode wins over legacy field",
			info: &schema.ConfigAndStacksInfo{
				TerraformFailureMode: terraformFailureModeFailFast,
				KeepGoing:            true,
			},
			wantMode:     terraformFailureModeFailFast,
			wantFailFast: true,
		},
		{
			name: "conflicting legacy fields are rejected",
			info: &schema.ConfigAndStacksInfo{
				FailFast:  true,
				KeepGoing: true,
			},
			wantMode:      terraformFailureModeKeepGoing,
			wantFailFast:  false,
			wantErrSubstr: "failure mode cannot be both",
		},
		{
			name: "unsupported explicit mode is rejected",
			info: &schema.ConfigAndStacksInfo{
				TerraformFailureMode: "eventually",
			},
			wantMode:      "eventually",
			wantFailFast:  false,
			wantErrSubstr: "unsupported Terraform failure mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantMode, effectiveTerraformFailureMode(tt.info))
			require.Equal(t, tt.wantFailFast, effectiveTerraformFailFast(tt.info))

			err := validateTerraformFailureMode(tt.info)
			if tt.wantErrSubstr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrSubstr)
		})
	}
}

func TestExecuteTerraformAcceptsDoubleDashAutoApprove(t *testing.T) {
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                    true,
			SubCommand:             "destroy",
			MaxConcurrency:         2,
			AdditionalArgsAndFlags: []string{"--auto-approve"},
		},
		Stacks: terraformAdapterTestStacksWithWorkdir(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
}

func TestContainsTerraformFlagParsesBooleanValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "bare flag",
			args: []string{"-auto-approve"},
			want: true,
		},
		{
			name: "double dash true",
			args: []string{"--auto-approve=true"},
			want: true,
		},
		{
			name: "numeric true",
			args: []string{"-auto-approve=1"},
			want: true,
		},
		{
			name: "yes true",
			args: []string{"-auto-approve=yes"},
			want: true,
		},
		{
			name: "false value",
			args: []string{"-auto-approve=false"},
			want: false,
		},
		{
			name: "numeric false",
			args: []string{"-auto-approve=0"},
			want: false,
		},
		{
			name: "no value",
			args: []string{"-auto-approve=no"},
			want: false,
		},
		{
			name: "unrelated flag",
			args: []string{"-lock=false"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, containsTerraformFlag(tt.args, "-auto-approve"))
		})
	}
}

func TestHasTerraformAutoApproveEnvChecksGlobalAndSubcommandArgs(t *testing.T) {
	t.Run("global", func(t *testing.T) {
		t.Setenv(terraformCLIArgsEnv, "-auto-approve")
		require.True(t, hasTerraformAutoApproveEnv("apply"))
	})

	t.Run("apply specific", func(t *testing.T) {
		t.Setenv(terraformCLIArgsEnvPrefix+"apply", "-auto-approve")
		require.True(t, hasTerraformAutoApproveEnv("apply"))
		require.False(t, hasTerraformAutoApproveEnv("destroy"))
	})

	t.Run("destroy specific", func(t *testing.T) {
		t.Setenv(terraformCLIArgsEnvPrefix+"destroy", "--auto-approve=true")
		require.True(t, hasTerraformAutoApproveEnv("destroy"))
		require.False(t, hasTerraformAutoApproveEnv("apply"))
	})

	t.Run("explicit false", func(t *testing.T) {
		t.Setenv(terraformCLIArgsEnvPrefix+"apply", "-auto-approve=false")
		require.False(t, hasTerraformAutoApproveEnv("apply"))
	})
}

func TestExecuteTerraformAllowsConcurrentApplyWithConfiguredAutoApprove(t *testing.T) {
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
	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					ApplyAutoApprove: true,
				},
			},
		},
		Info: &schema.ConfigAndStacksInfo{
			All:            true,
			SubCommand:     "apply",
			MaxConcurrency: 2,
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

func TestExecuteTerraformRunsDestroyInReverseDependencyOrder(t *testing.T) {
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                    true,
			SubCommand:             "destroy",
			MaxConcurrency:         1,
			AdditionalArgsAndFlags: []string{"-auto-approve"},
		},
		Stacks: terraformAdapterTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"app@dev", "database@dev", "vpc@dev"}, executed)
}

func TestExecuteTerraformDestroyFailureBlocksPrerequisites(t *testing.T) {
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:                    true,
			SubCommand:             "destroy",
			MaxConcurrency:         2,
			TerraformFailureMode:   terraformFailureModeKeepGoing,
			AdditionalArgsAndFlags: []string{"-auto-approve"},
		},
		Stacks: terraformAdapterTestStacksWithWorkdir(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			info := execution.Info
			executed = append(executed, info.Component+"@"+info.Stack)
			if info.Component == "app" {
				return TerraformExecutionResult{}, errors.New("destroy failed")
			}
			return TerraformExecutionResult{}, nil
		},
	})

	require.Error(t, err)
	require.Equal(t, []string{"app@dev"}, executed)
	require.Contains(t, err.Error(), "dependency app-dev failed")
}

func TestExecuteTerraformPassesSchedulerContextToExecutor(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("trace"), "expected")
	var sawContext bool

	err := ExecuteTerraform(ctx, TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
		},
		Stacks: terraformAdapterTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			sawContext = execution.Context.Value(contextKey("trace")) == "expected"
			return TerraformExecutionResult{}, nil
		},
	})

	require.NoError(t, err)
	require.True(t, sawContext)
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

func TestExecuteTerraformDoesNotRequireWorkdirForPlanApplyDestroy(t *testing.T) {
	for _, subcommand := range []string{"plan", "apply", "destroy"} {
		t.Run(subcommand, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{
				All:            true,
				SubCommand:     subcommand,
				MaxConcurrency: 2,
			}
			if requiresTerraformAutoApprove(info) {
				info.AdditionalArgsAndFlags = []string{"-auto-approve"}
			}

			err := ExecuteTerraform(context.Background(), TerraformOptions{
				AtmosConfig: &schema.AtmosConfiguration{},
				Info:        info,
				Stacks:      terraformAdapterTestStacks(),
				Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
					return TerraformExecutionResult{}, nil
				},
			})

			require.NoError(t, err)
		})
	}
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

func terraformAdapterTestStacksWithWorkdir() map[string]any {
	stacks := terraformAdapterTestStacks()
	terraformComponents := stacks["dev"].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.TerraformSectionName].(map[string]any)
	for componentName, component := range terraformComponents {
		componentMap := component.(map[string]any)
		componentMap["component_info"] = map[string]any{
			cfg.ComponentPathSectionName: terraformAdapterPath(componentName),
		}
		componentMap["provision"] = map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		}
	}
	return stacks
}

func terraformAdapterFailureModeStacks() map[string]any {
	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"a-fail": terraformAdapterComponent("selected", nil, nil),
					"b-blocked": terraformAdapterComponent(
						"selected",
						[]any{map[string]any{"component": "a-fail"}},
						nil,
					),
					"c-independent": terraformAdapterComponent("selected", nil, nil),
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
