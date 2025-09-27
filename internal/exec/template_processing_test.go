package exec

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessTemplateWithoutContext(t *testing.T) {
	testCases := []struct {
		name        string
		template    string
		expectError bool
		validate    func(t *testing.T, result string)
	}{
		{
			name:     "current timestamp",
			template: "timestamp: {{ now | date \"2006-01-02\" }}",
			validate: func(t *testing.T, result string) {
				expectedDate := time.Now().Format("2006-01-02")
				assert.Contains(t, result, "timestamp: "+expectedDate)
			},
		},
		{
			name:     "environment variable",
			template: "user: {{ env \"USER\" }}",
			validate: func(t *testing.T, result string) {
				expectedUser := os.Getenv("USER")
				if expectedUser != "" {
					assert.Contains(t, result, "user: "+expectedUser)
				} else {
					// On systems without USER env var
					assert.Contains(t, result, "user:")
				}
			},
		},
		{
			name:     "math operations",
			template: "result: {{ add 1 2 }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "result: 3")
			},
		},
		{
			name:     "string operations",
			template: "upper: {{ \"hello\" | upper }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "upper: HELLO")
			},
		},
		{
			name: "multiple operations without context",
			template: `
config:
  version: "1.0.0"
  timestamp: {{ now | date "2006-01-02T15:04:05Z07:00" }}
  random: {{ randAlphaNum 5 | upper }}
  calculated: {{ mul 5 10 }}`,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "version: \"1.0.0\"")
				assert.Contains(t, result, "timestamp:")
				assert.Contains(t, result, "random:")
				assert.Contains(t, result, "calculated: 50")
			},
		},
		{
			name:     "static content only",
			template: "static: value",
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "static: value", result)
			},
		},
		{
			name: "complex template without context",
			template: `
metadata:
  created_at: {{ now | date "2006-01-02" }}
  environment: {{ env "ENVIRONMENT" | default "development" }}
settings:
  debug: {{ env "DEBUG" | default "false" }}
  port: {{ env "PORT" | default "8080" }}`,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "created_at:")
				assert.Contains(t, result, "environment:")
				assert.Contains(t, result, "debug:")
				assert.Contains(t, result, "port:")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test with nil context
			result, err := ProcessTmpl("test.yaml.tmpl", tc.template, nil, false)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tc.validate(t, result)
			}

			// Test with empty context
			emptyContext := map[string]any{}
			result2, err2 := ProcessTmpl("test.yaml.tmpl", tc.template, emptyContext, false)
			if tc.expectError {
				require.Error(t, err2)
			} else {
				require.NoError(t, err2)
				tc.validate(t, result2)
			}
		})
	}
}

func TestProcessTemplateMixedContext(t *testing.T) {
	context := map[string]any{
		"name":    "test-app",
		"version": "2.0.0",
		"team":    "platform",
	}

	template := `
app: {{ .name }}
version: {{ .version }}
team: {{ .team }}
timestamp: {{ now | date "2006-01-02" }}
build_number: {{ env "BUILD_NUMBER" | default "local" }}
calculated: {{ add 10 20 }}`

	result, err := ProcessTmpl("test.yaml.tmpl", template, context, false)
	require.NoError(t, err)

	assert.Contains(t, result, "app: test-app")
	assert.Contains(t, result, "version: 2.0.0")
	assert.Contains(t, result, "team: platform")
	assert.Contains(t, result, "timestamp:")
	assert.Contains(t, result, "build_number:")
	assert.Contains(t, result, "calculated: 30")
}

func TestProcessTemplateWithMissingContext(t *testing.T) {
	template := `
app: {{ .name }}
timestamp: {{ now | date "2006-01-02" }}`

	// With ignoreMissingTemplateValues = false (should error)
	_, err := ProcessTmpl("test.yaml.tmpl", template, nil, false)
	assert.Error(t, err)
	// The error message can vary between Go versions
	assert.True(t, strings.Contains(err.Error(), "no entry for key") ||
		strings.Contains(err.Error(), "nil data"))

	// With ignoreMissingTemplateValues = true (should not error)
	result, err := ProcessTmpl("test.yaml.tmpl", template, nil, true)
	assert.NoError(t, err)
	assert.Contains(t, result, "app: <no value>")
	assert.Contains(t, result, "timestamp:")
}

