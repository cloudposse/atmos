package execution

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCommandName(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		flags   []string
		want    string
	}{
		{"plain plan", "plan", nil, "plan"},
		{"plan with destroy flag only plans a destroy", "plan", []string{"-destroy"}, "plan"},
		{"destroy", "destroy", nil, "destroy"},
		{"apply with destroy flag", "apply", []string{"-destroy"}, "destroy"},
		{"apply with double dash destroy", "apply", []string{"--destroy"}, "destroy"},
		{"apply with explicit true", "apply", []string{"-destroy=true"}, "destroy"},
		{"apply with explicit false", "apply", []string{"-destroy=false"}, "apply"},
		{"last destroy flag wins", "apply", []string{"-destroy", "-destroy=false"}, "apply"},
		{"init", "init", nil, "init"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, CommandName(test.command, test.flags))
		})
	}
}

func extra(commands ...string) schema.ComponentExtraArguments {
	return schema.ComponentExtraArguments{Commands: commands}
}

func TestApplyExtraArgumentsKeepsDeclarationOrder(t *testing.T) {
	dir := t.TempDir()
	required := filepath.Join(dir, "required.tfvars")
	optional := filepath.Join(dir, "optional.tfvars")
	require.NoError(t, os.WriteFile(required, nil, 0o600))
	require.NoError(t, os.WriteFile(optional, nil, 0o600))

	first := extra("plan")
	first.Arguments = []string{"-lock-timeout=5m"}
	first.RequiredVarFiles = []string{required}
	second := extra("plan", "apply")
	second.Arguments = []string{"-compact-warnings"}
	second.OptionalVarFiles = []string{optional, filepath.Join(dir, "absent.tfvars")}
	other := extra("apply")
	other.Arguments = []string{"-never-for-plan"}

	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{first, other, second}, &ArgumentsRequest{
		Command: "plan", Args: []string{"plan", "-out", "plan.tfplan"}, InsertAt: 1, WorkingDirectory: dir,
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"plan", "-lock-timeout=5m", "-var-file=" + required, "-compact-warnings", "-var-file=" + optional,
		"-out", "plan.tfplan",
	}, got.Args)
}

func TestApplyExtraArgumentsDoesNotMutateInput(t *testing.T) {
	item := extra("plan")
	item.Arguments = []string{"-x"}
	args := []string{"plan", "-out", "p"}
	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "plan", Args: args, InsertAt: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"plan", "-out", "p"}, args)
	got.Args[0] = "changed"
	require.Equal(t, "plan", args[0])
	require.Equal(t, []string{"-x"}, item.Arguments)
}

func TestApplyExtraArgumentsIgnoresNonListedCommands(t *testing.T) {
	item := extra("apply")
	item.Arguments = []string{"-never"}
	item.Environment = map[string]string{"NEVER": "1"}
	item.RequiredVarFiles = []string{"does-not-exist.tfvars"}
	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "init", Args: []string{"init", "-input=false"}, InsertAt: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"init", "-input=false"}, got.Args)
	require.Empty(t, got.Environment)
}

func TestApplyExtraArgumentsMatchesInitIndependently(t *testing.T) {
	forInit := extra("init")
	forInit.Arguments = []string{"-backend-config=x"}
	forPlan := extra("plan")
	forPlan.Arguments = []string{"-lock=false"}
	items := []schema.ComponentExtraArguments{forInit, forPlan}

	initArgs, err := ApplyExtraArguments(items, &ArgumentsRequest{Command: "init", Args: []string{"init"}, InsertAt: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"init", "-backend-config=x"}, initArgs.Args)
	planArgs, err := ApplyExtraArguments(items, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"plan", "-lock=false"}, planArgs.Args)
}

