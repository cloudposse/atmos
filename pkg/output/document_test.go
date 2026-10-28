package output

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrintOrWriteToFile(t *testing.T) {
	t.Parallel()

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 4,
			},
		},
	}

	// Test data to write
	testData := map[string]interface{}{
		"test": "data",
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	err := WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: "yaml", File: "", Data: testData})
	assert.NoError(t, err)

	err = WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: "json", File: "", Data: testData})
	assert.NoError(t, err)

	tempDir := t.TempDir()

	yamlFile := filepath.Join(tempDir, "test.yaml")
	err = WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: "yaml", File: yamlFile, Data: testData})
	assert.NoError(t, err)

	_, err = os.Stat(yamlFile)
	assert.NoError(t, err)

	jsonFile := filepath.Join(tempDir, "test.json")
	err = WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: "json", File: jsonFile, Data: testData})
	assert.NoError(t, err)

	_, err = os.Stat(jsonFile)
	assert.NoError(t, err)

	err = WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: "invalid", File: "", Data: testData})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid 'format'")

	// Test with default tab width (when TabWidth is 0)
	atmosConfigDefaultTabWidth := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 0, // Should default to 2
			},
		},
	}
	err = WriteDocument(DocumentRequest{AtmosConfig: atmosConfigDefaultTabWidth, Format: "yaml", File: "", Data: testData})
	assert.NoError(t, err)
}

// TestPrintOrWriteToFile_ScalarResults verifies the exact printed text for query-style results: a top-level string
// scalar is printed raw in YAML (so shell capture such as acct=$(atmos describe component ... --query .vars.acct)
// yields the bare value) and as a proper JSON string in JSON, while numbers, booleans, maps and lists keep their
// normal encoding with ambiguous strings inside structures quoted.
//
// The stdout sub-tests swap os.Stdout to capture output; they must run serially.
func TestPrintOrWriteToFile_ScalarResults(t *testing.T) {
	// Use standard streams even if an earlier test installed a buffered writer.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	tests := []struct {
		name         string
		data         any
		expectedYAML string
		expectedJSON string
	}{
		{"string with leading zero", "068007702576", "\"068007702576\"\n", "\"068007702576\"\n"},
		{"numeric-looking string", "123456789012", "\"123456789012\"\n", "\"123456789012\"\n"},
		{"bool-looking string", "true", "\"true\"\n", "\"true\"\n"},
		{"plain string", "dev", "dev\n", "\"dev\"\n"},
		{"integer", 123456789012, "123456789012\n", "123456789012\n"},
		{"max int64", int64(9223372036854775807), "9223372036854775807\n", "9223372036854775807\n"},
		{"float", 1.5, "1.5\n", "1.5\n"},
		{"bool", true, "true\n", "true\n"},
		{"null", nil, "null\n", "null\n"},
		{
			"map containing ambiguous values",
			map[string]any{"acct": "068007702576", "flag": "true", "count": 42, "ratio": 1.5},
			"acct: \"068007702576\"\ncount: 42\nflag: \"true\"\nratio: 1.5\n",
			"{\n  \"acct\": \"068007702576\",\n  \"count\": 42,\n  \"flag\": \"true\",\n  \"ratio\": 1.5\n}\n",
		},
		{
			"list containing ambiguous values",
			[]any{"007", 7, "false", 1.5},
			"- \"007\"\n- 7\n- \"false\"\n- 1.5\n",
			"[\n  \"007\",\n  7,\n  \"false\",\n  1.5\n]\n",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}

	for _, tt := range tests {
		for _, format := range []string{"yaml", "json"} {
			expected := tt.expectedYAML
			if format == "json" {
				expected = tt.expectedJSON
			}

			t.Run(tt.name+"/"+format+"/file", func(t *testing.T) {
				file := filepath.Join(t.TempDir(), "out."+format)
				require.NoError(t, WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: format, File: file, Data: tt.data}))
				content, err := os.ReadFile(file)
				require.NoError(t, err)
				assert.Equal(t, expected, string(content))
			})

			t.Run(tt.name+"/"+format+"/stdout", func(t *testing.T) {
				var printErr error
				out := captureStdout(t, func() {
					printErr = WriteDocument(DocumentRequest{AtmosConfig: atmosConfig, Format: format, File: "", Data: tt.data})
				})
				require.NoError(t, printErr)
				// Files preserve YAML scalar types; display output prints string results verbatim.
				if format == "yaml" {
					if value, ok := tt.data.(string); ok {
						expected = value + "\n"
					}
					// The YAML printer terminates every result with an extra newline.
					expected += "\n"
				}
				assert.Equal(t, expected, out)
			})
		}
	}
}

// captureStdout captures stdout during the execution of fn and returns the captured output.
// It restores stdout and logger output after the function completes, even if fn panics.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	oldLogOut := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w
	log.SetOutput(w)

	closed := false

	// Ensure stdout and logger are restored even if fn panics.
	defer func() {
		os.Stdout = oldStdout
		log.SetOutput(oldLogOut)
		if !closed {
			_ = w.Close()
			_ = r.Close()
		}
	}()

	fn()

	// Close writer before reading to avoid deadlock.
	_ = w.Close()
	closed = true
	os.Stdout = oldStdout
	log.SetOutput(oldLogOut)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}

	return buf.String()
}
