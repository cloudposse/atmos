package doctor

import (
	"bytes"
	"context"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

func (e *Engine) checkStacks(ctx context.Context, p *project, o *Options, r *Report) error {
	if !enabled(o, ruleStacks, categoryValidation) {
		return nil
	}
	count := stackCount(p)
	if count == 0 {
		r.Checks = append(r.Checks, Check{ID: ruleStacks, Status: "skipped", Reason: "No stack manifests"})
		return nil
	}
	if !e.canCheckStacks(p) {
		r.Checks = append(r.Checks, Check{ID: ruleStacks, Status: "incomplete", Files: count, Reason: "Effective stack validation is incomplete because " + e.stackBlocker(p) + "; templates, YAML functions, profiles, and remote or templated imports remain unverified"})
		return nil
	}
	config, err := staticConfig(p)
	if err != nil {
		// The runtime decodes leniently; a configuration neither decoder accepts is a coverage gap, not a crash.
		r.Checks = append(r.Checks, Check{ID: ruleStacks, Status: "incomplete", Files: count, Reason: "Configuration could not be decoded for stack validation: " + err.Error()})
		return nil //nolint:nilerr // Undecodable configuration is recorded as an incomplete check, not an operational failure.
	}
	inputs, err := e.staticInputs(p, config)
	if err != nil {
		return err
	}
	e.emit(Event{Phase: "Check", Label: "Static stacks"})
	if err := e.Stacks.Validate(ctx, config, inputs); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.Findings = append(r.Findings, Finding{RuleID: ruleStacks, Category: categoryValidation, Severity: validation.SeverityError, Message: err.Error(), Status: statusManual, Remediation: "Review stack inheritance and duplicate instance declarations"})
	}
	status := checkStatus(r, ruleStacks)
	r.Checks = append(r.Checks, Check{ID: ruleStacks, Status: status, Files: len(inputs.Files)})
	e.emit(Event{Phase: "Check", Label: "Static stacks", Done: true, Status: status})
	return nil
}

func staticConfig(p *project) (*schema.AtmosConfiguration, error) {
	config := schema.AtmosConfiguration{}
	if err := decodeConfig(p.config, &config); err != nil {
		return nil, err
	}
	config.StacksBaseAbsolutePath = p.stackDir
	config.BasePath = p.base
	config.TerraformDirAbsolutePath = absolute(p.base, config.Components.Terraform.BasePath)
	config.HelmfileDirAbsolutePath = absolute(p.base, config.Components.Helmfile.BasePath)
	config.PackerDirAbsolutePath = absolute(p.base, config.Components.Packer.BasePath)
	config.AnsibleDirAbsolutePath = absolute(p.base, config.Components.Ansible.BasePath)
	return &config, nil
}

// staticInputs lists the top-level stack manifests below the stacks base path that the stack
// consumers load, honoring stacks.included_paths and stacks.excluded_paths. The reader serves
// the documents doctor captured, so a repair candidate is validated instead of the disk.
func (e *Engine) staticInputs(p *project, config *schema.AtmosConfiguration) (StackInputs, error) {
	inputs := StackInputs{Reader: projectReader{project: p, fs: e.FS}}
	for _, d := range p.docs {
		if d.kind != documentStack || d.node == nil || !within(p.stackDir, d.path) {
			continue
		}
		path := relative(p.stackDir, d.path)
		included, err := stackPathIncluded(config, path)
		if err != nil {
			return StackInputs{}, err
		}
		if included {
			inputs.Files = append(inputs.Files, path)
		}
	}
	return inputs, nil
}

// projectReader serves the content doctor captured for a document and reads anything else, such as
// an imported file excluded from the run, through the engine's file system.
type projectReader struct {
	project *project
	fs      filesystem.FileSystem
}

func (r projectReader) ReadFile(path string) ([]byte, error) {
	defer perf.Track(nil, "doctor.projectReader.ReadFile")()

	if d, ok := r.project.byPath[filepath.Clean(path)]; ok {
		return bytes.Clone(d.content), nil
	}
	return r.fs.ReadFile(path)
}

func stackPathIncluded(config *schema.AtmosConfiguration, path string) (bool, error) {
	included := len(config.Stacks.IncludedPaths) == 0
	for _, pattern := range config.Stacks.IncludedPaths {
		match, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, err
		}
		included = included || match
	}
	for _, pattern := range config.Stacks.ExcludedPaths {
		match, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, err
		}
		if match {
			return false, nil
		}
	}
	return included, nil
}

func stackCount(p *project) int {
	count := 0
	for _, d := range p.docs {
		if d.kind == "stack" {
			count++
		}
	}
	return count
}

func (e *Engine) canCheckStacks(p *project) bool {
	return e.Stacks != nil && p.complete && p.configCount == 1 && len(p.profiles) == 0
}

// decodeConfig decodes merged configuration the way the runtime does. Strict YAML decoding is
// tried first; values the runtime accepts through weak typing, such as the string "false" for a
// boolean, fall back to the same lenient decoder Viper applies when Atmos loads configuration.
func decodeConfig(values map[string]any, config *schema.AtmosConfiguration) error {
	content, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	if err = yaml.Unmarshal(content, config); err == nil {
		return nil
	}
	*config = schema.AtmosConfiguration{}
	v := viper.New()
	if err = v.MergeConfigMap(values); err != nil {
		return err
	}
	return v.Unmarshal(config)
}
