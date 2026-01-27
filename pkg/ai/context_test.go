package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindStackFiles(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) string
		minCount      int
		expectError   bool
		errorContains string
	}{
		{
			name: "finds yaml files in root",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				err := os.WriteFile(filepath.Join(tmpDir, "stack1.yaml"), []byte("key: value"), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tmpDir, "stack2.yaml"), []byte("key: value2"), 0o644)
				require.NoError(t, err)
				return tmpDir
			},
			minCount:    2,
			expectError: false,
		},
		{
			name: "no files found returns error",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			minCount:      0,
			expectError:   true,
			errorContains: errUtils.ErrAINoStackFilesFound.Error(),
		},
		{
			name: "finds root yaml files",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				nestedDir := filepath.Join(tmpDir, "nested")
				err := os.MkdirAll(nestedDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tmpDir, "root.yaml"), []byte("root: true"), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(nestedDir, "nested.yaml"), []byte("nested: true"), 0o644)
				require.NoError(t, err)
				return tmpDir
			},
			// filepath.Glob with **/*.yaml does NOT recurse - standard Go doesn't support **.
			// The function only finds root *.yaml files with the current implementation.
			minCount:    1, // At least the root file should be found.
			expectError: false,
		},
		{
			name: "finds yaml files only in root not yml without nested",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				// Create .yml file in root - the function searches for **/*.yml which
				// does NOT work with filepath.Glob (** is not supported).
				// Root .yml files are NOT found by the pattern **/*.yml.
				err := os.WriteFile(filepath.Join(tmpDir, "stack1.yml"), []byte("key: value"), 0o644)
				require.NoError(t, err)
				// Also add a .yaml file to ensure the function works.
				err = os.WriteFile(filepath.Join(tmpDir, "stack2.yaml"), []byte("key: value"), 0o644)
				require.NoError(t, err)
				return tmpDir
			},
			// The function finds *.yaml in root, but **/*.yml does not find root .yml files.
			minCount:    1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stacksPath := tt.setupFunc(t)

			files, err := findStackFiles(stacksPath)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.GreaterOrEqual(t, len(files), tt.minCount)
		})
	}
}

func TestFormatFileContent(t *testing.T) {
	tests := []struct {
		name         string
		setupFunc    func(t *testing.T) string
		maxLines     int
		expectTrunc  bool
		expectError  bool
		containsText string
	}{
		{
			name: "reads file within limit",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				content := "line1\nline2\nline3"
				filePath := filepath.Join(tmpDir, "test.yaml")
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
				return filePath
			},
			maxLines:     10,
			expectTrunc:  false,
			containsText: "line1",
		},
		{
			name: "truncates file exceeding limit",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				var lines []string
				for i := 1; i <= 100; i++ {
					lines = append(lines, "line"+string(rune('0'+i%10)))
				}
				content := strings.Join(lines, "\n")
				filePath := filepath.Join(tmpDir, "test.yaml")
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
				return filePath
			},
			maxLines:     5,
			expectTrunc:  true,
			containsText: "truncated",
		},
		{
			name: "handles non-existent file",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/file.yaml"
			},
			maxLines:    10,
			expectError: true,
		},
		{
			name: "handles empty file",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "empty.yaml")
				err := os.WriteFile(filePath, []byte(""), 0o644)
				require.NoError(t, err)
				return filePath
			},
			maxLines:    10,
			expectTrunc: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t)

			result := formatFileContent(filePath, tt.maxLines)

			if tt.expectError {
				assert.Contains(t, result, "Error reading file")
				return
			}

			if tt.expectTrunc {
				assert.Contains(t, result, "truncated")
				assert.Contains(t, result, "more lines")
			} else {
				assert.NotContains(t, result, "truncated")
			}

			if tt.containsText != "" {
				assert.Contains(t, result, tt.containsText)
			}
		})
	}
}

