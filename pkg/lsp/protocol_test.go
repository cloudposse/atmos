package lsp

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPosition tests the Position type.
func TestPosition(t *testing.T) {
	tests := []struct {
		name     string
		position Position
		wantJSON string
	}{
		{
			name: "zero position",
			position: Position{
				Line:      0,
				Character: 0,
			},
			wantJSON: `{"line":0,"character":0}`,
		},
		{
			name: "typical position",
			position: Position{
				Line:      10,
				Character: 25,
			},
			wantJSON: `{"line":10,"character":25}`,
		},
		{
			name: "large line number",
			position: Position{
				Line:      999999,
				Character: 12345,
			},
			wantJSON: `{"line":999999,"character":12345}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.position)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded Position
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.position, decoded)
		})
	}
}

// TestRange tests the Range type.
func TestRange(t *testing.T) {
	tests := []struct {
		name     string
		r        Range
		wantJSON string
	}{
		{
			name: "single line range",
			r: Range{
				Start: Position{Line: 5, Character: 10},
				End:   Position{Line: 5, Character: 20},
			},
			wantJSON: `{"start":{"line":5,"character":10},"end":{"line":5,"character":20}}`,
		},
		{
			name: "multi-line range",
			r: Range{
				Start: Position{Line: 1, Character: 0},
				End:   Position{Line: 10, Character: 15},
			},
			wantJSON: `{"start":{"line":1,"character":0},"end":{"line":10,"character":15}}`,
		},
		{
			name: "zero range",
			r: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 0},
			},
			wantJSON: `{"start":{"line":0,"character":0},"end":{"line":0,"character":0}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.r)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded Range
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.r, decoded)
		})
	}
}

