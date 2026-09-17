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
	content, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Needs    yaml.Node `yaml:"needs"`
			Strategy struct {
				Matrix struct {
					Shard   []int `yaml:"shard"`
					Include []struct {
						Tests string `yaml:"tests"`
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		}
	}
	// race's matrix is an expression, not a mapping. Decode only the jobs
	// whose matrices we inspect below.
	var document struct{ Jobs map[string]yaml.Node }
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	delete(document.Jobs, "race")
	data, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"linux", "macos", "windows"} {
		for _, family := range []string{"test", "mock", "terraform-registry-cache"} {
			job := family + "-" + platform
			var dependency string
			definition := workflow.Jobs[job]
			if err := definition.Needs.Decode(&dependency); err != nil {
				t.Fatal(err)
			}
			if dependency != "build-"+platform {
				t.Fatalf("%s depends on %q", job, dependency)
			}
		}
		var dependencies []string
		job := workflow.Jobs["test-required-"+platform]
		if err := job.Needs.Decode(&dependencies); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(dependencies, []string{"test-" + platform, "terraform-registry-cache-" + platform}) {
			t.Fatalf("%s required check waits on %v", platform, dependencies)
		}
		shards := workflow.Jobs["test-"+platform].Strategy.Matrix.Shard
		if !reflect.DeepEqual(shards, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
			t.Fatalf("%s shards: %v", platform, shards)
		}
	}
	var coverageNeeds []string
	coverage := workflow.Jobs["coverage"]
	if err := coverage.Needs.Decode(&coverageNeeds); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(coverageNeeds, []string{"build-linux", "test-linux", "magefiles"}) {
		t.Fatalf("coverage waits on unrelated jobs: %v", coverageNeeds)
	}
	if _, exists := workflow.Jobs["build-macos-intel"]; exists {
		t.Fatal("dedicated Intel build must not return")
	}
	if err := verifyWorkflow(root, 10); err != nil {
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
