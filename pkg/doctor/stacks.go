package doctor

import (
	"context"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

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
	if !e.canCheckStacks(p, o) {
		r.Checks = append(r.Checks, Check{ID: ruleStacks, Status: "incomplete", Files: count, Reason: "Effective stack validation needs complete static inputs and a stack validator; imports, profiles, excluded sources, and executable/template inputs remain unverified"})
		return nil
	}
	config, err := staticConfig(p)
	if err != nil {
		return err
	}
	inputs, err := staticInputs(p, config)
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
	r.Checks = append(r.Checks, Check{ID: ruleStacks, Status: status, Files: len(inputs)})
	e.emit(Event{Phase: "Check", Label: "Static stacks", Done: true, Status: status})
	return nil
}

func staticConfig(p *project) (*schema.AtmosConfiguration, error) {
	config := schema.AtmosConfiguration{}
	content, err := yaml.Marshal(p.config)
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(content, &config); err != nil {
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

func staticInputs(p *project, config *schema.AtmosConfiguration) (map[string]map[string]any, error) {
	inputs := map[string]map[string]any{}
	for _, d := range p.docs {
		if d.kind != "stack" || d.node == nil {
			continue
		}
		path := relative(p.stackDir, d.path)
		included, err := stackPathIncluded(config, path)
		if err != nil {
			return nil, err
		}
		if !included {
			continue
		}
		var values map[string]any
		if err := d.node.Decode(&values); err != nil {
			return nil, err
		}
		inputs[path] = values
	}
	return inputs, nil
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

func (e *Engine) canCheckStacks(p *project, o *Options) bool {
	return e.Stacks != nil && p.complete && p.configCount == 1 && len(o.Profiles) == 0
}