// TestLocation tests the Location type.
func TestLocation(t *testing.T) {
	tests := []struct {
		name     string
		location Location
		wantJSON string
	}{
		{
			name: "file URI location",
			location: Location{
				URI: "file:///path/to/file.yaml",
				Range: Range{
					Start: Position{Line: 1, Character: 0},
					End:   Position{Line: 1, Character: 10},
				},
			},
			wantJSON: `{"uri":"file:///path/to/file.yaml","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":10}}}`,
		},
		{
			name: "relative URI",
			location: Location{
				URI: "file.txt",
				Range: Range{
					Start: Position{Line: 0, Character: 0},
					End:   Position{Line: 0, Character: 5},
				},
			},
			wantJSON: `{"uri":"file.txt","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.location)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded Location
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.location, decoded)
		})
	}
}

// TestDiagnosticSeverity tests the DiagnosticSeverity type and its constants.
func TestDiagnosticSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity DiagnosticSeverity
		wantInt  int
	}{
		{
			name:     "error severity",
			severity: DiagnosticSeverityError,
			wantInt:  1,
		},
		{
			name:     "warning severity",
			severity: DiagnosticSeverityWarning,
			wantInt:  2,
		},
		{
			name:     "information severity",
			severity: DiagnosticSeverityInformation,
			wantInt:  3,
		},
		{
			name:     "hint severity",
			severity: DiagnosticSeverityHint,
			wantInt:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantInt, int(tt.severity))
		})
	}
}

// TestDiagnosticSeverityMarshalText tests the MarshalText method.
func TestDiagnosticSeverityMarshalText(t *testing.T) {
	tests := []struct {
		name     string
		severity DiagnosticSeverity
		want     string
	}{
		{
			name:     "error",
			severity: DiagnosticSeverityError,
			want:     "1",
		},
		{
			name:     "warning",
			severity: DiagnosticSeverityWarning,
			want:     "2",
		},
		{
			name:     "information",
			severity: DiagnosticSeverityInformation,
			want:     "3",
		},
		{
			name:     "hint",
			severity: DiagnosticSeverityHint,
			want:     "4",
		},
		{
			name:     "zero value",
			severity: DiagnosticSeverity(0),
			want:     "0",
		},
		{
			name:     "negative value",
			severity: DiagnosticSeverity(-1),
			want:     "-1",
		},
		{
			name:     "large value",
			severity: DiagnosticSeverity(999),
			want:     "999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.severity.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(data))
		})
	}
}

// TestDiagnosticSeverityUnmarshalText tests the UnmarshalText method.
func TestDiagnosticSeverityUnmarshalText(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      DiagnosticSeverity
		wantError bool
	}{
		{
			name:      "error",
			input:     "1",
			want:      DiagnosticSeverityError,
			wantError: false,
		},
		{
			name:      "warning",
			input:     "2",
			want:      DiagnosticSeverityWarning,
			wantError: false,
		},
		{
			name:      "information",
			input:     "3",
			want:      DiagnosticSeverityInformation,
			wantError: false,
		},
		{
			name:      "hint",
			input:     "4",
			want:      DiagnosticSeverityHint,
			wantError: false,
		},
		{
			name:      "with whitespace",
			input:     "  2  ",
			want:      DiagnosticSeverityWarning,
			wantError: false,
		},
		{
			name:      "with leading whitespace",
			input:     "   3",
			want:      DiagnosticSeverityInformation,
			wantError: false,
		},
		{
			name:      "with trailing whitespace",
			input:     "1   ",
			want:      DiagnosticSeverityError,
			wantError: false,
		},
		{
			name:      "zero value",
			input:     "0",
			want:      DiagnosticSeverity(0),
			wantError: false,
		},
		{
			name:      "invalid non-numeric",
			input:     "invalid",
			want:      DiagnosticSeverity(0),
			wantError: true,
		},
		{
			name:      "empty string",
			input:     "",
			want:      DiagnosticSeverity(0),
			wantError: true,
		},
		{
			name:      "mixed content",
			input:     "1abc",
			want:      DiagnosticSeverity(0),
			wantError: true,
		},
		{
			name:      "float value",
			input:     "1.5",
			want:      DiagnosticSeverity(0),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var severity DiagnosticSeverity
			err := severity.UnmarshalText([]byte(tt.input))

			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, severity)
			}
		})
	}
}

// TestDiagnosticSeverityRoundTrip tests marshaling and unmarshaling together.
func TestDiagnosticSeverityRoundTrip(t *testing.T) {
	severities := []DiagnosticSeverity{
		DiagnosticSeverityError,
		DiagnosticSeverityWarning,
		DiagnosticSeverityInformation,
		DiagnosticSeverityHint,
		DiagnosticSeverity(0),
		DiagnosticSeverity(100),
	}

	for _, original := range severities {
		t.Run(strconv.Itoa(int(original)), func(t *testing.T) {
			// Marshal.
			data, err := original.MarshalText()
			require.NoError(t, err)

			// Unmarshal.
			var decoded DiagnosticSeverity
			err = decoded.UnmarshalText(data)
			require.NoError(t, err)

			// Should match original.
			assert.Equal(t, original, decoded)
		})
	}
}

// TestDiagnostic tests the Diagnostic type.
func TestDiagnostic(t *testing.T) {
	tests := []struct {
		name       string
		diagnostic Diagnostic
		wantJSON   string
	}{
		{
			name: "basic error",
			diagnostic: Diagnostic{
				Range: Range{
					Start: Position{Line: 1, Character: 0},
					End:   Position{Line: 1, Character: 10},
				},
				Severity: DiagnosticSeverityError,
				Message:  "Syntax error",
			},
			wantJSON: `{
				"range": {"start": {"line": 1, "character": 0}, "end": {"line": 1, "character": 10}},
				"severity": "1",
				"message": "Syntax error",
				"relatedInformation": null
			}`,
		},
		{
			name: "diagnostic with code string",
			diagnostic: Diagnostic{
				Range: Range{
					Start: Position{Line: 5, Character: 10},
					End:   Position{Line: 5, Character: 20},
				},
				Severity: DiagnosticSeverityWarning,
				Code:     "YAML001",
				Source:   "yaml-language-server",
				Message:  "Deprecated field",
			},
			wantJSON: `{
				"range": {"start": {"line": 5, "character": 10}, "end": {"line": 5, "character": 20}},
				"severity": "2",
				"code": "YAML001",
				"source": "yaml-language-server",
				"message": "Deprecated field",
				"relatedInformation": null
			}`,
		},
		{
			name: "diagnostic with code number",
			diagnostic: Diagnostic{
				Range: Range{
					Start: Position{Line: 0, Character: 0},
					End:   Position{Line: 0, Character: 5},
				},
				Severity: DiagnosticSeverityInformation,
				Code:     42,
				Message:  "Info message",
			},
			wantJSON: `{
				"range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 5}},
				"severity": "3",
				"code": 42,
				"message": "Info message",
				"relatedInformation": null
			}`,
		},
		{
			name: "diagnostic with related information",
			diagnostic: Diagnostic{
				Range: Range{
					Start: Position{Line: 10, Character: 5},
					End:   Position{Line: 10, Character: 15},
				},
				Severity: DiagnosticSeverityHint,
				Message:  "Consider refactoring",
				RelatedInformation: []DiagnosticInfo{
					{
						Location: Location{
							URI: "file:///path/to/file.yaml",
							Range: Range{
								Start: Position{Line: 1, Character: 0},
								End:   Position{Line: 1, Character: 10},
							},
						},
						Message: "Related definition here",
					},
				},
			},
			wantJSON: `{
				"range": {"start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 15}},
				"severity": "4",
				"message": "Consider refactoring",
				"relatedInformation": [
					{
						"location": {
							"uri": "file:///path/to/file.yaml",
							"range": {"start": {"line": 1, "character": 0}, "end": {"line": 1, "character": 10}}
						},
						"message": "Related definition here"
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.diagnostic)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling and round-trip.
			var decoded Diagnostic
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			// Re-marshal and compare JSON to handle nil vs empty slice differences.
			decodedData, err := json.Marshal(decoded)
			require.NoError(t, err)
			assert.JSONEq(t, string(data), string(decodedData))
		})
	}
}

// TestDiagnosticInfo tests the DiagnosticInfo type.
func TestDiagnosticInfo(t *testing.T) {
	tests := []struct {
		name     string
		info     DiagnosticInfo
		wantJSON string
	}{
		{
			name: "basic info",
			info: DiagnosticInfo{
				Location: Location{
					URI: "file:///test.yaml",
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: 0, Character: 10},
					},
				},
				Message: "See also",
			},
			wantJSON: `{
				"location": {
					"uri": "file:///test.yaml",
					"range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 10}}
				},
				"message": "See also"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.info)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded DiagnosticInfo
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.info, decoded)
		})
	}
}

// TestPublishDiagnosticsParams tests the PublishDiagnosticsParams type.
func TestPublishDiagnosticsParams(t *testing.T) {
	tests := []struct {
		name     string
		params   PublishDiagnosticsParams
		wantJSON string
	}{
		{
			name: "empty diagnostics",
			params: PublishDiagnosticsParams{
				URI:         "file:///test.yaml",
				Diagnostics: []Diagnostic{},
			},
			wantJSON: `{
				"uri": "file:///test.yaml",
				"diagnostics": []
			}`,
		},
		{
			name: "with diagnostics",
			params: PublishDiagnosticsParams{
				URI: "file:///test.yaml",
				Diagnostics: []Diagnostic{
					{
						Range: Range{
							Start: Position{Line: 1, Character: 0},
							End:   Position{Line: 1, Character: 10},
						},
						Severity: DiagnosticSeverityError,
						Message:  "Error 1",
					},
					{
						Range: Range{
							Start: Position{Line: 2, Character: 5},
							End:   Position{Line: 2, Character: 15},
						},
						Severity: DiagnosticSeverityWarning,
						Message:  "Warning 1",
					},
				},
			},
			wantJSON: `{
				"uri": "file:///test.yaml",
				"diagnostics": [
					{
						"range": {"start": {"line": 1, "character": 0}, "end": {"line": 1, "character": 10}},
						"severity": "1",
						"message": "Error 1",
						"relatedInformation": null
					},
					{
						"range": {"start": {"line": 2, "character": 5}, "end": {"line": 2, "character": 15}},
						"severity": "2",
						"message": "Warning 1",
						"relatedInformation": null
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded PublishDiagnosticsParams
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.params.URI, decoded.URI)
			assert.Len(t, decoded.Diagnostics, len(tt.params.Diagnostics))
			for i := range tt.params.Diagnostics {
				assert.Equal(t, tt.params.Diagnostics[i].Range, decoded.Diagnostics[i].Range)
				assert.Equal(t, tt.params.Diagnostics[i].Severity, decoded.Diagnostics[i].Severity)
				assert.Equal(t, tt.params.Diagnostics[i].Message, decoded.Diagnostics[i].Message)
			}
		})
	}
}

// TestTextDocumentItem tests the TextDocumentItem type.
func TestTextDocumentItem(t *testing.T) {
	tests := []struct {
		name     string
		item     TextDocumentItem
		wantJSON string
	}{
		{
			name: "yaml document",
			item: TextDocumentItem{
				URI:        "file:///test.yaml",
				LanguageID: "yaml",
				Version:    1,
				Text:       "key: value\n",
			},
			wantJSON: `{
				"uri": "file:///test.yaml",
				"languageId": "yaml",
				"version": 1,
				"text": "key: value\n"
			}`,
		},
		{
			name: "empty document",
			item: TextDocumentItem{
				URI:        "file:///empty.txt",
				LanguageID: "plaintext",
				Version:    0,
				Text:       "",
			},
			wantJSON: `{
				"uri": "file:///empty.txt",
				"languageId": "plaintext",
				"version": 0,
				"text": ""
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.item)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded TextDocumentItem
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.item, decoded)
		})
	}
}

// TestTextDocumentIdentifier tests the TextDocumentIdentifier type.
func TestTextDocumentIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		id       TextDocumentIdentifier
		wantJSON string
	}{
		{
			name: "basic identifier",
			id: TextDocumentIdentifier{
				URI: "file:///test.yaml",
			},
			wantJSON: `{"uri": "file:///test.yaml"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.id)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded TextDocumentIdentifier
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.id, decoded)
		})
	}
}

