package exec

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateDelimiterSettingsSourcePreservation(t *testing.T) {
	for name, declaration := range map[string]string{
		"flow":    `      delimiters: ['<<', '>>']`,
		"escaped": `      delimiters: ["\u003c\u003c", "\u003e\u003e"]`,
		"block": `      delimiters:
        - <<
        - ">>"`,
		"literal-scalars": "      delimiters:\n        - |-\n          <<\n        - |-\n          >>",
	} {
		t.Run(name, func(t *testing.T) {
			input := "settings:\n  templates:\n    settings:\n" + declaration + "\nvars:\n  value: '<< .stage >>'\n"
			protected, restoreSettings := protectTemplateDelimiterSettings(input)
			protected, restoreLiterals := literalsource.ProtectEncoded(protected, "<<", ">>")
			tmpl, err := template.New("test").Delims("<<", ">>").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, tmpl.Execute(&output, map[string]any{"stage": "dev"}))
			assert.Equal(t, "settings:\n  templates:\n    settings:\n"+declaration+"\nvars:\n  value: 'dev'\n", restoreSettings(restoreLiterals(output.String())))
		})
	}
}
