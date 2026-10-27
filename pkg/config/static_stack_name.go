package config

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// RenderStaticStackName renders merged fields with Go's default delimiters,
// matching runtime stack naming. Only pure Go built-ins are available.
func RenderStaticStackName(source, file string, section map[string]any) (string, error) {
	defer perf.Track(nil, "config.RenderStaticStackName")()

	tmpl, err := template.New("static-stack-name").Option("missingkey=error").Parse(source)
	if err != nil {
		return "", staticStackNameError(file, err)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, literalsource.RestoreMap(section)); err != nil {
		return "", staticStackNameError(file, err)
	}
	if yamlsource.Contains(output.String()) {
		return "", staticStackNameError(file, errUtils.ErrTemplateEvaluation)
	}
	return output.String(), nil
}

func staticStackNameError(file string, err error) error {
	return fmt.Errorf("%w: cannot determine stack name for %q without evaluating dynamic expressions: %w", errUtils.ErrTemplateEvaluation, file, err)
}

// StaticStackManifestName restores literal names and refuses unresolved expressions,
// whose unknown result could otherwise hide a matching stack from validation.
func StaticStackManifestName(config *schema.AtmosConfiguration, file, name string) (string, error) {
	defer perf.Track(config, "config.StaticStackManifestName")()

	if literal, ok := literalsource.Decode(name); ok {
		return literal, nil
	}
	delims, err := delimiters.Parse(config.Templates.Settings.Delimiters)
	if err != nil {
		return "", err
	}
	if yamlsource.Contains(name) || delims.ContainsAny(name) {
		return "", fmt.Errorf("%w: cannot determine stack name for %q without evaluating dynamic expressions", errUtils.ErrTemplateEvaluation, file)
	}
	return name, nil
}

// ValidateStaticStackNamePattern checks only fields consumed by the legacy pattern.
// Literal fields remain data; unused dynamic variables do not affect the name.
func ValidateStaticStackNamePattern(config *schema.AtmosConfiguration, file string, vars map[string]any) error {
	defer perf.Track(config, "config.ValidateStaticStackNamePattern")()

	for _, part := range strings.Split(config.Stacks.NamePattern, "-") {
		switch part {
		case "{namespace}", "{tenant}", "{environment}", "{stage}":
			if value, ok := vars[strings.Trim(part, "{}")].(string); ok {
				if _, err := StaticStackManifestName(config, file, value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