func TestApplyExtraArgumentsRequiredVarFileMustExist(t *testing.T) {
	dir := t.TempDir()
	item := extra("plan")
	item.RequiredVarFiles = []string{"relative-missing.tfvars"}
	_, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1, WorkingDirectory: dir})
	require.ErrorIs(t, err, errUtils.ErrComponentVarFileMissing)
	require.ErrorContains(t, err, "relative-missing.tfvars")
}

func TestApplyExtraArgumentsResolvesRelativeFilesInWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "here.tfvars"), nil, 0o600))
	item := extra("plan")
	item.RequiredVarFiles = []string{"here.tfvars"}
	item.OptionalVarFiles = []string{"missing.tfvars"}
	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1, WorkingDirectory: dir})
	require.NoError(t, err)
	// Terraform runs in the same directory, so the path is passed as written.
	require.Equal(t, []string{"plan", "-var-file=here.tfvars"}, got.Args)
}

func TestApplyExtraArgumentsKeepsLastDuplicateVarFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	item := extra("plan")
	item.RequiredVarFiles = []string{"a", "b", "a"}
	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1, WorkingDirectory: dir})
	require.NoError(t, err)
	require.Equal(t, []string{"plan", "-var-file=b", "-var-file=a"}, got.Args)
}

func TestApplyExtraArgumentsEnvironmentLaterBlocksWin(t *testing.T) {
	first := extra("plan")
	first.Environment = map[string]string{"TF_LOG": "INFO", "KEEP": "1"}
	second := extra("plan")
	second.Environment = map[string]string{"TF_LOG": "DEBUG", "ADD": "2"}
	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{first, second}, &ArgumentsRequest{Command: "plan", Args: []string{"plan"}, InsertAt: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"ADD=2", "KEEP=1", "TF_LOG=DEBUG"}, got.Environment)
}

func TestApplyExtraArgumentsSavedPlanDropsVariableArguments(t *testing.T) {
	item := extra("apply")
	item.Arguments = []string{"-var=a=b", "-var-file=x.tfvars", "-parallelism=1"}
	item.RequiredVarFiles = []string{"never-checked.tfvars"}
	item.Environment = map[string]string{"STILL": "applied"}
	args := []string{"apply", "-auto-approve", "plan.tfplan"}

	withPlan, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "apply", Args: args, InsertAt: 1, PlanFileArgument: true})
	require.NoError(t, err)
	require.Equal(t, []string{"apply", "-parallelism=1", "-auto-approve", "plan.tfplan"}, withPlan.Args)
	require.Equal(t, []string{"STILL=applied"}, withPlan.Environment)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "never-checked.tfvars"), nil, 0o600))
	withoutPlan, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "apply", Args: []string{"apply"}, InsertAt: 1, WorkingDirectory: dir})
	require.NoError(t, err)
	require.Equal(t, []string{"apply", "-var=a=b", "-var-file=x.tfvars", "-parallelism=1", "-var-file=never-checked.tfvars"}, withoutPlan.Args)
}

func TestApplyExtraArgumentsInsertsAfterSubcommands(t *testing.T) {
	item := extra("state")
	item.Arguments = []string{"-lock=false"}
	got, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "state", Args: []string{"state", "list", "addr"}, InsertAt: 2})
	require.NoError(t, err)
	require.Equal(t, []string{"state", "list", "-lock=false", "addr"}, got.Args)
}

func TestApplyExtraArgumentsSavedPlanDropsVariableValuesGivenAsSeparateArguments(t *testing.T) {
	item := extra("apply")
	item.Arguments = []string{"-var", "name=value", "-parallelism=1", "-var-file", "x.tfvars", "-lock=false"}
	args := []string{"apply", "plan.tfplan"}

	result, err := ApplyExtraArguments([]schema.ComponentExtraArguments{item}, &ArgumentsRequest{Command: "apply", Args: args, InsertAt: 1, PlanFileArgument: true})

	require.NoError(t, err)
	require.Equal(t, []string{"apply", "-parallelism=1", "-lock=false", "plan.tfplan"}, result.Args)
}