// TestVersionedTextDocumentIdentifier tests the VersionedTextDocumentIdentifier type.
func TestVersionedTextDocumentIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		id       VersionedTextDocumentIdentifier
		wantJSON string
	}{
		{
			name: "versioned identifier",
			id: VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: TextDocumentIdentifier{
					URI: "file:///test.yaml",
				},
				Version: 5,
			},
			wantJSON: `{
				"uri": "file:///test.yaml",
				"version": 5
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.id)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded VersionedTextDocumentIdentifier
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.id, decoded)
		})
	}
}

// TestDidOpenTextDocumentParams tests the DidOpenTextDocumentParams type.
func TestDidOpenTextDocumentParams(t *testing.T) {
	tests := []struct {
		name     string
		params   DidOpenTextDocumentParams
		wantJSON string
	}{
		{
			name: "open document",
			params: DidOpenTextDocumentParams{
				TextDocument: TextDocumentItem{
					URI:        "file:///test.yaml",
					LanguageID: "yaml",
					Version:    1,
					Text:       "content",
				},
			},
			wantJSON: `{
				"textDocument": {
					"uri": "file:///test.yaml",
					"languageId": "yaml",
					"version": 1,
					"text": "content"
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded DidOpenTextDocumentParams
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.params, decoded)
		})
	}
}

