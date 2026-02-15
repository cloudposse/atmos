package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

func TestValidateYAMLSyntax(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		wantDiagnostics   int
		checkDiagnostic   func(t *testing.T, diag protocol.Diagnostic)
		expectErrorAtLine uint32
	}{
		{
			name:            "valid YAML",
			content:         "key: value\nlist:\n  - item1\n  - item2",
			wantDiagnostics: 0,
		},
		{
			name:              "invalid YAML - tab indentation",
			content:           "key:\n\tvalue",
			wantDiagnostics:   1,
			expectErrorAtLine: 1, // Error on line 2 (0-indexed: line 1)
			checkDiagnostic: func(t *testing.T, diag protocol.Diagnostic) {
				assert.Contains(t, diag.Message, "YAML syntax error")
				assert.Equal(t, protocol.DiagnosticSeverityError, *diag.Severity)
				assert.Equal(t, "atmos-lsp", *diag.Source)
			},
		},
		{
			name:              "invalid YAML - mapping values not allowed",
			content:           "key: value\ninvalid mapping\nkey2: value2",
			wantDiagnostics:   1,
			expectErrorAtLine: 1, // Error on line 2
			checkDiagnostic: func(t *testing.T, diag protocol.Diagnostic) {
				assert.Contains(t, diag.Message, "YAML syntax error")
			},
		},
		{
			name:              "invalid YAML - unclosed quote",
			content:           "key: \"unclosed\nkey2: value",
			wantDiagnostics:   1,
			expectErrorAtLine: 0, // Error typically on first line
			checkDiagnostic: func(t *testing.T, diag protocol.Diagnostic) {
				assert.Contains(t, diag.Message, "YAML syntax error")
			},
		},
		{
			name:            "empty YAML",
			content:         "",
			wantDiagnostics: 0,
		},
		{
			name:            "YAML comments only",
			content:         "# Comment 1\n# Comment 2",
			wantDiagnostics: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{
				URI:  "file:///test.yaml",
				Text: tt.content,
			}

			handler := &Handler{}
			diagnostics := handler.validateYAMLSyntax(doc)

			assert.Len(t, diagnostics, tt.wantDiagnostics)

			if tt.wantDiagnostics > 0 && tt.checkDiagnostic != nil {
				tt.checkDiagnostic(t, diagnostics[0])

				// Verify error position is not hardcoded 0:0 (unless it's actually at line 0).
				if tt.expectErrorAtLine > 0 {
					assert.Greater(t, diagnostics[0].Range.Start.Line, uint32(0),
						"Error position should not be hardcoded to 0:0")
				}
			}
		})
	}
}

func TestExtractErrorPosition(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantLine    int
		wantCol     int
		description string
	}{
		{
			name:        "nil error",
			err:         nil,
			wantLine:    0,
			wantCol:     0,
			description: "Returns 0:0 for nil error",
		},
		{
			name:        "line number pattern",
			err:         &yaml.TypeError{Errors: []string{"yaml: line 5: mapping values are not allowed"}},
			wantLine:    4, // 1-based to 0-based
			wantCol:     0,
			description: "Extracts line number from 'line X' pattern",
		},
		{
			name:        "line and column pattern",
			err:         &yaml.TypeError{Errors: []string{"yaml: line 10: column 15: invalid syntax"}},
			wantLine:    9,  // 1-based to 0-based
			wantCol:     14, // 1-based to 0-based
			description: "Extracts line and column from 'line X: column Y' pattern",
		},
		{
			name:        "error without line info",
			err:         &yaml.TypeError{Errors: []string{"generic yaml error"}},
			wantLine:    0,
			wantCol:     0,
			description: "Returns 0:0 when no line info found",
		},
		{
			name:        "line at start of file",
			err:         &yaml.TypeError{Errors: []string{"line 1: error"}},
			wantLine:    0, // Line 1 becomes 0 (0-indexed)
			wantCol:     0,
			description: "Correctly converts line 1 to index 0",
		},
		{
			name:        "large line number",
			err:         &yaml.TypeError{Errors: []string{"line 1234: error"}},
			wantLine:    1233,
			wantCol:     0,
			description: "Handles large line numbers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{}
			line, col := handler.extractErrorPosition(tt.err)

			assert.Equal(t, tt.wantLine, line, tt.description)
			assert.Equal(t, tt.wantCol, col, tt.description)
		})
	}
}

