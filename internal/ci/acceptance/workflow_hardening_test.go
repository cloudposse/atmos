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
	t.Parallel()
	root := filepath.Join("..", "..", "..", ".github")
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".yml" {
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
		checked += checkEndpointNodes(t, path, &document)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no endpoint lists checked")
	}
}

func checkEndpointNodes(t *testing.T, path string, node *yaml.Node) int {
	t.Helper()
	checked := 0
	endpoint := regexp.MustCompile(`^[a-zA-Z0-9.*-]+:[0-9]+$`)
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
		checked += checkEndpointNodes(t, path, child)
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
