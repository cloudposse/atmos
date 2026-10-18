package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchesTag(t *testing.T) {
	tests := []struct {
		name, input, prefix string
		want                bool
	}{
		{"whitespace boundary", "!terraform.state app dev out", "!terraform.state", true},
		{"index boundary", "!terraform.state[0]", "!terraform.state", true},
		{"object boundary", "!template{\"k\":1}", "!template", true},
		{"end of string", "!aws.region", "!aws.region", true},
		{"name continuation letter", "!git.branches are protected", "!git.branch", false},
		{"name continuation dot", "!env.FOO", "!env", false},
		{"name continuation dash", "!env-x", "!env", false},
		{"name continuation underscore", "!env_x", "!env", false},
		{"name continuation digit", "!env2", "!env", false},
		{"different prefix", "!exec ls", "!env", false},
		{"missing bang", "env x", "!env", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchesTag(tt.input, tt.prefix))
		})
	}
}

func TestStringFormYAMLIsSubsetOfCatalog(t *testing.T) {
	stringForm := StringFormYAML()
	assert.NotEmpty(t, stringForm)
	for _, name := range stringForm {
		assert.True(t, IsValidYAML(name), "%s must be a registered function", name)
	}
	// Functions consumed from the YAML node tag are never string-form.
	for _, name := range []string{ToYAML(CEL), ToYAML(Include), ToYAML(IncludeRaw), ToYAML(Append), ToYAML(Cwd), ToYAML(Random), ToYAML(Literal), ToYAML(Starlark)} {
		assert.NotContains(t, stringForm, name)
	}
}

func TestMatchesStringForm(t *testing.T) {
	for value, want := range map[string]bool{
		"!terraform.state app dev output":  true,
		"!terraform.state[0]":              true,
		"!git.branch":                      true,
		"!aws.account_id":                  true,
		"!git.branches are protected":      false,
		"!terraform.staet app dev output":  false,
		"!cwd":                             false,
		"!include other.yaml":              false,
		"!important":                       false,
		"plain text":                       false,
		"":                                 false,
		"!literal !terraform.state a b c.": false,
	} {
		assert.Equal(t, want, MatchesStringForm(value), value)
	}
}