func TestProcessTemplateErrorCases(t *testing.T) {
	testCases := []struct {
		name     string
		template string
		context  map[string]any
	}{
		{
			name:     "invalid template syntax",
			template: "value: {{ invalid syntax }}",
		},
		{
			name:     "unclosed template delimiter",
			template: "value: {{ now",
		},
		{
			name:     "invalid function",
			template: "value: {{ nonexistentfunc }}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ProcessTmpl("test.yaml.tmpl", tc.template, tc.context, false)
			assert.Error(t, err)
		})
	}
}

// TestProcessTemplateWithSprigFunctions tests various Sprig template functions work without context.
func TestProcessTemplateWithSprigFunctions(t *testing.T) {
	testCases := []struct {
		name     string
		template string
		validate func(t *testing.T, result string)
	}{
		{
			name:     "uuid generation",
			template: "id: {{ uuidv4 }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "id:")
				// UUID v4 format: 8-4-4-4-12 hex digits
				parts := strings.Split(result, ": ")
				if len(parts) == 2 {
					uuid := strings.TrimSpace(parts[1])
					assert.Len(t, uuid, 36) // UUID length with dashes
				}
			},
		},
		{
			name:     "random string",
			template: "token: {{ randAlphaNum 10 }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "token:")
				parts := strings.Split(result, ": ")
				if len(parts) == 2 {
					token := strings.TrimSpace(parts[1])
					assert.Len(t, token, 10)
				}
			},
		},
		{
			name:     "base64 encoding",
			template: "encoded: {{ \"hello world\" | b64enc }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "encoded: aGVsbG8gd29ybGQ=")
			},
		},
		{
			name:     "sha256 hash",
			template: "hash: {{ \"test\" | sha256sum }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "hash: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
			},
		},
		{
			name:     "date formatting",
			template: "formatted: {{ now | date \"15:04:05\" }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "formatted:")
				// Should contain time in HH:MM:SS format
				assert.Regexp(t, `formatted: \d{2}:\d{2}:\d{2}`, result)
			},
		},
		{
			name:     "string manipulation chain",
			template: "result: {{ \"hello world\" | upper | replace \" \" \"-\" }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "result: HELLO-WORLD")
			},
		},
		{
			name:     "list operations",
			template: "first: {{ list \"a\" \"b\" \"c\" | first }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "first: a")
			},
		},
		{
			name:     "ternary operator",
			template: "value: {{ true | ternary \"yes\" \"no\" }}",
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "value: yes")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ProcessTmpl("test.yaml.tmpl", tc.template, nil, false)
			require.NoError(t, err)
			tc.validate(t, result)
		})
	}
}

// BenchmarkTemplateProcessing benchmarks template processing performance.
func BenchmarkTemplateProcessing(b *testing.B) {
	template := "value: {{ add 1 2 }}"
	context := map[string]any{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ProcessTmpl("bench.yaml.tmpl", template, context, false)
	}
}

func BenchmarkTemplateProcessingComplex(b *testing.B) {
	template := `
config:
  timestamp: {{ now | date "2006-01-02T15:04:05Z07:00" }}
  random: {{ randAlphaNum 10 }}
  calculated: {{ mul 5 10 }}
  encoded: {{ "test" | b64enc }}`
	context := map[string]any{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ProcessTmpl("bench.yaml.tmpl", template, context, false)
	}
}

func BenchmarkTemplateProcessingWithContext(b *testing.B) {
	template := `
app: {{ .name }}
version: {{ .version }}
timestamp: {{ now | date "2006-01-02" }}`
	context := map[string]any{
		"name":    "bench-app",
		"version": "1.0.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ProcessTmpl("bench.yaml.tmpl", template, context, false)
	}
}
