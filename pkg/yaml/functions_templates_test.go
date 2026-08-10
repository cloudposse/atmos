package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file locks in a hard requirement: edits must preserve Atmos custom YAML
// function tags (!terraform.output, !env, !store, ...) and embedded Go/Gomplate
// templates ({{ ... }}) verbatim. Stack manifests rely on both heavily, so an
// edit that silently strips a tag or mangles a template delimiter would corrupt
// configuration.

const fixtureFunctionsTemplates = `vars:
  # Atmos YAML functions (custom tags).
  password: !terraform.output rds admin_password
  region: !env AWS_REGION
  secret: !store ssm /app/secret
  vpc_id: !terraform.state vpc default vpc_id
  # Go / Gomplate templates as scalar values.
  name: '{{ .vars.namespace }}-{{ .vars.stage }}'
  greeting: "Hello {{ .name | upper }}"
  multiline: |
    region: {{ .region }}
    account: {{ .account_id }}
  unquoted_tpl: prefix-{{ .suffix }}
  target: old
`

// preservedTokens must all appear verbatim after any unrelated edit.
var preservedTokens = []string{
	"!terraform.output rds admin_password",
	"!env AWS_REGION",
	"!store ssm /app/secret",
	"!terraform.state vpc default vpc_id",
	"'{{ .vars.namespace }}-{{ .vars.stage }}'",
	`"Hello {{ .name | upper }}"`,
	"region: {{ .region }}",
	"account: {{ .account_id }}",
	"prefix-{{ .suffix }}",
}

func TestPreserve_FunctionsAndTemplates_OnUnrelatedSet(t *testing.T) {
	out, err := Set([]byte(fixtureFunctionsTemplates), "vars.target", "new")
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "target: new", "target updated")
	for _, tok := range preservedTokens {
		assert.Containsf(t, s, tok, "token must be preserved verbatim: %q", tok)
	}
}

func TestPreserve_FunctionsAndTemplates_OnDelete(t *testing.T) {
	out, err := Delete([]byte(fixtureFunctionsTemplates), "vars.target")
	require.NoError(t, err)
	s := string(out)

	assert.NotContains(t, s, "target:")
	for _, tok := range preservedTokens {
		assert.Containsf(t, s, tok, "token must survive delete: %q", tok)
	}
}

func TestPreserve_FunctionsAndTemplates_OnFormat(t *testing.T) {
	out, err := Format([]byte(fixtureFunctionsTemplates))
	require.NoError(t, err)
	s := string(out)

	for _, tok := range preservedTokens {
		assert.Containsf(t, s, tok, "token must survive format: %q", tok)
	}
}

// TestSet_TemplateValueRoundTrips verifies we can SET a value that is itself a
// Go template and read it back unchanged (delimiters intact).
func TestSet_TemplateValueRoundTrips(t *testing.T) {
	const src = "vars:\n  name: placeholder\n"
	tpl := "{{ .vars.namespace }}-{{ .vars.stage }}"

	out, err := Set([]byte(src), "vars.name", tpl)
	require.NoError(t, err)

	got, err := Get(out, "vars.name")
	require.NoError(t, err)
	assert.Equal(t, tpl, got, "template value must round-trip without delimiter mangling")
}

// TestSet_AdjacentToFunctionKeepsTag confirms that updating a sibling key does
// not strip the function tag on a neighboring key.
func TestSet_AdjacentToFunctionKeepsTag(t *testing.T) {
	const src = `vars:
  region: !env AWS_REGION
  stage: dev
`
	out, err := Set([]byte(src), "vars.stage", "prod")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "stage: prod")
	assert.Contains(t, s, "!env AWS_REGION", "neighboring function tag preserved")
}