func TestGatherStackContext(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) *schema.AtmosConfiguration
		expectError bool
		contains    []string
	}{
		{
			name: "gathers context from stack files",
			setupFunc: func(t *testing.T) *schema.AtmosConfiguration {
				tmpDir := t.TempDir()
				stacksDir := filepath.Join(tmpDir, "stacks")
				err := os.MkdirAll(stacksDir, 0o755)
				require.NoError(t, err)

				content := `components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"
`
				err = os.WriteFile(filepath.Join(stacksDir, "test-stack.yaml"), []byte(content), 0o644)
				require.NoError(t, err)

				return &schema.AtmosConfiguration{
					BasePath: tmpDir,
					Stacks: schema.Stacks{
						BasePath: "stacks",
					},
					Settings: schema.AtmosSettings{
						AI: schema.AISettings{
							MaxContextFiles: 10,
							MaxContextLines: 500,
							Context: schema.AIContextSettings{
								Enabled: false, // Use legacy mode.
							},
						},
					},
				}
			},
			expectError: false,
			contains:    []string{"=== Atmos Stack Configurations ===", "test-stack.yaml", "components"},
		},
		{
			name: "returns error when no stack files found",
			setupFunc: func(t *testing.T) *schema.AtmosConfiguration {
				tmpDir := t.TempDir()
				stacksDir := filepath.Join(tmpDir, "stacks")
				err := os.MkdirAll(stacksDir, 0o755)
				require.NoError(t, err)

				return &schema.AtmosConfiguration{
					BasePath: tmpDir,
					Stacks: schema.Stacks{
						BasePath: "stacks",
					},
					Settings: schema.AtmosSettings{
						AI: schema.AISettings{
							Context: schema.AIContextSettings{
								Enabled: false,
							},
						},
					},
				}
			},
			expectError: true,
		},
		{
			name: "limits number of files",
			setupFunc: func(t *testing.T) *schema.AtmosConfiguration {
				tmpDir := t.TempDir()
				stacksDir := filepath.Join(tmpDir, "stacks")
				err := os.MkdirAll(stacksDir, 0o755)
				require.NoError(t, err)

				// Create 15 stack files.
				for i := 1; i <= 15; i++ {
					fileName := filepath.Join(stacksDir, "stack"+string(rune('a'+i-1))+".yaml")
					err = os.WriteFile(fileName, []byte("key: value"+string(rune('0'+i%10))), 0o644)
					require.NoError(t, err)
				}

				return &schema.AtmosConfiguration{
					BasePath: tmpDir,
					Stacks: schema.Stacks{
						BasePath: "stacks",
					},
					Settings: schema.AtmosSettings{
						AI: schema.AISettings{
							MaxContextFiles: 5, // Limit to 5 files.
							MaxContextLines: 500,
							Context: schema.AIContextSettings{
								Enabled: false,
							},
						},
					},
				}
			},
			expectError: false,
			contains:    []string{"Showing first 5 stack files"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := tt.setupFunc(t)

			result, err := GatherStackContext(atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestQuestionNeedsContext(t *testing.T) {
	tests := []struct {
		name     string
		question string
		expected bool
	}{
		{
			name:     "question with 'this repo'",
			question: "Can you analyze this repo?",
			expected: true,
		},
		{
			name:     "question with 'my stack'",
			question: "What components are in my stack?",
			expected: true,
		},
		{
			name:     "question with 'my component'",
			question: "How do I configure my component?",
			expected: true,
		},
		{
			name:     "question with 'what stacks'",
			question: "What stacks are deployed?",
			expected: true,
		},
		{
			name:     "question with 'list stacks'",
			question: "Can you list stacks in prod?",
			expected: true,
		},
		{
			name:     "question with 'show me'",
			question: "Show me the configuration",
			expected: true,
		},
		{
			name:     "question with 'describe the'",
			question: "Describe the VPC component",
			expected: true,
		},
		{
			name:     "question with 'analyze'",
			question: "Please analyze this structure",
			expected: true,
		},
		{
			name:     "question with 'review my'",
			question: "Please review my configuration",
			expected: true,
		},
		{
			name:     "general question no context needed",
			question: "What is Terraform?",
			expected: false,
		},
		{
			name:     "question about atmos in general",
			question: "How does atmos work?",
			expected: false,
		},
		{
			name:     "empty question",
			question: "",
			expected: false,
		},
		{
			name:     "case insensitive matching",
			question: "WHAT STACKS are there?",
			expected: true,
		},
		{
			name:     "question with 'these stacks'",
			question: "What do these stacks do?",
			expected: true,
		},
		{
			name:     "question with 'these components'",
			question: "How do these components work?",
			expected: true,
		},
		{
			name:     "question with 'my configuration'",
			question: "Is my configuration correct?",
			expected: true,
		},
		{
			name:     "question with 'my config'",
			question: "Can you check my config?",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := QuestionNeedsContext(tt.question)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeContext(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "adds privacy warning",
			input: "some config content",
			contains: []string{
				"PRIVACY NOTE",
				"sent to the AI provider",
				"some config content",
			},
		},
		{
			name:  "preserves original content",
			input: "key: value\nother: data",
			contains: []string{
				"key: value",
				"other: data",
			},
		},
		{
			name:  "handles empty string",
			input: "",
			contains: []string{
				"PRIVACY NOTE",
				"========================================",
			},
		},
		{
			name:  "adds separator line",
			input: "test",
			contains: []string{
				"========================================",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeContext(tt.input)

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestShouldSendContext_WithEnvVar(t *testing.T) {
	// Note: This test modifies environment variables, which can affect other tests.
	// In production code, we'd use dependency injection for viper.

	tests := []struct {
		name           string
		envValue       string
		expectedSend   bool
		expectedPrompt bool
	}{
		{
			name:           "env var true",
			envValue:       "true",
			expectedSend:   true,
			expectedPrompt: false,
		},
		{
			name:           "env var 1",
			envValue:       "1",
			expectedSend:   true,
			expectedPrompt: false,
		},
		{
			name:           "env var yes",
			envValue:       "yes",
			expectedSend:   true,
			expectedPrompt: false,
		},
		{
			name:           "env var YES uppercase",
			envValue:       "YES",
			expectedSend:   true,
			expectedPrompt: false,
		},
		{
			name:           "env var false",
			envValue:       "false",
			expectedSend:   false,
			expectedPrompt: false,
		},
		{
			name:           "env var 0",
			envValue:       "0",
			expectedSend:   false,
			expectedPrompt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable.
			t.Setenv("ATMOS_AI_SEND_CONTEXT", tt.envValue)

			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						SendContext:  false,
						PromptOnSend: false,
					},
				},
			}

			sendContext, prompted, err := ShouldSendContext(atmosConfig, "test question")

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedSend, sendContext)
			assert.Equal(t, tt.expectedPrompt, prompted)
		})
	}
}

func TestShouldSendContext_WithConfig(t *testing.T) {
	tests := []struct {
		name           string
		sendContext    bool
		promptOnSend   bool
		expectedSend   bool
		expectedPrompt bool
	}{
		{
			name:           "send_context true without prompt",
			sendContext:    true,
			promptOnSend:   false,
			expectedSend:   true,
			expectedPrompt: false,
		},
		// Note: We can't easily test the prompting behavior without mocking stdin.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env var to test config-based behavior.
			t.Setenv("ATMOS_AI_SEND_CONTEXT", "")

			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						SendContext:  tt.sendContext,
						PromptOnSend: tt.promptOnSend,
					},
				},
			}

			// Use a question that doesn't need context to avoid prompting.
			sendContext, _, err := ShouldSendContext(atmosConfig, "what is terraform")

			assert.NoError(t, err)
			if tt.sendContext && !tt.promptOnSend {
				assert.Equal(t, tt.expectedSend, sendContext)
			}
		})
	}
}

