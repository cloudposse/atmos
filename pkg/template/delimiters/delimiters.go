// Package delimiters resolves and applies the Go template delimiters that
// `templates.settings.delimiters` configures in atmos.yaml.
//
// Stack manifests have always honored the configured delimiters. Workflow,
// custom-command, and lifecycle-hook step templates are edition-gated: from the
// "templates.step_delimiters" behavior transition on, they use the same pair,
// while projects pinned to an earlier edition keep the default "{{" and "}}".
// Callers resolve the effective pair once per invocation with ForSteps and pass
// the resulting Delimiters value down to every site that detects, protects, or
// parses step templates.
package delimiters

import (
	"fmt"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultLeft is the standard Go template left delimiter.
	DefaultLeft = "{{"
	// DefaultRight is the standard Go template right delimiter.
	DefaultRight = "}}"
	// StepBehaviorKey is the edition journal transition key that lets step templates
	// honor templates.settings.delimiters.
	StepBehaviorKey = "templates.step_delimiters"
)

// Delimiters is a left and right Go template delimiter pair. The zero value is
// the default pair, so a Delimiters that was never configured behaves like Default.
type Delimiters struct {
	Left  string
	Right string
}

// Default returns the standard "{{" and "}}" pair.
func Default() Delimiters {
	defer perf.Track(nil, "delimiters.Default")()

	return Delimiters{Left: DefaultLeft, Right: DefaultRight}
}

// Parse validates a configured delimiter list and returns the pair it selects.
// An empty list selects the default pair; any other list must hold exactly two
// non-empty strings (left, then right).
func Parse(configured []string) (Delimiters, error) {
	defer perf.Track(nil, "delimiters.Parse")()

	if len(configured) == 0 {
		return Default(), nil
	}

	if len(configured) != 2 || configured[0] == "" || configured[1] == "" {
		return Delimiters{}, fmt.Errorf("%w: invalid 'templates.settings.delimiters' config in 'atmos.yaml': %v\n"+
			"'delimiters' must be an array with two string items: left and right delimiter\n"+
			"the left and right delimiters must not be an empty string", errUtils.ErrInvalidTemplateSettings, configured)
	}

	return Delimiters{Left: configured[0], Right: configured[1]}, nil
}

// ForSteps returns the delimiters that workflow, custom-command, and lifecycle-hook step
// templates use. The configured templates.settings.delimiters apply when the project's
// edition pin selects the "templates.step_delimiters" behavior (unpinned projects and pins on
// or after its date); earlier pins keep the default pair. An invalid pin or an invalid
// configured pair is returned as an error instead of silently selecting a pair.
func ForSteps(atmosConfig *schema.AtmosConfiguration) (Delimiters, error) {
	defer perf.Track(atmosConfig, "delimiters.ForSteps")()

	pin := ""
	if atmosConfig != nil {
		pin = atmosConfig.Edition
	}
	honorConfigured, err := edition.BehaviorChanged(pin, StepBehaviorKey)
	if err != nil {
		return Default(), err
	}
	if !honorConfigured || atmosConfig == nil {
		return Default(), nil
	}

	return Parse(atmosConfig.Templates.Settings.Delimiters)
}

// effective returns d, or the default pair for the zero value or a half-set pair.
func (d Delimiters) effective() Delimiters {
	if d.Left == "" || d.Right == "" {
		return Default()
	}
	return d
}

// Normalize returns the pair itself, or the default pair when d is the zero value.
func (d Delimiters) Normalize() Delimiters {
	defer perf.Track(nil, "delimiters.Delimiters.Normalize")()

	return d.effective()
}

// IsDefault reports whether d is the standard "{{" and "}}" pair.
func (d Delimiters) IsDefault() bool {
	defer perf.Track(nil, "delimiters.Delimiters.IsDefault")()

	return d.effective() == Default()
}

// Contains reports whether s holds a template opening, which is the left delimiter.
func (d Delimiters) Contains(s string) bool {
	defer perf.Track(nil, "delimiters.Delimiters.Contains")()

	return strings.Contains(s, d.effective().Left)
}

// ContainsAny reports whether s holds either the left or the right delimiter. It is the
// conservative check for text that might be a half-written template.
func (d Delimiters) ContainsAny(s string) bool {
	defer perf.Track(nil, "delimiters.Delimiters.ContainsAny")()

	e := d.effective()
	return strings.Contains(s, e.Left) || strings.Contains(s, e.Right)
}

// Wrap returns expr as one template action, for example "{{ .step.name }}".
func (d Delimiters) Wrap(expr string) string {
	defer perf.Track(nil, "delimiters.Delimiters.Wrap")()

	e := d.effective()
	return e.Left + " " + expr + " " + e.Right
}

// Protect replaces every left delimiter in s with placeholder, so the text cannot open a
// template action. Unprotect reverses it.
func (d Delimiters) Protect(s, placeholder string) string {
	defer perf.Track(nil, "delimiters.Delimiters.Protect")()

	return strings.ReplaceAll(s, d.effective().Left, placeholder)
}

// Unprotect restores the left delimiters that Protect replaced with placeholder.
func (d Delimiters) Unprotect(s, placeholder string) string {
	defer perf.Track(nil, "delimiters.Delimiters.Unprotect")()

	return strings.ReplaceAll(s, placeholder, d.effective().Left)
}

// Apply sets the pair on t and returns t. It must run before t parses its text.
func (d Delimiters) Apply(t *template.Template) *template.Template {
	defer perf.Track(nil, "delimiters.Delimiters.Apply")()

	e := d.effective()
	return t.Delims(e.Left, e.Right)
}
