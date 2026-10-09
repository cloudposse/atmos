package manifest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Rain (the archived AWS CloudFormation CLI) preprocessed templates before deployment, resolving
// `!Rain::*` directives and `${Rain::Constant}` substitutions into plain CloudFormation. Atmos
// never preprocesses a template file, so a template still carrying those directives is not valid
// CloudFormation and would only fail at the API ("YAML not well-formed (line N, column M)") with
// no mention of Rain. This file recognizes them locally and names the Atmos-native replacement for
// each -- parity, not compatibility: there is no Rain emulation.

const (
	rainTagPrefix       = "!Rain::"
	rainSubPrefix       = "${Rain::"
	rainMigrationGuide  = "https://atmos.tools/migration/rain"
	rainDirectiveModule = "Module"
)

// rainDirectiveParity maps each Rain directive to its Atmos-native replacement.
var rainDirectiveParity = map[string]string{
	"Env":      "`!env NAME` in the component's `parameters:`, or inside the template when it is included with `template: !include template.yaml | eval`",
	"Constant": "a CloudFormation Parameter fed from stack `vars:`, or `{{ .vars.name }}` inside an inline template",
	"Include":  "`!include fragment.yaml` inside the template, included with `template: !include template.yaml | eval`",
	"Embed":    "`!include file` inside the template, included with `template: !include template.yaml | eval` (a non-YAML file loads as a string)",
	"S3":       "an `archive` + `publish` hook step before apply, with the uploaded key passed through `parameters:`",
	"S3Http":   "an `archive` + `publish` hook step before apply, with the uploaded URL passed through `parameters:`",
	"Module":   "no Atmos equivalent; expand modules in a build step (or use AWS CDK) and point `path:` at the output",
}

var rainDirectivePattern = regexp.MustCompile(`!Rain::([A-Za-z0-9]+)|\$\{Rain::([A-Za-z0-9_]+)\}`)

// rainSectionDirectives maps a Rain-specific top-level template section to the directive its
// migration hint belongs to: `Rain:` holds `Constants:` (and other Rain settings), `Modules:` is
// Rain's module system.
var rainSectionDirectives = map[string]string{
	"Rain":    "Constant",
	"Modules": rainDirectiveModule,
}

// RainDirectiveHint returns the migration hint for a `!Rain::<Directive>` YAML tag, or "" for any
// other tag. It is registered with the stack-manifest tag walker so a Rain directive inside an
// inline or `!include`d template fails with the replacement named, not the generic supported-tags
// list.
func RainDirectiveHint(tag string) []string {
	defer perf.Track(nil, "cloudformation.manifest.RainDirectiveHint")()

	tag = strings.TrimSpace(tag)
	if !strings.HasPrefix(tag, rainTagPrefix) {
		return nil
	}
	return rainHints([]string{strings.TrimPrefix(tag, rainTagPrefix)})
}

// DetectRainDirectives inspects a template body for Rain usage and returns the distinct directive
// names in order of first appearance: `!Rain::X` tags, `${Rain::X}` substitutions inside scalar
// values (reported as "Constant", the only directive that form belongs to), and the Rain-specific
// top-level sections `Rain:` and `Modules:`. The template is parsed as YAML so that the words in a
// `Description:` or a comment never count as directives; a body that is not parseable YAML falls
// back to a plain text scan, since CloudFormation would reject it anyway.
func DetectRainDirectives(body string) []string {
	defer perf.Track(nil, "cloudformation.manifest.DetectRainDirectives")()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return scanRainText(body)
	}
	collector := &rainCollector{seen: map[string]bool{}}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			if name, ok := rainSectionDirectives[root.Content[i].Value]; ok {
				collector.add(name)
			}
		}
	}
	collector.walk(root)
	return collector.found
}

// rainCollector accumulates distinct directive names in document order.
type rainCollector struct {
	seen  map[string]bool
	found []string
}

func (c *rainCollector) add(name string) {
	if !c.seen[name] {
		c.seen[name] = true
		c.found = append(c.found, name)
	}
}

func (c *rainCollector) walk(node *yaml.Node) {
	if node == nil {
		return
	}
	if tag := strings.TrimSpace(node.Tag); strings.HasPrefix(tag, rainTagPrefix) {
		c.add(strings.TrimPrefix(tag, rainTagPrefix))
	}
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, rainSubPrefix) {
		c.add("Constant")
	}
	for _, child := range node.Content {
		c.walk(child)
	}
}

// scanRainText is the fallback for a body that does not parse as YAML.
func scanRainText(body string) []string {
	collector := &rainCollector{seen: map[string]bool{}}
	for _, match := range rainDirectivePattern.FindAllStringSubmatch(body, -1) {
		name := match[1]
		if name == "" {
			name = "Constant"
		}
		collector.add(name)
	}
	return collector.found
}

// RainDirectiveError builds the error for a `path:` template file that still contains Rain
// directives, listing each directive found and its Atmos-native replacement.
func RainDirectiveError(templateFile string, directives []string) error {
	defer perf.Track(nil, "cloudformation.manifest.RainDirectiveError")()

	builder := errUtils.Build(errUtils.ErrAwsCloudFormationRainDirective).
		WithExplanationf("%s uses Rain directives (%s). Rain resolved these by preprocessing the template before deployment; Atmos sends a `path:` template to CloudFormation as written, so CloudFormation would reject it.", templateFile, strings.Join(directives, ", "))
	for _, hint := range rainHints(directives) {
		builder = builder.WithHint(hint)
	}
	return builder.
		WithContext("template", templateFile).
		WithContext("directives", strings.Join(directives, ",")).
		Err()
}

// rainHints renders one hint per directive (its Atmos-native replacement) plus the guide link.
func rainHints(directives []string) []string {
	names := append([]string(nil), directives...)
	sort.Strings(names)
	hints := make([]string, 0, len(names)+1)
	for _, name := range names {
		replacement, ok := rainDirectiveParity[name]
		if !ok {
			replacement = "not a directive Rain documented; remove it or move the value into stack configuration"
		}
		hints = append(hints, fmt.Sprintf("Replace !Rain::%s with %s.", name, replacement))
	}
	hints = append(hints, "Migration guide: "+rainMigrationGuide)
	return hints
}