// TestDidChangeTextDocumentParams tests the DidChangeTextDocumentParams type.
func TestDidChangeTextDocumentParams(t *testing.T) {
	tests := []struct {
		name     string
		params   DidChangeTextDocumentParams
		wantJSON string
	}{
		{
			name: "full content change",
			params: DidChangeTextDocumentParams{
				TextDocument: VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: TextDocumentIdentifier{
						URI: "file:///test.yaml",
					},
					Version: 2,
				},
				ContentChanges: []TextDocumentContentChangeEvent{
					{
						Text: "new content",
					},
				},
			},
			wantJSON: `{
				"textDocument": {
					"uri": "file:///test.yaml",
					"version": 2
				},
				"contentChanges": [
					{
						"text": "new content"
					}
				]
			}`,
		},
		{
			name: "incremental change",
			params: DidChangeTextDocumentParams{
				TextDocument: VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: TextDocumentIdentifier{
						URI: "file:///test.yaml",
					},
					Version: 3,
				},
				ContentChanges: []TextDocumentContentChangeEvent{
					{
						Range: &Range{
							Start: Position{Line: 1, Character: 0},
							End:   Position{Line: 1, Character: 5},
						},
						RangeLength: 5,
						Text:        "updated",
					},
				},
			},
			wantJSON: `{
				"textDocument": {
					"uri": "file:///test.yaml",
					"version": 3
				},
				"contentChanges": [
					{
						"range": {"start": {"line": 1, "character": 0}, "end": {"line": 1, "character": 5}},
						"rangeLength": 5,
						"text": "updated"
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded DidChangeTextDocumentParams
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.params, decoded)
		})
	}
}

// TestTextDocumentContentChangeEvent tests the TextDocumentContentChangeEvent type.
func TestTextDocumentContentChangeEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    TextDocumentContentChangeEvent
		wantJSON string
	}{
		{
			name: "full document change",
			event: TextDocumentContentChangeEvent{
				Text: "entire new content",
			},
			wantJSON: `{"text": "entire new content"}`,
		},
		{
			name: "incremental change with range",
			event: TextDocumentContentChangeEvent{
				Range: &Range{
					Start: Position{Line: 5, Character: 10},
					End:   Position{Line: 5, Character: 20},
				},
				RangeLength: 10,
				Text:        "replacement",
			},
			wantJSON: `{
				"range": {"start": {"line": 5, "character": 10}, "end": {"line": 5, "character": 20}},
				"rangeLength": 10,
				"text": "replacement"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.event)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded TextDocumentContentChangeEvent
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.event, decoded)
		})
	}
}

