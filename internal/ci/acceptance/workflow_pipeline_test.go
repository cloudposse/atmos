package acceptance

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Inspect the real workflow after resolving aliases: regressions in needs can
// silently restore the all-platform barrier while every individual test passes.
func TestWorkflowPlatformDependencies(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..")
	if err := verifyWorkflow(root, 10); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"linux", "macos", "windows"} {
		data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci-test-"+platform+".yml"))
		if err != nil {
			t.Fatal(err)
		}
		var workflow struct {
			On struct {
				WorkflowRun struct {
					Workflows []string `yaml:"workflows"`
				} `yaml:"workflow_run"`
			} `yaml:"on"`
			Jobs map[string]struct {
				Needs []string `yaml:"needs"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(data, &workflow); err != nil {
			t.Fatal(err)
		}
		names := map[string]string{"linux": "CI Build Linux", "macos": "CI Build macOS", "windows": "CI Build Windows"}
		if !reflect.DeepEqual(workflow.On.WorkflowRun.Workflows, []string{names[platform]}) {
			t.Fatal("cross-platform build barrier")
		}
		for _, family := range []string{"test", "mock", "terraform-registry-cache"} {
			if !reflect.DeepEqual(workflow.Jobs[family+"-"+platform].Needs, []string{"source"}) {
				t.Fatal("missing source dependency")
			}
		}
	}
	content, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci-test-floci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Include []struct {
						Tests string `yaml:"tests"`
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}

	// Every test selected by the old Floci job must appear exactly once in the
	// isolated groups, including future test names matching the old selector.
	old := regexp.MustCompile(`^Test((AWS(StoreHooks|Secrets)|GCPSecrets|AzureSecrets)FlociE2E|LocalGitOpsPushE2E|Scaffold(AWSLandingZone|GCPLandingZone|AzureLandingZone|AWSApp)FlociE2E|InitFromTemplateRepoGiteaE2E|TerraformFlociTfmigrateS3History)$`)
	var groups []*regexp.Regexp
	for _, group := range workflow.Jobs["floci-go"].Strategy.Matrix.Include {
		groups = append(groups, regexp.MustCompile(group.Tests))
	}
	files, err := filepath.Glob(filepath.Join(root, "tests", "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	functions := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	selected := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range functions.FindAllSubmatch(data, -1) {
			name := string(match[1])
			count := 0
			for _, group := range groups {
				if group.MatchString(name) {
					count++
				}
			}
			want := 0
			if old.MatchString(name) {
				want = 1
				selected++
			}
			if count != want {
				t.Errorf("Floci %s assigned %d times, want %d", name, count, want)
			}
		}
	}
	if selected == 0 {
		t.Fatal("no Floci tests found")
	}
}

// Cache permissions must remain read-only when workflow_run executes PR tests.
func TestWorkflowCachePolicies(t *testing.T) {
	t.Parallel()
	policies := map[string]string{
		"ci-build-linux": "write", "ci-build-macos": "write", "ci-build-windows": "write",
		"ci-lint": "write", "setup-go-cache-warmup": "write",
		"ci-test-linux": "read", "ci-test-macos": "read", "ci-test-windows": "read",
		"ci-test-floci": "read", "ci-test-k3s-linux": "read", "ci-test-k3s-macos": "read",
		"ci-test-integrations": "read", "ci-test-go": "read", "ci-test-kubernetes": "read",
		"ci-report": "none", "ci-coverage": "none", "ci-timing-summary": "none",
	}
	for name, want := range policies {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", name+".yml"))
			if err != nil {
				t.Fatal(err)
			}
			var workflow struct {
				Mode string `yaml:"cache-mode"`
				Jobs map[string]struct {
					Mode string `yaml:"cache-mode"`
				} `yaml:"jobs"`
			}
			if err := yaml.Unmarshal(data, &workflow); err != nil {
				t.Fatal(err)
			}
			if workflow.Mode != want {
				t.Errorf("cache-mode = %q, want %q", workflow.Mode, want)
			}
			for job, config := range workflow.Jobs {
				if want != "write" && config.Mode != "" && config.Mode != want && config.Mode != "none" {
					t.Errorf("%s broadens cache access to %q", job, config.Mode)
				}
			}
			if name == "setup-go-cache-warmup" && workflow.Jobs["prune"].Mode != "none" {
				t.Error("prune must disable cache service access")
			}
		})
	}
}
