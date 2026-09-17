package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type workflowRoute struct {
	Name     string `yaml:"name"`
	Strategy struct {
		Matrix struct {
			Shard  []int `yaml:"shard"`
			Flavor []struct {
				Target string `yaml:"target"`
			} `yaml:"flavor"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []struct {
		Run string `yaml:"run"`
	} `yaml:"steps"`
}

type workflowCheck struct {
	Name string   `json:"name"`
	Jobs []string `json:"jobs"`
}

type workflowPolicy struct {
	Parent   string          `json:"parent"`
	Required []workflowCheck `json:"required"`
}

// Validate the actual platform files and the trusted reporter's required checks
// together. YAML parsing handles multiline matrices and aliases without regexes.
func verifyWorkflow(repoRoot string, shardCount int) error {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "actions", "ci-pipeline", "policy.json"))
	if err != nil {
		return fmt.Errorf("read CI check policy: %w", err)
	}
	var policy map[string]workflowPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("parse CI check policy: %w", err)
	}
	for _, platform := range []string{"linux", "macos", "windows"} {
		file := "ci-test-" + platform + ".yml"
		if err := verifyPlatformWorkflow(repoRoot, platform, shardCount, policy[file]); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	return nil
}

func verifyPlatformWorkflow(repoRoot, platform string, shardCount int, policy workflowPolicy) error {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci-test-"+platform+".yml"))
	if err != nil {
		return fmt.Errorf("read platform workflow: %w", err)
	}
	var workflow struct {
		Env  map[string]string        `yaml:"env"`
		Jobs map[string]workflowRoute `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return fmt.Errorf("parse platform workflow: %w", err)
	}
	if err := verifyPlatformTargets(workflow.Jobs, platform); err != nil {
		return err
	}
	route := workflow.Jobs["test-"+platform]
	if workflow.Env["TEST_SHARD_COUNT"] != strconv.Itoa(shardCount) || len(route.Strategy.Matrix.Shard) != shardCount {
		return fmt.Errorf("%w: workflow shard count differs from %d", errShardPlan, shardCount)
	}
	wantName := fmt.Sprintf("Acceptance Tests (${{ matrix.flavor.target }}, shard ${{ matrix.shard }}/%d)", shardCount)
	if route.Name != wantName {
		return fmt.Errorf("%w: unexpected shard job name %q", errShardPlan, route.Name)
	}
	expected := make([]string, 0, shardCount+1)
	for index, shard := range route.Strategy.Matrix.Shard {
		if shard != index+1 {
			return fmt.Errorf("%w: shard position %d contains %d", errShardPlan, index+1, shard)
		}
		expected = append(expected, fmt.Sprintf("Acceptance Tests (%s, shard %d/%d)", platform, shard, shardCount))
	}
	expected = append(expected, "Terraform registry cache test ("+platform+")")
	return verifyReporterPolicy(platform, expected, policy)
}

func verifyReporterPolicy(platform string, expected []string, policy workflowPolicy) error {
	if policy.Parent != "ci-build-"+platform+".yml" {
		return fmt.Errorf("%w: reporter uses the wrong platform build", errShardPlan)
	}
	matched := 0
	for _, check := range policy.Required {
		if check.Name != "Acceptance Tests ("+platform+")" {
			continue
		}
		matched++
		if !slices.Equal(check.Jobs, expected) {
			return fmt.Errorf("%w: reporter does not require every shard and registry test", errShardPlan)
		}
	}
	if matched != 1 {
		return fmt.Errorf("%w: expected exactly one required acceptance check", errShardPlan)
	}
	return nil
}

func verifyPlatformTargets(jobs map[string]workflowRoute, platform string) error {
	for _, job := range []string{"test-" + platform, "terraform-registry-cache-" + platform} {
		flavors := jobs[job].Strategy.Matrix.Flavor
		if len(flavors) != 1 || flavors[0].Target != platform {
			return fmt.Errorf("%w: %s must target %s exactly once", errShardPlan, job, platform)
		}
	}
	registry := jobs["terraform-registry-cache-"+platform]
	if registry.Name != "Terraform registry cache test (${{ matrix.flavor.target }})" {
		return fmt.Errorf("%w: registry check name changed", errShardPlan)
	}
	if !slices.ContainsFunc(registry.Steps, func(step struct {
		Run string `yaml:"run"`
	},
	) bool {
		return strings.Contains(step.Run, "go test ./tests -run '^"+RegistryTest+"$'")
	}) {
		return fmt.Errorf("%w: %s has no dedicated workflow route", errShardPlan, RegistryTest)
	}
	return nil
}
