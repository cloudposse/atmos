package cloudformation

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/filesystem"
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

// forceBlockStyle recursively resets every node's Style to the encoder's
// default heuristics. A plain yaml.Node round-trip preserves each node's
// original style, so content that started as JSON (CloudFormation's common
// stored representation for a deployed template, regardless of how it was
// originally authored) survives formatTemplate's re-indentation still
// wrapped in JSON's flow notation (`{...}`/`[...]`) and double-quoted keys
// instead of rendering as normal, idiomatic YAML. Only Style controls
// flow/quoting presentation; Tag (which carries intrinsic function short
// forms like !Ref/!Sub/!GetAtt) is untouched.
func forceBlockStyle(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style = 0
	for _, child := range node.Content {
		forceBlockStyle(child)
	}
}

// formatTemplateAsBlockYAML re-serializes a template body as pretty-printed,
// block-style YAML regardless of whether CloudFormation returned it as JSON
// or YAML — for read-only display (e.g. `get template`), where there's no
// locally-authored file/style to preserve. This differs from formatTemplate,
// which deliberately preserves a local file's existing style/comments for
// `aws cfn fmt`.
func formatTemplateAsBlockYAML(body string) (string, error) {
	defer perf.Track(nil, "cloudformation.formatTemplateAsBlockYAML")()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", fmt.Errorf(wrapFmt, errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	forceBlockStyle(&doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf(wrapFmt, errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf(wrapFmt, errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}

	return buf.String(), nil
}

// runFmt formats the component's local template. With --check, reports
// whether the file is already formatted (via ErrAwsCloudFormationFmtNotClean,
// for CI) without writing; otherwise formats in place.
func runFmt(spec *stackSpec, flags map[string]any, summary map[string]any) (map[string]any, error) {
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
	if err := writeTemplateAtomic(spec.TemplateAbsPath, formatted); err != nil {
		return summary, fmt.Errorf("%w: %s: %w", errUtils.ErrAwsCloudFormationFmtWriteFailed, spec.TemplateAbsPath, err)
	}
	_ = data.Writeln(fmt.Sprintf("%s: formatted", spec.TemplateAbsPath))
	return summary, nil
}

// writeTemplateAtomic writes the formatted template to a temp file in the same directory and
// renames it over TemplateAbsPath (via pkg/filesystem's WriteFileAtomic), instead of truncating
// the existing file in place with os.WriteFile. A disk-full or I/O error mid-write then leaves
// the original template untouched rather than empty or partially overwritten. The original
// file's mode is preserved when it can be read; templateFilePermissions is used as a fallback
// (e.g. the file was somehow removed between load and format).
func writeTemplateAtomic(path, formatted string) error {
	defer perf.Track(nil, "cloudformation.writeTemplateAtomic")()

	mode := os.FileMode(templateFilePermissions)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(path, []byte(formatted), mode)
}
