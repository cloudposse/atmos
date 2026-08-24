package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestDescribeAffectedWithInclude tests that `describe affected` works correctly
// when stack manifests use !include and !include.raw YAML functions.
// This is the integration test for the fix of GitHub issue #2090.
func TestDescribeAffectedWithInclude(t *testing.T) {
	RequireGitRemoteWithValidURL(t)

	basePath := filepath.Join("tests", "fixtures", "scenarios", "atmos-describe-affected-with-include")
	pathPrefix := ".."

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	tempDir := t.TempDir()

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink:     func(string) cp.SymlinkAction { return cp.Skip },
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") ||
				strings.Contains(src, ".claude") ||
				strings.Contains(src, ".terraform") {
				return true, nil
			}
			isSocket, err := u.IsSocket(src)
			if err != nil {
				return true, err
			}
			if isSocket {
				return true, nil
			}
			return false, nil
		},
	}

	prepareDescribeAffectedWithIncludeBaseRepo(t, pathPrefix, tempDir, basePath, stacksPath, &copyOptions)

	// Set BasePath for the fixture.
	atmosConfig.BasePath = basePath

	t.Run("describe affected resolves includes in both HEAD and BASE", func(t *testing.T) {
		affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
			&atmosConfig,
			tempDir,
			false,
			false,
			"",
			false,
			true, // processYamlFunctions: true - this triggers !include processing.
			nil,
			false,
			nil,   // authManager
			false, // authDisabled
		)

		require.NoError(t, err, "describe affected should not fail when stacks use !include")

		// All three components should be affected (environment var changed in all).
		require.GreaterOrEqual(t, len(affected), 3,
			"should detect at least 3 affected components (app-with-includes, app-with-raw-includes, simple-app)")

		// Verify the specific components are in the affected list.
		componentNames := make(map[string]bool)
		for _, a := range affected {
			componentNames[a.Component] = true
		}

		assert.True(t, componentNames["app-with-includes"],
			"app-with-includes should be affected")
		assert.True(t, componentNames["app-with-raw-includes"],
			"app-with-raw-includes should be affected")
		assert.True(t, componentNames["simple-app"],
			"simple-app should be affected")
	})

	t.Run("describe affected with includes produces valid stack data", func(t *testing.T) {
		affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
			&atmosConfig,
			tempDir,
			false,
			false,
			"",
			false,
			true,
			nil,
			false,
			nil,   // authManager
			false, // authDisabled
		)

		require.NoError(t, err)

		// Verify affected components report the right affected reason.
		for _, a := range affected {
			assert.Equal(t, "terraform", a.ComponentType)
			assert.Equal(t, "nonprod", a.Stack)
			// All changes are in stack.vars since we changed the environment var.
			assert.Contains(t, a.AffectedAll, "stack.vars",
				"component %s should be affected due to stack.vars change", a.Component)
		}
	})
}

// TestDescribeAffectedWithIncludeSelfComparison tests that comparing a repo with
// itself produces no affected components, even when stacks use !include.
func TestDescribeAffectedWithIncludeSelfComparison(t *testing.T) {
	RequireGitRemoteWithValidURL(t)

	basePath := filepath.Join("tests", "fixtures", "scenarios", "atmos-describe-affected-with-include")
	pathPrefix := ".."

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	atmosConfig.BasePath = filepath.Join(".", basePath)

	// Compare with itself — should produce empty affected list.
	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		pathPrefix,
		false,
		false,
		"",
		false,
		true, // processYamlFunctions: true.
		nil,
		false,
		nil,   // authManager
		false, // authDisabled
	)

	require.NoError(t, err,
		"describe affected should not fail when stacks use !include (self-comparison)")
	assert.Equal(t, 0, len(affected),
		"self-comparison should produce empty affected list")
}