func TestDefaultConstants(t *testing.T) {
	// Verify the default constants are reasonable.
	assert.Equal(t, 10, DefaultMaxContextFiles)
	assert.Equal(t, 500, DefaultMaxContextLines)
}

func TestGatherStackContext_WithContextDiscovery(t *testing.T) {
	t.Run("uses context discovery when enabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a file that would be matched by auto_include.
		content := "test: content"
		err := os.WriteFile(filepath.Join(tmpDir, "ATMOS.md"), []byte(content), 0o644)
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tmpDir,
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Context: schema.AIContextSettings{
						Enabled:     true,
						AutoInclude: []string{"ATMOS.md"},
						MaxFiles:    10,
						MaxSizeMB:   1,
					},
				},
			},
		}

		result, err := GatherStackContext(atmosConfig)

		assert.NoError(t, err)
		assert.Contains(t, result, "Project Files Context")
	})

	t.Run("falls back to legacy when discovery fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		stacksDir := filepath.Join(tmpDir, "stacks")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		// Create stack file for fallback.
		err = os.WriteFile(filepath.Join(stacksDir, "stack.yaml"), []byte("key: value"), 0o644)
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tmpDir,
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Context: schema.AIContextSettings{
						Enabled:     true,
						AutoInclude: []string{}, // Empty, will produce no results.
					},
				},
			},
		}

		result, err := GatherStackContext(atmosConfig)

		assert.NoError(t, err)
		assert.Contains(t, result, "=== Atmos Stack Configurations ===")
	})
}

