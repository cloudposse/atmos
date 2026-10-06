package workflow

import (
	"path/filepath"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/yaml/includescope"
)

// LoadManifest parses the content of the workflow manifest at file.
//
// Local !include and !include.raw paths resolve the same way wherever Atmos runs from: "./x"
// and "../x" against the manifest's directory, bare "x/y" against the Atmos project base path,
// and absolute paths as written. A step whose script comes from such a file records that file in
// WorkflowStep.ScriptSource, so the script's own load() calls resolve relative to it.
func LoadManifest(atmosConfig *schema.AtmosConfiguration, file string, content []byte) (schema.WorkflowManifest, error) {
	defer perf.Track(atmosConfig, "workflow.LoadManifest")()

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return schema.WorkflowManifest{}, err
	}

	absFile, err := filepath.Abs(file)
	if err != nil {
		return schema.WorkflowManifest{}, err
	}
	scope := includescope.Scope{File: absFile, BasePath: manifestBasePath(atmosConfig)}
	scope.Rewrite(&root)
	if err := scope.ExpandLocalYAML(&root); err != nil {
		return schema.WorkflowManifest{}, err
	}

	// Let the YAML decoder follow aliases and merge keys while retaining each script's
	// include node. A literal tree walk misses inherited scripts and included step lists.
	var sources struct {
		Workflows map[string]struct {
			Steps stepSources `yaml:"steps"`
		} `yaml:"workflows"`
	}
	if err := root.Decode(&sources); err != nil {
		return schema.WorkflowManifest{}, err
	}
	manifest, err := utils.UnmarshalYAMLFromNode[schema.WorkflowManifest](atmosConfig, &root, file)
	if err != nil {
		return schema.WorkflowManifest{}, err
	}
	for name := range manifest.Workflows {
		// Steps share their backing array with the map value, so setting a field in place sticks.
		applyScriptSources(manifest.Workflows[name].Steps, sources.Workflows[name].Steps)
	}
	return manifest, nil
}

// manifestBasePath returns the absolute Atmos project base path used for bare include paths.
func manifestBasePath(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig == nil {
		return ""
	}
	if atmosConfig.BasePathAbsolute != "" {
		return atmosConfig.BasePathAbsolute
	}
	if abs, err := filepath.Abs(atmosConfig.BasePath); err == nil {
		return abs
	}
	return atmosConfig.BasePath
}

// stepSource records where a step's script came from, mirroring the shape of the step tree.
type stepSource struct {
	script   string
	children []stepSource
}

type stepSources []stepSource

// UnmarshalYAML defers unresolved remote or queried step-list includes to the generic loader.
func (s *stepSources) UnmarshalYAML(node *yaml.Node) error {
	defer perf.Track(nil, "workflow.stepSources.UnmarshalYAML")()

	if node.Kind != yaml.SequenceNode {
		return nil
	}
	type plain stepSources
	return node.Decode((*plain)(s))
}

// UnmarshalYAML ignores shorthand command steps, which carry no script provenance.
func (s *stepSource) UnmarshalYAML(node *yaml.Node) error {
	defer perf.Track(nil, "workflow.stepSource.UnmarshalYAML")()

	if node.Kind != yaml.MappingNode {
		return nil
	}
	var fields struct {
		Script yaml.Node   `yaml:"script"`
		Steps  stepSources `yaml:"steps"`
	}
	if err := node.Decode(&fields); err != nil {
		return err
	}
	s.script, _ = includescope.LocalFile(&fields.Script)
	s.children = fields.Steps
	return nil
}

// applyScriptSources copies recorded script sources onto the decoded steps by position.
func applyScriptSources(steps []schema.WorkflowStep, sources []stepSource) {
	for i := range steps {
		if i >= len(sources) {
			return
		}
		steps[i].ScriptSource = sources[i].script
		applyScriptSources(steps[i].Steps, sources[i].children)
	}
}