func TestValidateAtmosStack(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantDiagnostics int
		checkErrors     func(t *testing.T, diags []protocol.Diagnostic)
	}{
		{
			name: "valid stack with all sections",
			content: `import:
  - catalog/vpc
components:
  terraform:
    vpc:
      vars:
        cidr: 10.0.0.0/16
vars:
  region: us-east-1`,
			wantDiagnostics: 0,
		},
		{
			name: "import as string instead of array",
			content: `import: "not-an-array"
components:
  terraform:
    vpc: {}`,
			wantDiagnostics: 1,
			checkErrors: func(t *testing.T, diags []protocol.Diagnostic) {
				assert.Contains(t, diags[0].Message, "'import' should be an array")
			},
		},
		{
			name: "components as array instead of map",
			content: `import: []
components:
  - invalid`,
			wantDiagnostics: 1,
			checkErrors: func(t *testing.T, diags []protocol.Diagnostic) {
				assert.Contains(t, diags[0].Message, "'components' should be a map")
			},
		},
		{
			name: "terraform section as array",
			content: `components:
  terraform:
    - invalid`,
			wantDiagnostics: 1,
			checkErrors: func(t *testing.T, diags []protocol.Diagnostic) {
				assert.Contains(t, diags[0].Message, "'components.terraform' should be a map")
			},
		},
		{
			name: "helmfile section as array",
			content: `components:
  helmfile:
    - invalid`,
			wantDiagnostics: 1,
			checkErrors: func(t *testing.T, diags []protocol.Diagnostic) {
				assert.Contains(t, diags[0].Message, "'components.helmfile' should be a map")
			},
		},
		{
			name: "vars as array instead of map",
			content: `vars:
  - invalid`,
			wantDiagnostics: 1,
			checkErrors: func(t *testing.T, diags []protocol.Diagnostic) {
				assert.Contains(t, diags[0].Message, "'vars' should be a map")
			},
		},
		{
			name: "multiple validation errors",
			content: `import: "not-an-array"
components:
  - invalid
vars:
  - invalid`,
			wantDiagnostics: 3, // import, components, vars all invalid
		},
		{
			name:            "empty stack",
			content:         ``,
			wantDiagnostics: 0,
		},
		{
			name: "valid minimal stack",
			content: `components:
  terraform: {}`,
			wantDiagnostics: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{
				URI:  "file:///test.yaml",
				Text: tt.content,
			}

			handler := &Handler{}
			diagnostics := handler.validateAtmosStack(doc)

			assert.Len(t, diagnostics, tt.wantDiagnostics)

			if tt.checkErrors != nil && len(diagnostics) > 0 {
				tt.checkErrors(t, diagnostics)
			}

			// Verify all diagnostics have severity and source.
			for _, diag := range diagnostics {
				assert.NotNil(t, diag.Severity)
				assert.NotNil(t, diag.Source)
				assert.Equal(t, "atmos-lsp", *diag.Source)
			}
		})
	}
}

