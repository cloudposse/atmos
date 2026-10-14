package config

import (
	"github.com/spf13/viper"
	goyaml "go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/config/casemap"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preprocessGitHookSteps uses the step-aware decoder before generic preprocessing
// discards YAML tags. This preserves !literal fields and included script origins.
// Decoded hooks are removed from the remaining tree so functions run only once.
// Keep the tree intact instead of serializing it: settings can reference anchors in a decoded hook.
func preprocessGitHookSteps(root *goyaml.Node, v *viper.Viper, sourceFile string) error {
	if len(root.Content) == 0 {
		return nil
	}
	gitNode := mappingValueNode(root.Content[0], "git")
	if gitNode == nil || gitNode.Kind != goyaml.MappingNode {
		return nil
	}
	hooks := mappingValueNode(gitNode, "hooks")
	if hooks == nil || hooks.Kind != goyaml.MappingNode {
		return nil
	}
	return decodeGitHookSteps(hooks, v, sourceFile)
}

func decodeGitHookSteps(hooks *goyaml.Node, v *viper.Viper, sourceFile string) error {
	remaining := make([]*goyaml.Node, 0, len(hooks.Content))
	for i := 0; i+1 < len(hooks.Content); i += 2 {
		key, hook := hooks.Content[i], hooks.Content[i+1]
		if hook.Kind != goyaml.MappingNode || mappingValueNode(hook, "steps") == nil {
			remaining = append(remaining, key, hook)
			continue
		}
		decoded, err := decodeNodeWithYamlFunctionsForFile(hook, sourceFile)
		if err != nil {
			return err
		}
		v.Set("git.hooks."+key.Value, decoded)
	}
	hooks.Content = remaining
	return nil
}

// restoreGitHookStepEnv restores case lost through Viper's map decoding. Env keys use the same
// authored environment key map that custom command execution uses; vars and outputs keys use the
// step maps collected by mergeRecursiveStepCaseKeys.
func restoreGitHookStepEnv(tasks schema.Tasks, caseMaps *casemap.CaseMaps) {
	for i := range tasks {
		tasks[i].Env = caseMaps.ApplyCase(envKey, tasks[i].Env)
		tasks[i].Vars = caseMaps.ApplyCase(stepVarsCaseKey, tasks[i].Vars)
		tasks[i].Outputs = caseMaps.ApplyCase(stepOutputsCaseKey, tasks[i].Outputs)
		restoreGitHookChildStepEnv(tasks[i].Steps, caseMaps)
	}
}

func restoreGitHookChildStepEnv(steps []schema.WorkflowStep, caseMaps *casemap.CaseMaps) {
	for i := range steps {
		steps[i].Env = caseMaps.ApplyCase(envKey, steps[i].Env)
		steps[i].Vars = caseMaps.ApplyCase(stepVarsCaseKey, steps[i].Vars)
		steps[i].Outputs = caseMaps.ApplyCase(stepOutputsCaseKey, steps[i].Outputs)
		restoreGitHookChildStepEnv(steps[i].Steps, caseMaps)
	}
}