func prepareDescribeAffectedWithIncludeBaseRepo(
	t *testing.T,
	sourceRoot string,
	tempDir string,
	basePath string,
	stacksPath string,
	copyOptions *cp.Options,
) {
	t.Helper()

	copyFixturePath(
		t,
		filepath.Join(sourceRoot, "tests", "fixtures", "components"),
		filepath.Join(tempDir, "tests", "fixtures", "components"),
		copyOptions,
	)
	copyFixturePath(
		t,
		filepath.Join(stacksPath, "atmos.yaml"),
		filepath.Join(tempDir, basePath, "atmos.yaml"),
		copyOptions,
	)
	copyFixturePath(
		t,
		filepath.Join(stacksPath, "config"),
		filepath.Join(tempDir, basePath, "config"),
		copyOptions,
	)

	// Copy the affected stacks over the normal stacks path in the temp repo. This
	// simulates a base ref with different vars without copying the full checkout.
	copyFixturePath(
		t,
		filepath.Join(stacksPath, "stacks-affected"),
		filepath.Join(tempDir, basePath, "stacks"),
		copyOptions,
	)

	repo, err := git.PlainInit(tempDir, false)
	require.NoError(t, err)

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/cloudposse/atmos.git"},
	})
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	_, err = worktree.Commit("base fixture", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Atmos Tests",
			Email: "test@example.com",
			When:  time.Unix(0, 0),
		},
	})
	require.NoError(t, err)
}

func copyFixturePath(t *testing.T, src string, dest string, copyOptions *cp.Options) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	require.NoError(t, cp.Copy(src, dest, *copyOptions))
}

// TestDescribeAffectedWithIncludeComponentsLoadCorrectly verifies that all
// components using !include directives can be loaded via ExecuteDescribeComponent.
func TestDescribeAffectedWithIncludeComponentsLoadCorrectly(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "atmos-describe-affected-with-include"))

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	components := []string{
		"app-with-includes",
		"app-with-raw-includes",
		"simple-app",
	}

	for _, componentName := range components {
		t.Run(componentName, func(t *testing.T) {
			componentSection, err := e.ExecuteDescribeComponent(
				&e.ExecuteDescribeComponentParams{
					Component: componentName,
					Stack:     "nonprod",
				},
			)

			require.NoError(t, err, "component %s should load without errors", componentName)
			require.NotNil(t, componentSection)

			vars, ok := componentSection["vars"].(map[string]interface{})
			require.True(t, ok, "vars should be a map for %s", componentName)
			assert.Equal(t, "nonprod", vars["environment"],
				"environment should be 'nonprod' for %s", componentName)
		})
	}
}

// TestDescribeAffectedWithIncludeVerifyIncludedValues verifies that !include
// directives resolve correctly and produce the expected values.
func TestDescribeAffectedWithIncludeVerifyIncludedValues(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "atmos-describe-affected-with-include"))

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	t.Run("app-with-includes has correct included values", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "app-with-includes",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		vars := componentSection["vars"].(map[string]interface{})

		// JSON config should be parsed as a map.
		jsonConfig, ok := vars["json_config"].(map[string]interface{})
		require.True(t, ok, "json_config should be a map")
		assert.Equal(t, "my-app", jsonConfig["app_name"])
		assert.Equal(t, true, jsonConfig["enabled"])

		// YQ expression should extract the app_name.
		assert.Equal(t, "my-app", vars["app_name"])

		// .rego file should be a raw string.
		policyBody, ok := vars["policy_body"].(string)
		require.True(t, ok, "policy_body should be a string")
		assert.Contains(t, policyBody, "package spacelift")
		assert.Contains(t, policyBody, "default allow = false")

		// YAML settings should be parsed as a map.
		settings, ok := vars["settings"].(map[string]interface{})
		require.True(t, ok, "settings should be a map")
		assert.Equal(t, "info", settings["log_level"])
		assert.Equal(t, 3, settings["retry_count"])
	})

	t.Run("app-with-raw-includes has correct raw values", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "app-with-raw-includes",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		vars := componentSection["vars"].(map[string]interface{})

		// !include.raw should return raw strings, not parsed maps.
		jsonRaw, ok := vars["json_raw"].(string)
		require.True(t, ok, "json_raw should be a string (raw), got %T", vars["json_raw"])
		assert.Contains(t, jsonRaw, "\"app_name\"")

		policyRaw, ok := vars["policy_raw"].(string)
		require.True(t, ok, "policy_raw should be a string")
		assert.Contains(t, policyRaw, "package spacelift")
	})
}