func TestValidateAtmosFile(t *testing.T) {
	tests := []struct {
		name            string
		uri             string
		content         string
		wantDiagnostics int
		description     string
	}{
		{
			name:            "valid YAML stack file",
			uri:             "file:///stacks/prod.yaml",
			content:         "components:\n  terraform: {}",
			wantDiagnostics: 0,
			description:     "Valid YAML should have no diagnostics",
		},
		{
			name:            "invalid YAML syntax",
			uri:             "file:///stacks/prod.yaml",
			content:         "key:\n\tvalue",
			wantDiagnostics: 1,
			description:     "YAML syntax errors should be caught",
		},
		{
			name:            "invalid Atmos structure after valid YAML",
			uri:             "file:///stacks/prod.yaml",
			content:         "import: not-an-array",
			wantDiagnostics: 1,
			description:     "Atmos structure validation should run after YAML syntax validation",
		},
		{
			name:            "YAML syntax error prevents Atmos validation",
			uri:             "file:///stacks/prod.yaml",
			content:         "invalid:\n\ttab\nimport: not-an-array",
			wantDiagnostics: 1, // Only YAML syntax error, Atmos validation skipped
			description:     "Atmos validation should be skipped if YAML syntax is invalid",
		},
		{
			name:            "YML extension",
			uri:             "file:///stacks/prod.yml",
			content:         "components:\n  terraform: {}",
			wantDiagnostics: 0,
			description:     ".yml extension should be recognized",
		},
		{
			name:            "non-YAML file",
			uri:             "file:///main.tf",
			content:         "resource \"aws_vpc\" {}",
			wantDiagnostics: 0,
			description:     "Non-YAML files should not be validated",
		},
		{
			name:            "unknown extension",
			uri:             "file:///test.txt",
			content:         "some text",
			wantDiagnostics: 0,
			description:     "Unknown file types should not be validated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{
				URI:  tt.uri,
				Text: tt.content,
			}

			handler := &Handler{}
			diagnostics := handler.validateAtmosFile(doc)

			assert.Len(t, diagnostics, tt.wantDiagnostics, tt.description)
		})
	}
}

func TestCreateDiagnostic(t *testing.T) {
	handler := &Handler{}
	message := "Test error message"

	diag := handler.createDiagnostic(message)

	// Verify diagnostic structure.
	assert.Equal(t, message, diag.Message)
	assert.Equal(t, uint32(0), diag.Range.Start.Line)
	assert.Equal(t, uint32(0), diag.Range.Start.Character)
	assert.Equal(t, uint32(0), diag.Range.End.Line)
	assert.Equal(t, uint32(0), diag.Range.End.Character)
	assert.NotNil(t, diag.Severity)
	assert.Equal(t, protocol.DiagnosticSeverityError, *diag.Severity)
	assert.NotNil(t, diag.Source)
	assert.Equal(t, "atmos-lsp", *diag.Source)
}

func TestValidateImportSection(t *testing.T) {
	tests := []struct {
		name            string
		stackContent    map[string]interface{}
		wantDiagnostics int
	}{
		{
			name: "valid import array",
			stackContent: map[string]interface{}{
				"import": []interface{}{"catalog/vpc", "catalog/subnet"},
			},
			wantDiagnostics: 0,
		},
		{
			name: "invalid import string",
			stackContent: map[string]interface{}{
				"import": "not-an-array",
			},
			wantDiagnostics: 1,
		},
		{
			name:            "missing import",
			stackContent:    map[string]interface{}{},
			wantDiagnostics: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{}
			diagnostics := handler.validateImportSection(tt.stackContent)
			assert.Len(t, diagnostics, tt.wantDiagnostics)
		})
	}
}

func TestValidateComponentsSection(t *testing.T) {
	tests := []struct {
		name            string
		stackContent    map[string]interface{}
		wantDiagnostics int
	}{
		{
			name: "valid components",
			stackContent: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{},
					},
				},
			},
			wantDiagnostics: 0,
		},
		{
			name: "components as array",
			stackContent: map[string]interface{}{
				"components": []interface{}{"invalid"},
			},
			wantDiagnostics: 1,
		},
		{
			name:            "missing components",
			stackContent:    map[string]interface{}{},
			wantDiagnostics: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{}
			diagnostics := handler.validateComponentsSection(tt.stackContent)
			assert.Len(t, diagnostics, tt.wantDiagnostics)
		})
	}
}

func TestValidateVarsSection(t *testing.T) {
	tests := []struct {
		name            string
		stackContent    map[string]interface{}
		wantDiagnostics int
	}{
		{
			name: "valid vars",
			stackContent: map[string]interface{}{
				"vars": map[string]interface{}{
					"region": "us-east-1",
				},
			},
			wantDiagnostics: 0,
		},
		{
			name: "vars as array",
			stackContent: map[string]interface{}{
				"vars": []interface{}{"invalid"},
			},
			wantDiagnostics: 1,
		},
		{
			name:            "missing vars",
			stackContent:    map[string]interface{}{},
			wantDiagnostics: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{}
			diagnostics := handler.validateVarsSection(tt.stackContent)
			assert.Len(t, diagnostics, tt.wantDiagnostics)
		})
	}
}
