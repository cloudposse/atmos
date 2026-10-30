package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func TestMain(m *testing.M) {
	if runToolchainChild() {
		os.Exit(0)
	}
	if os.Getenv("ATMOS_TERRAGRUNT_VERSION_CHILD") != "" {
		_, _ = os.Stdout.WriteString(os.Getenv("ATMOS_TERRAGRUNT_VERSION_CHILD"))
		os.Exit(0)
	}
	if os.Getenv("ATMOS_TERRAGRUNT_ENV_CHILD") == "1" {
		cwd, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"cwd": cwd, "profile": os.Getenv("AWS_PROFILE"), "token": os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"), "custom": os.Getenv("CUSTOM_VALUE"), "path": os.Getenv("PATH")})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestEvaluatorUsesScopedEnvironmentForFunctionsAndCommands(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	module := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.MkdirAll(module, 0o755))
	executable, err := os.Executable()
	require.NoError(t, err)
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	name := "fixture-environment-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(bin, name), content, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, name), content, 0o755))
	sourceJSON, err := json.Marshal(filepath.ToSlash(module))
	require.NoError(t, err)
	config := `terraform { source = ` + string(sourceJSON) + ` }
inputs = {
  direct = get_env("CUSTOM_VALUE")
  child = jsondecode(run_cmd("` + name + `"))
 relative = jsondecode(run_cmd("./` + name + `"))
 directory = get_terragrunt_dir()
}
`
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o644))
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	t.Setenv("CUSTOM_VALUE", "ambient")
	physical, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	type outcome struct {
		profile string
		result  *model.Resolution
		err     error
	}
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for _, profile := range []string{"development", "production"} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			env := map[string]string{"PATH": bin, "AWS_PROFILE": profile, "CUSTOM_VALUE": profile, "ATMOS_TERRAGRUNT_ENV_CHILD": "1"}
			result, err := Resolve(t.Context(), model.Request{Path: path, BasePath: root, Command: "plan", Tofu: executable, Environment: env})
			outcomes <- outcome{profile: profile, result: result, err: err}
		}()
	}
	workers.Wait()
	close(outcomes)
	for outcome := range outcomes {
		require.NoError(t, outcome.err)
		result, profile := outcome.result, outcome.profile
		require.Equal(t, filepath.ToSlash(physical), result.Inputs["directory"])
		require.Equal(t, profile, result.Inputs["direct"])
		child := result.Inputs["child"].(map[string]any)
		require.Equal(t, filepath.Clean(physical), filepath.Clean(child["cwd"].(string)))
		require.Equal(t, child, result.Inputs["relative"])
		require.Equal(t, profile, child["profile"])
		require.Equal(t, profile, child["custom"])
		require.Equal(t, "", child["token"])
		require.Equal(t, bin, child["path"])
		require.Equal(t, "ambient", os.Getenv("CUSTOM_VALUE"))
		require.Equal(t, "ambient-token-must-not-return", os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"))
	}
}
