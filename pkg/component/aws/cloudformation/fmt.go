package cloudformation

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// yamlIndent is the indentation width formatTemplate re-emits with, matching
// this codebase's general YAML formatting convention.
const yamlIndent = 2

// templateFilePermissions matches the mode template files are typically
// authored with.
const templateFilePermissions = 0o644

// formatTemplate re-serializes a CloudFormation template with consistent
// indentation, preserving comments and key order via yaml.v3's Node API.
// Plain gopkg.in/yaml.v3 (not the Atmos custom-tag-aware u.UnmarshalYAML) is
// used deliberately, matching parameters.go's registerNoEchoValues. CFN's
// short-form intrinsic function tags (such as Ref, Sub, and GetAtt) would
// otherwise be rejected as unknown Atmos custom tags. There is no cfn-format
// binary to shell out to (and Rain's own formatter is archived along with the
// rest of Rain — see the PRD's migration notes), so this is a native,
// dependency-free round-trip rather than a wrapped external tool.
func formatTemplate(body string) (string, error) {
	defer perf.Track(nil, "cloudformation.formatTemplate")()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}

	return buf.String(), nil
}

// runFmt formats the component's local template. With --check, reports
// whether the file is already formatted (via ErrAwsCloudFormationFmtNotClean,
// for CI) without writing; otherwise formats in place.
func runFmt(spec *stackSpec, flags map[string]any, summary map[string]any) (map[string]any, error) {
	if spec.TemplateAbsPath == "" {
		return summary, errUtils.ErrAwsCloudFormationFmtRequiresPath
	}

	formatted, err := formatTemplate(spec.TemplateBody)
	if err != nil {
		return summary, err
	}

	clean := formatted == spec.TemplateBody
	summary["formatted"] = !clean

	check, _ := flags["check"].(bool)
	if check {
		if !clean {
			_ = data.Writeln(fmt.Sprintf("%s: not formatted", spec.TemplateAbsPath))
			return summary, fmt.Errorf("%w: %s", errUtils.ErrAwsCloudFormationFmtNotClean, spec.TemplateAbsPath)
		}
		_ = data.Writeln(fmt.Sprintf("%s: formatted", spec.TemplateAbsPath))
		return summary, nil
	}

	if clean {
		return summary, nil
	}
	if err := os.WriteFile(spec.TemplateAbsPath, []byte(formatted), templateFilePermissions); err != nil {
		return summary, fmt.Errorf("%w: %s: %w", errUtils.ErrAwsCloudFormationFmtNotClean, spec.TemplateAbsPath, err)
	}
	_ = data.Writeln(fmt.Sprintf("%s: formatted", spec.TemplateAbsPath))
	return summary, nil
}
