package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Harden Runner splits on spaces, not arbitrary whitespace. A literal YAML
// block silently turns the entire allowlist into a malformed first endpoint.
func TestWorkflowEndpointLists(t *testing.T) {
	// Keep the YAML scan apart from parallel subprocess tests on Windows.
	endpoint := regexp.MustCompile(`^[a-zA-Z0-9.*-]+:[0-9]+$`)
	root := filepath.Join("..", "..", "..", ".github")
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		ext := filepath.Ext(path)
		if entry.IsDir() || (ext != ".yml" && ext != ".yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			return err
		}
		checked += checkEndpointNodes(t, path, &document, endpoint)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no endpoint lists checked")
	}
}

func checkEndpointNodes(t *testing.T, path string, node *yaml.Node, endpoint *regexp.Regexp) int {
	t.Helper()
	checked := 0
	for index, child := range node.Content {
		if node.Kind == yaml.MappingNode && index%2 == 0 && child.Value == "allowed-endpoints" {
			value := node.Content[index+1].Value
			for _, item := range strings.Split(strings.TrimSpace(value), " ") {
				if item != "" && !endpoint.MatchString(item) {
					t.Errorf("%s:%d: endpoint is not space-delimited: %q", path, child.Line, item)
				}
			}
			checked++
		}
		checked += checkEndpointNodes(t, path, child, endpoint)
	}
	return checked
}

func TestBuildScriptsUseEnvironmentForInputs(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "actions", "ci-build", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Runs struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(data, &action); err != nil {
		t.Fatal(err)
	}
	for _, step := range action.Runs.Steps {
		if strings.Contains(step.Run, "${{") {
			t.Errorf("%s interpolates workflow data into shell source", step.Name)
		}
	}
}

// Same-run consumers also supply tokens. Explicitly forwarding an empty run-id
// overrides download-artifact's default and makes it request /runs/NaN/artifacts.
func TestArtifactRetryRunIDFallback(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", ".github", "actions", "download-artifact-retry", "action.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Runs struct {
			Steps []struct {
				Uses string            `yaml:"uses"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(data, &action); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	for _, step := range action.Runs.Steps {
		if !strings.HasPrefix(step.Uses, "actions/download-artifact@") {
			continue
		}
		attempts++
		if step.With["run-id"] != "${{ inputs.run-id || github.run_id }}" {
			t.Errorf("attempt %d must prefer the producer ID and fall back to the current run", attempts)
		}
		if step.With["github-token"] != "${{ inputs.github-token }}" {
			t.Errorf("attempt %d must preserve the caller's download credentials", attempts)
		}
	}
	if attempts != 3 {
		t.Errorf("checked %d attempts, want 3", attempts)
	}
}