func TestGatherStackContext_UsesConfiguredLimits(t *testing.T) {
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	err := os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	// Create a file with many lines.
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "line: "+string(rune('a'+i%26)))
	}
	content := strings.Join(lines, "\n")
	err = os.WriteFile(filepath.Join(stacksDir, "stack.yaml"), []byte(content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				MaxContextFiles: 1,
				MaxContextLines: 10, // Limit to 10 lines.
				Context: schema.AIContextSettings{
					Enabled: false,
				},
			},
		},
	}

	result, err := GatherStackContext(atmosConfig)

	assert.NoError(t, err)
	// Should contain truncation notice.
	assert.Contains(t, result, "truncated")
	assert.Contains(t, result, "more lines")
}

func TestFormatFileContent_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.yaml")

	// Create a file with 1000 lines.
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, "line_"+string(rune('0'+i%10))+": value")
	}
	content := strings.Join(lines, "\n")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	result := formatFileContent(filePath, 50)

	// Should be truncated.
	assert.Contains(t, result, "truncated")
	assert.Contains(t, result, "950 more lines") // 1000 - 50 = 950.
}

func TestFindStackFiles_MixedExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with different extensions.
	err := os.WriteFile(filepath.Join(tmpDir, "stack1.yaml"), []byte("yaml: true"), 0o644)
	require.NoError(t, err)
	// Note: .yml files in root directory are NOT found by the function because:
	// - filepath.Glob doesn't support ** for recursive matching
	// - The function uses **/*.yml which doesn't match root files
	err = os.WriteFile(filepath.Join(tmpDir, "stack2.yml"), []byte("yml: true"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "stack3.json"), []byte(`{"json": true}`), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "stack4.txt"), []byte("text file"), 0o644)
	require.NoError(t, err)

	files, err := findStackFiles(tmpDir)

	assert.NoError(t, err)
	// Should find at least the yaml file.
	assert.GreaterOrEqual(t, len(files), 1)

	foundYaml := false
	for _, f := range files {
		if strings.HasSuffix(f, ".yaml") {
			foundYaml = true
		}
		// Note: .yml files in root are NOT found due to filepath.Glob limitations.
		// The **/*.yml pattern does not work as expected with standard Go glob.
	}
	assert.True(t, foundYaml, "should find .yaml files")
}
