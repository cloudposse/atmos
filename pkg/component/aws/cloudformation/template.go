package cloudformation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/aws/cloudformation/manifest"
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
	body := string(data)
	// A template a user brings over from Rain may still carry `!Rain::` directives. CloudFormation
	// rejects those with a bare "YAML not well-formed" at the API; catch them here, before any
	// AWS call, and name the Atmos-native replacement for each.
	if directives := manifest.DetectRainDirectives(body); len(directives) > 0 {
		return "", manifest.RainDirectiveError(templateFile, directives)
	}
	return body, nil
}

// loadStackPolicyBody reads the component's stack policy file from disk, if configured.
// An inline stack_policy.body is already in the spec and never touches the component directory,
// so it is returned unchanged. Returns an empty string when neither is set.
func loadStackPolicyBody(componentPath string, spec *stackSpec) (string, error) {
	defer perf.Track(nil, "cloudformation.loadStackPolicyBody")()

	if spec.StackPolicyFile == "" {
		return spec.StackPolicyBody, nil
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

// inferSourceTemplatePath inspects the provisioned result, never the URI's
// extension. Only an unambiguous regular root file can become the template.
func inferSourceTemplatePath(componentPath string) (string, error) {
	entries, err := os.ReadDir(componentPath)
	if err != nil {
		return "", fmt.Errorf("%w: inspect provisioned source: %w", errUtils.ErrMissingAwsCloudFormationTemplate, err)
	}
	var name string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if name != "" || !entry.Type().IsRegular() {
			return "", errUtils.Build(errUtils.ErrMissingAwsCloudFormationTemplate).WithHint("Set path explicitly when the provisioned source contains a directory or multiple files.").Err()
		}
		name = entry.Name()
	}
	if name == "" {
		return "", errUtils.ErrMissingAwsCloudFormationTemplate
	}
	return name, nil
}

// resolveTemplateBody preserves inline templates, infers a missing source filename when unambiguous,
// and loads the resolved file into the stack specification.
func resolveTemplateBody(componentPath string, spec *stackSpec) error {
	if spec.TemplateBody != "" {
		return nil
	}
	var err error
	if spec.TemplatePath == "" {
		spec.TemplatePath, err = inferSourceTemplatePath(componentPath)
		if err != nil {
			return err
		}
	}
	spec.TemplateAbsPath = resolveTemplateFilePath(componentPath, spec)
	spec.TemplateBody, err = loadTemplateBody(componentPath, spec)
	return err
}
