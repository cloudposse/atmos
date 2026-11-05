package execbatch_test

import (
	"context"
	"os"
	osExec "os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var _ = schema.ExecFunctionSettings{MaxConcurrency: nil}

// TestParallelExecDocument verifies decoding, result placement, and skipping in both execution modes.
func TestParallelExecDocument(t *testing.T) {
	for _, limit := range []int{1, 3} {
		config := &schema.AtmosConfiguration{}
		config.Settings.Functions.Exec.MaxConcurrency = &limit
		input := map[string]any{"vars": map[string]any{
			"one": execSource("printf 'first\\n'"), "nested": []any{execSource("printf '[1,2]'"), execSource("printf '!env MUST_NOT_BE_EVALUATED'")},
		}}
		got, err := exec.ProcessCustomYamlTags(config, input, "test", nil, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"vars": map[string]any{"one": "first", "nested": []any{[]any{float64(1), float64(2)}, "!env MUST_NOT_BE_EVALUATED"}}}, got)
		skipped, err := exec.ProcessCustomYamlTags(config, input, "test", []string{"exec"}, nil)
		require.NoError(t, err)
		assert.Equal(t, input, skipped)
	}
}

// TestParallelExecWithStarlarkReferences verifies that dependent expressions receive evaluated exec values.
func TestParallelExecWithStarlarkReferences(t *testing.T) {
	limit := 2
	config := &schema.AtmosConfiguration{}
	config.Settings.Functions.Exec.MaxConcurrency = &limit
	input := map[string]any{"vars": map[string]any{
		"count":    execSource("printf '3'"),
		"computed": (starlarksource.Source{Code: `return ctx.vars["count"] * 2`, File: "test.yaml", Line: 1}).Encode(),
	}}
	info := &schema.ConfigAndStacksInfo{ComponentSection: input}
	got, err := exec.ProcessCustomYamlTags(config, input, "dev", nil, info)
	require.NoError(t, err)
	assert.Equal(t, float64(3), got["vars"].(map[string]any)["count"])
	assert.EqualValues(t, 6, got["vars"].(map[string]any)["computed"])
}

// TestParallelExecInheritedStdin checks both production execution paths in isolated child processes.
func TestParallelExecInheritedStdin(t *testing.T) {
	t.Parallel()
	const helperEnv = "ATMOS_TEST_EXEC_STDIN_LIMIT"
	if value := os.Getenv(helperEnv); value != "" {
		limit, err := strconv.Atoi(value)
		require.NoError(t, err)
		config := &schema.AtmosConfiguration{}
		config.Settings.Functions.Exec.MaxConcurrency = &limit
		input := map[string]any{"vars": map[string]any{
			"input": execSource(`read -r value; printf '%s' "$value"`),
			"other": execSource("printf 'unrelated'"),
		}}
		got, err := exec.ProcessCustomYamlTags(config, input, "test", nil, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"vars": map[string]any{
			"input": "inherited input", "other": "unrelated",
		}}, got)
		return
	}

	executable, err := os.Executable()
	require.NoError(t, err)
	for _, limit := range []int{1, 3} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			command := osExec.CommandContext(ctx, executable, "-test.run=^TestParallelExecInheritedStdin$")
			command.Env = append(os.Environ(), helperEnv+"="+strconv.Itoa(limit))
			command.Stdin = strings.NewReader("inherited input\n")
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}

// execSource represents a deferred tag produced by the YAML loader.
func execSource(command string) string {
	return (yamlsource.Source{Tag: "!exec", Argument: command}).Encode()
}

// TestParallelExecEdition verifies that concurrency preserves tag identity and legacy behavior.
func TestParallelExecEdition(t *testing.T) {
	for _, pin := range []string{"", "2026-10-19", "2026-10-20", "2026-10-21"} {
		for _, limit := range []int{1, 3} {
			t.Run(pin+"/"+strconv.Itoa(limit), func(t *testing.T) {
				config := &schema.AtmosConfiguration{Edition: pin}
				config.Settings.Functions.Exec.MaxConcurrency = &limit
				input, err := u.UnmarshalYAMLFromFile[map[string]any](config, `
tagged: &tagged !exec "printf value"
alias: *tagged
single: '!exec printf value'
double: "!exec printf value"
typed: !!str "!exec printf value"
block: |-
  !exec printf value
list: [!exec "printf value", "!exec printf value"]
`, "parallel-exec.yaml")
				require.NoError(t, err)
				got, err := exec.ProcessCustomYamlTags(config, input, "test", nil, nil)
				require.NoError(t, err)
				assert.Equal(t, "value", got["tagged"])
				assert.Equal(t, "value", got["alias"])
				literal := "!exec printf value"
				if pin == "2026-10-19" {
					literal = "value"
				}
				for _, key := range []string{"single", "double", "typed", "block"} {
					assert.Equal(t, literal, got[key], key)
				}
				assert.Equal(t, []any{"value", literal}, got["list"])
			})
		}
	}
}