// TestDidCloseTextDocumentParams tests the DidCloseTextDocumentParams type.
func TestDidCloseTextDocumentParams(t *testing.T) {
	tests := []struct {
		name     string
		params   DidCloseTextDocumentParams
		wantJSON string
	}{
		{
			name: "close document",
			params: DidCloseTextDocumentParams{
				TextDocument: TextDocumentIdentifier{
					URI: "file:///test.yaml",
				},
			},
			wantJSON: `{
				"textDocument": {
					"uri": "file:///test.yaml"
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded DidCloseTextDocumentParams
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.params, decoded)
		})
	}
}

// TestInitializeParams tests the InitializeParams type.
func TestInitializeParams(t *testing.T) {
	tests := []struct {
		name     string
		params   InitializeParams
		wantJSON string
	}{
		{
			name: "basic initialization",
			params: InitializeParams{
				ProcessID: 1234,
				RootURI:   "file:///workspace",
				Capabilities: ClientCapabilities{
					TextDocument: TextDocumentClientCapabilities{
						PublishDiagnostics: PublishDiagnosticsCapabilities{
							RelatedInformation: true,
						},
					},
				},
			},
			wantJSON: `{
				"processId": 1234,
				"rootUri": "file:///workspace",
				"initializationOptions": null,
				"capabilities": {
					"textDocument": {
						"publishDiagnostics": {
							"relatedInformation": true
						}
					}
				}
			}`,
		},
		{
			name: "with initialization options",
			params: InitializeParams{
				ProcessID: 5678,
				RootURI:   "file:///project",
				InitializationOptions: map[string]interface{}{
					"validate": true,
					"format":   map[string]interface{}{"enable": true},
				},
				Capabilities: ClientCapabilities{
					TextDocument: TextDocumentClientCapabilities{
						PublishDiagnostics: PublishDiagnosticsCapabilities{
							RelatedInformation: false,
						},
					},
				},
			},
			wantJSON: `{
				"processId": 5678,
				"rootUri": "file:///project",
				"initializationOptions": {
					"validate": true,
					"format": {"enable": true}
				},
				"capabilities": {
					"textDocument": {
						"publishDiagnostics": {
							"relatedInformation": false
						}
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded InitializeParams
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			// Use JSONEq for comparison due to interface{} in InitializationOptions.
			decodedJSON, _ := json.Marshal(decoded)
			assert.JSONEq(t, tt.wantJSON, string(decodedJSON))
		})
	}
}

// TestClientCapabilities tests the ClientCapabilities type.
func TestClientCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		capabilities ClientCapabilities
		wantJSON     string
	}{
		{
			name: "with diagnostics support",
			capabilities: ClientCapabilities{
				TextDocument: TextDocumentClientCapabilities{
					PublishDiagnostics: PublishDiagnosticsCapabilities{
						RelatedInformation: true,
					},
				},
			},
			wantJSON: `{
				"textDocument": {
					"publishDiagnostics": {
						"relatedInformation": true
					}
				}
			}`,
		},
		{
			name: "without diagnostics support",
			capabilities: ClientCapabilities{
				TextDocument: TextDocumentClientCapabilities{
					PublishDiagnostics: PublishDiagnosticsCapabilities{
						RelatedInformation: false,
					},
				},
			},
			wantJSON: `{
				"textDocument": {
					"publishDiagnostics": {
						"relatedInformation": false
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.capabilities)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded ClientCapabilities
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.capabilities, decoded)
		})
	}
}

// TestInitializeResult tests the InitializeResult type.
func TestInitializeResult(t *testing.T) {
	tests := []struct {
		name     string
		result   InitializeResult
		wantJSON string
	}{
		{
			name: "with text document sync",
			result: InitializeResult{
				Capabilities: ServerCapabilities{
					TextDocumentSync: 1,
				},
			},
			wantJSON: `{
				"capabilities": {
					"textDocumentSync": 1
				}
			}`,
		},
		{
			name: "with complex sync options",
			result: InitializeResult{
				Capabilities: ServerCapabilities{
					TextDocumentSync: map[string]interface{}{
						"openClose": true,
						"change":    2,
					},
				},
			},
			wantJSON: `{
				"capabilities": {
					"textDocumentSync": {
						"openClose": true,
						"change": 2
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.result)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded InitializeResult
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			// Use JSONEq for comparison due to interface{} in TextDocumentSync.
			decodedJSON, _ := json.Marshal(decoded)
			assert.JSONEq(t, tt.wantJSON, string(decodedJSON))
		})
	}
}

// TestServerCapabilities tests the ServerCapabilities type.
func TestServerCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		capabilities ServerCapabilities
		wantJSON     string
	}{
		{
			name: "simple sync kind",
			capabilities: ServerCapabilities{
				TextDocumentSync: 2,
			},
			wantJSON: `{
				"textDocumentSync": 2
			}`,
		},
		{
			name: "detailed sync options",
			capabilities: ServerCapabilities{
				TextDocumentSync: map[string]interface{}{
					"openClose":         true,
					"change":            1,
					"willSave":          false,
					"willSaveWaitUntil": false,
					"save": map[string]interface{}{
						"includeText": true,
					},
				},
			},
			wantJSON: `{
				"textDocumentSync": {
					"openClose": true,
					"change": 1,
					"willSave": false,
					"willSaveWaitUntil": false,
					"save": {
						"includeText": true
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling.
			data, err := json.Marshal(tt.capabilities)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test JSON unmarshaling.
			var decoded ServerCapabilities
			err = json.Unmarshal([]byte(tt.wantJSON), &decoded)
			require.NoError(t, err)
			// Use JSONEq for comparison due to interface{} in TextDocumentSync.
			decodedJSON, _ := json.Marshal(decoded)
			assert.JSONEq(t, tt.wantJSON, string(decodedJSON))
		})
	}
}

// TestDiagnosticSeverityEdgeCases tests edge cases for DiagnosticSeverity.
func TestDiagnosticSeverityEdgeCases(t *testing.T) {
	t.Run("very large value", func(t *testing.T) {
		severity := DiagnosticSeverity(1000000)
		data, err := severity.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, "1000000", string(data))

		var decoded DiagnosticSeverity
		err = decoded.UnmarshalText(data)
		require.NoError(t, err)
		assert.Equal(t, severity, decoded)
	})

	t.Run("negative value", func(t *testing.T) {
		severity := DiagnosticSeverity(-100)
		data, err := severity.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, "-100", string(data))

		var decoded DiagnosticSeverity
		err = decoded.UnmarshalText(data)
		require.NoError(t, err)
		assert.Equal(t, severity, decoded)
	})

	t.Run("unmarshal with tabs", func(t *testing.T) {
		var severity DiagnosticSeverity
		err := severity.UnmarshalText([]byte("\t3\t"))
		require.NoError(t, err)
		assert.Equal(t, DiagnosticSeverityInformation, severity)
	})

	t.Run("unmarshal with mixed whitespace", func(t *testing.T) {
		var severity DiagnosticSeverity
		err := severity.UnmarshalText([]byte(" \t 2 \t "))
		require.NoError(t, err)
		assert.Equal(t, DiagnosticSeverityWarning, severity)
	})
}

// TestComplexDiagnosticScenarios tests complex real-world scenarios.
func TestComplexDiagnosticScenarios(t *testing.T) {
	t.Run("multiple diagnostics with related information", func(t *testing.T) {
		params := PublishDiagnosticsParams{
			URI: "file:///atmos.yaml",
			Diagnostics: []Diagnostic{
				{
					Range: Range{
						Start: Position{Line: 10, Character: 2},
						End:   Position{Line: 10, Character: 15},
					},
					Severity: DiagnosticSeverityError,
					Code:     "duplicate-key",
					Source:   "atmos-lsp",
					Message:  "Duplicate key 'components'",
					RelatedInformation: []DiagnosticInfo{
						{
							Location: Location{
								URI: "file:///atmos.yaml",
								Range: Range{
									Start: Position{Line: 5, Character: 2},
									End:   Position{Line: 5, Character: 15},
								},
							},
							Message: "First declaration here",
						},
					},
				},
				{
					Range: Range{
						Start: Position{Line: 20, Character: 4},
						End:   Position{Line: 20, Character: 10},
					},
					Severity: DiagnosticSeverityWarning,
					Code:     1001,
					Source:   "atmos-lsp",
					Message:  "Deprecated syntax",
				},
			},
		}

		// Test marshaling.
		data, err := json.Marshal(params)
		require.NoError(t, err)

		// Test unmarshaling.
		var decoded PublishDiagnosticsParams
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		// Verify content.
		assert.Equal(t, params.URI, decoded.URI)
		assert.Len(t, decoded.Diagnostics, 2)
		assert.Equal(t, "duplicate-key", decoded.Diagnostics[0].Code)
		assert.Equal(t, 1001, int(decoded.Diagnostics[1].Code.(float64)))
	})

	t.Run("empty related information array", func(t *testing.T) {
		diagnostic := Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 1},
			},
			Severity:           DiagnosticSeverityError,
			Message:            "Test",
			RelatedInformation: []DiagnosticInfo{},
		}

		data, err := json.Marshal(diagnostic)
		require.NoError(t, err)

		var decoded Diagnostic
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.NotNil(t, decoded.RelatedInformation)
		assert.Len(t, decoded.RelatedInformation, 0)
	})
}

// TestJSONOmitEmpty tests that omitempty works correctly.
func TestJSONOmitEmpty(t *testing.T) {
	t.Run("diagnostic without optional fields", func(t *testing.T) {
		diagnostic := Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 1},
			},
			Severity: DiagnosticSeverityError,
			Message:  "Error message",
		}

		data, err := json.Marshal(diagnostic)
		require.NoError(t, err)

		// Code and Source should be omitted.
		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)

		_, hasCode := raw["code"]
		assert.False(t, hasCode, "code field should be omitted")

		_, hasSource := raw["source"]
		assert.False(t, hasSource, "source field should be omitted")
	})

	t.Run("content change without range", func(t *testing.T) {
		event := TextDocumentContentChangeEvent{
			Text: "full content",
		}

		data, err := json.Marshal(event)
		require.NoError(t, err)

		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)

		_, hasRange := raw["range"]
		assert.False(t, hasRange, "range field should be omitted")

		_, hasRangeLength := raw["rangeLength"]
		assert.False(t, hasRangeLength, "rangeLength field should be omitted")
	})
}
