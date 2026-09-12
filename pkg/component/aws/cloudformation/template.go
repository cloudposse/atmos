package cloudformation

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolveComponentPath returns the on-disk directory for this component instance
// (template.yaml, stack policy, and any local assets live relative to it).
func resolveComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "cloudformation.resolveComponentPath")()

	return u.GetComponentPath(atmosConfig, cfg.CloudFormationComponentType, info.ComponentFolderPrefix, info.FinalComponent)
}

// resolveTemplateFilePath resolves the component's template file's absolute
// path, joined with componentPath when spec.TemplatePath is relative.
func resolveTemplateFilePath(componentPath string, spec *stackSpec) string {
	templateFile := spec.TemplatePath
	if !filepath.IsAbs(templateFile) {
		templateFile = filepath.Join(componentPath, templateFile)
	}
	return templateFile
}

// loadTemplateBody reads the component's template file from disk, resolved
// relative to componentPath. Returns ErrMissingAwsCloudFormationTemplate wrapped
// with the resolved path when the file cannot be read.
func loadTemplateBody(componentPath string, spec *stackSpec) (string, error) {
	defer perf.Track(nil, "cloudformation.loadTemplateBody")()

	templateFile := resolveTemplateFilePath(componentPath, spec)

	data, err := os.ReadFile(templateFile)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMissingAwsCloudFormationTemplate, templateFile, err)
	}
	return string(data), nil
}

// loadStackPolicyBody reads the component's stack policy file from disk, if configured.
// Returns an empty string when no stack_policy.file is set.
func loadStackPolicyBody(componentPath string, spec *stackSpec) (string, error) {
	defer perf.Track(nil, "cloudformation.loadStackPolicyBody")()

	if spec.StackPolicyFile == "" {
		return "", nil
	}

	policyFile := spec.StackPolicyFile
	if !filepath.IsAbs(policyFile) {
		policyFile = filepath.Join(componentPath, policyFile)
	}

	data, err := os.ReadFile(policyFile)
	if err != nil {
		return "", fmt.Errorf("stack policy file %s: %w", policyFile, err)
	}
	return string(data), nil
}
