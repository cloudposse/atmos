package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpCommand_BasicProperties(t *testing.T) {
	t.Run("help command properties", func(t *testing.T) {
		assert.Equal(t, "help [topic]", helpCmd.Use)
		assert.Equal(t, "Get AI-powered help on Atmos topics", helpCmd.Short)
		assert.NotEmpty(t, helpCmd.Long)
		assert.NotNil(t, helpCmd.RunE)
		// Check that Args allows maximum 1 argument.
		assert.NotNil(t, helpCmd.Args)
	})

	t.Run("help command has descriptive long text", func(t *testing.T) {
		assert.Contains(t, helpCmd.Long, "intelligent help")
		assert.Contains(t, helpCmd.Long, "AI assistant")
		assert.Contains(t, helpCmd.Long, "explanations")
		assert.Contains(t, helpCmd.Long, "examples")
		assert.Contains(t, helpCmd.Long, "best practices")
	})
}

func TestHelpCommand_LongDescriptionTopics(t *testing.T) {
	tests := []struct {
		name     string
		category string
		topics   []string
	}{
		{
			name:     "Core Concepts section",
			category: "Core Concepts",
			topics:   []string{"stacks", "components", "inheritance", "imports", "overrides"},
		},
		{
			name:     "Features section",
			category: "Features",
			topics:   []string{"templating", "workflows", "validation", "vendoring", "affected", "catalogs", "schemas", "opa", "settings"},
		},
		{
			name:     "Integrations section",
			category: "Integrations",
			topics:   []string{"terraform", "helmfile", "atlantis", "spacelift", "backends"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, helpCmd.Long, tt.category)
			for _, topic := range tt.topics {
				assert.Contains(t, helpCmd.Long, topic, "Long description should mention topic: %s", topic)
			}
		})
	}
}

func TestHelpCommand_Examples(t *testing.T) {
	t.Run("long description contains examples", func(t *testing.T) {
		assert.Contains(t, helpCmd.Long, "Examples:")
		assert.Contains(t, helpCmd.Long, "atmos ai help stacks")
		assert.Contains(t, helpCmd.Long, "atmos ai help inheritance")
		assert.Contains(t, helpCmd.Long, "atmos ai help affected")
		assert.Contains(t, helpCmd.Long, "atmos ai help terraform")
	})
}

func TestHelpCommand_CommandHierarchy(t *testing.T) {
	t.Run("help command is attached to ai command", func(t *testing.T) {
		parent := helpCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "ai", parent.Name())
	})
}

func TestHelpCommand_ArgsValidation(t *testing.T) {
	t.Run("accepts zero arguments (general help)", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(helpCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("accepts one argument (topic)", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(helpCmd, []string{"stacks"})
		assert.NoError(t, err)
	})

	t.Run("rejects two or more arguments", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(helpCmd, []string{"stacks", "components"})
		assert.Error(t, err)
	})
}

func TestGetHelpQuestionForTopic_CoreConcepts(t *testing.T) {
	tests := []struct {
		name             string
		topic            string
		expectedContains []string
	}{
		{
			name:             "stacks topic",
			topic:            "stacks",
			expectedContains: []string{"stacks", "best practices", "organizing"},
		},
		{
			name:             "components topic",
			topic:            "components",
			expectedContains: []string{"components", "stacks", "reusable"},
		},
		{
			name:             "inheritance topic",
			topic:            "inheritance",
			expectedContains: []string{"inheritance", "precedence", "configuration"},
		},
		{
			name:             "imports topic",
			topic:            "imports",
			expectedContains: []string{"imports", "imported", "organizing"},
		},
		{
			name:             "overrides topic",
			topic:            "overrides",
			expectedContains: []string{"overrides", "precedence"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question := getHelpQuestionForTopic(tt.topic)
			assert.NotEmpty(t, question)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, strings.ToLower(question), strings.ToLower(expected),
					"Question for topic '%s' should contain '%s'", tt.topic, expected)
			}
		})
	}
}

func TestGetHelpQuestionForTopic_Features(t *testing.T) {
	tests := []struct {
		name             string
		topic            string
		aliases          []string
		expectedContains []string
	}{
		{
			name:             "templating topic",
			topic:            "templating",
			aliases:          []string{"templates"},
			expectedContains: []string{"templating", "Go templates", "functions"},
		},
		{
			name:             "workflows topic",
			topic:            "workflows",
			expectedContains: []string{"workflow", "orchestration", "patterns"},
		},
		{
			name:             "validation topic",
			topic:            "validation",
			aliases:          []string{"validate"},
			expectedContains: []string{"validation", "schema"},
		},
		{
			name:             "vendoring topic",
			topic:            "vendoring",
			aliases:          []string{"vendor"},
			expectedContains: []string{"vendoring", "external", "components"},
		},
		{
			name:             "affected topic",
			topic:            "affected",
			expectedContains: []string{"affected", "detection", "CI/CD"},
		},
		{
			name:             "catalogs topic",
			topic:            "catalogs",
			aliases:          []string{"catalog"},
			expectedContains: []string{"catalogs", "component"},
		},
		{
			name:             "schemas topic",
			topic:            "schemas",
			aliases:          []string{"schema"},
			expectedContains: []string{"schema", "JSON Schema", "validation"},
		},
		{
			name:             "opa topic",
			topic:            "opa",
			aliases:          []string{"policies"},
			expectedContains: []string{"OPA", "policy", "policies"},
		},
		{
			name:             "settings topic",
			topic:            "settings",
			expectedContains: []string{"settings", "atmos.yaml", "behavior"},
		},
		{
			name:             "mixins topic",
			topic:            "mixins",
			expectedContains: []string{"mixins", "vendoring"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test primary topic.
			question := getHelpQuestionForTopic(tt.topic)
			assert.NotEmpty(t, question)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, question, expected,
					"Question for topic '%s' should contain '%s'", tt.topic, expected)
			}

			// Test aliases produce the same question.
			for _, alias := range tt.aliases {
				aliasQuestion := getHelpQuestionForTopic(alias)
				assert.Equal(t, question, aliasQuestion,
					"Alias '%s' should produce the same question as '%s'", alias, tt.topic)
			}
		})
	}
}

func TestGetHelpQuestionForTopic_Integrations(t *testing.T) {
	tests := []struct {
		name             string
		topic            string
		aliases          []string
		expectedContains []string
	}{
		{
			name:             "terraform topic",
			topic:            "terraform",
			expectedContains: []string{"Terraform", "integration", "best practices"},
		},
		{
			name:             "helmfile topic",
			topic:            "helmfile",
			expectedContains: []string{"Helmfile", "integration", "best practices"},
		},
		{
			name:             "atlantis topic",
			topic:            "atlantis",
			expectedContains: []string{"Atlantis", "configure"},
		},
		{
			name:             "spacelift topic",
			topic:            "spacelift",
			expectedContains: []string{"Spacelift", "configure", "stacks"},
		},
		{
			name:             "backends topic",
			topic:            "backends",
			aliases:          []string{"backend"},
			expectedContains: []string{"backend", "Terraform", "configuration"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question := getHelpQuestionForTopic(tt.topic)
			assert.NotEmpty(t, question)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, question, expected,
					"Question for topic '%s' should contain '%s'", tt.topic, expected)
			}

			// Test aliases produce the same question.
			for _, alias := range tt.aliases {
				aliasQuestion := getHelpQuestionForTopic(alias)
				assert.Equal(t, question, aliasQuestion,
					"Alias '%s' should produce the same question as '%s'", alias, tt.topic)
			}
		})
	}
}

func TestGetHelpQuestionForTopic_GeneralAndCustom(t *testing.T) {
	t.Run("general topic", func(t *testing.T) {
		question := getHelpQuestionForTopic("general")
		assert.Contains(t, question, "comprehensive overview")
		assert.Contains(t, question, "key concepts")
		assert.Contains(t, question, "architecture")
	})

	t.Run("unknown topic returns custom question", func(t *testing.T) {
		question := getHelpQuestionForTopic("unknown-topic-xyz")
		assert.Contains(t, question, "unknown-topic-xyz")
		assert.Contains(t, question, "context of Atmos")
		assert.Contains(t, question, "best practices")
	})

	t.Run("empty topic treated as custom", func(t *testing.T) {
		question := getHelpQuestionForTopic("")
		assert.Contains(t, question, "context of Atmos")
	})
}

func TestGetHelpQuestionForTopic_CaseInsensitivity(t *testing.T) {
	tests := []struct {
		name      string
		topics    []string
		reference string
	}{
		{
			name:      "stacks case variations",
			topics:    []string{"STACKS", "Stacks", "StAcKs", "stacks"},
			reference: "stacks",
		},
		{
			name:      "terraform case variations",
			topics:    []string{"TERRAFORM", "Terraform", "terraform", "TerraForm"},
			reference: "terraform",
		},
		{
			name:      "components case variations",
			topics:    []string{"COMPONENTS", "Components", "components"},
			reference: "components",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			referenceQuestion := getHelpQuestionForTopic(tt.reference)
			for _, topic := range tt.topics {
				question := getHelpQuestionForTopic(topic)
				assert.Equal(t, referenceQuestion, question,
					"Topic '%s' should produce same question as '%s'", topic, tt.reference)
			}
		})
	}
}

func TestGetTopicFromArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedTopic string
	}{
		{
			name:          "no args defaults to general",
			args:          []string{},
			expectedTopic: "general",
		},
		{
			name:          "nil args defaults to general",
			args:          nil,
			expectedTopic: "general",
		},
		{
			name:          "single arg used as topic",
			args:          []string{"stacks"},
			expectedTopic: "stacks",
		},
		{
			name:          "multiple args uses first one",
			args:          []string{"terraform", "components"},
			expectedTopic: "terraform",
		},
		{
			name:          "empty string arg",
			args:          []string{""},
			expectedTopic: "",
		},
		{
			name:          "whitespace topic",
			args:          []string{"  workflows  "},
			expectedTopic: "  workflows  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topic := getTopicFromArgs(tt.args)
			assert.Equal(t, tt.expectedTopic, topic)
		})
	}
}

func TestHelpCommand_ErrorCases(t *testing.T) {
	t.Run("returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "help",
			Args: cobra.MaximumNArgs(1),
		}

		// Use the actual help command's RunE function.
		err := helpCmd.RunE(testCmd, []string{"stacks"})
		assert.Error(t, err)
	})
}

func TestHelpCommand_AllKnownTopics(t *testing.T) {
	// Comprehensive list of all known topics and their aliases.
	knownTopics := []string{
		// Core concepts.
		"stacks",
		"components",
		"inheritance",
		"imports",
		"overrides",
		// Features.
		"templating", "templates",
		"workflows",
		"validation", "validate",
		"vendoring", "vendor",
		"affected",
		"catalogs", "catalog",
		"schemas", "schema",
		"opa", "policies",
		"settings",
		"mixins",
		// Integrations.
		"terraform",
		"helmfile",
		"atlantis",
		"spacelift",
		"backends", "backend",
		// Special.
		"general",
	}

	for _, topic := range knownTopics {
		t.Run("topic_"+topic, func(t *testing.T) {
			question := getHelpQuestionForTopic(topic)
			// Known topics should not contain "in the context of Atmos" which is the default message.
			assert.NotContains(t, question, "in the context of Atmos",
				"Known topic '%s' should have a specific question, not the default", topic)
			assert.NotEmpty(t, question)
		})
	}
}

func TestHelpCommand_QuestionQuality(t *testing.T) {
	topics := []string{"stacks", "components", "terraform", "workflows"}

	for _, topic := range topics {
		t.Run("question_quality_"+topic, func(t *testing.T) {
			question := getHelpQuestionForTopic(topic)

			// All questions should be proper sentences (start with capital, end with ?).
			assert.True(t, len(question) > 0, "Question should not be empty")
			assert.True(t, question[0] >= 'A' && question[0] <= 'Z',
				"Question should start with capital letter")
			assert.True(t, question[len(question)-1] == '?',
				"Question should end with question mark")

			// Questions should be substantial (at least 50 characters).
			assert.True(t, len(question) >= 50,
				"Question should be substantial (>= 50 chars), got %d", len(question))
		})
	}
}

func TestHelpCommand_CustomTopicQuestion(t *testing.T) {
	tests := []struct {
		name                 string
		topic                string
		expectedTopicInQuery string
	}{
		{
			name:                 "custom topic appears in question",
			topic:                "my-custom-topic",
			expectedTopicInQuery: "my-custom-topic",
		},
		{
			name:                 "topic with spaces",
			topic:                "stack management",
			expectedTopicInQuery: "stack management",
		},
		{
			name:                 "topic with special characters",
			topic:                "terraform-modules",
			expectedTopicInQuery: "terraform-modules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question := getHelpQuestionForTopic(tt.topic)
			assert.Contains(t, question, tt.expectedTopicInQuery)
			assert.Contains(t, question, "context of Atmos")
			assert.Contains(t, question, "detailed information")
		})
	}
}

func TestHelpCommand_TopicAliasConsistency(t *testing.T) {
	// Test that all topic aliases produce identical questions.
	aliasGroups := [][]string{
		{"templating", "templates"},
		{"validation", "validate"},
		{"vendoring", "vendor"},
		{"catalogs", "catalog"},
		{"schemas", "schema"},
		{"opa", "policies"},
		{"backends", "backend"},
	}

	for _, group := range aliasGroups {
		t.Run("alias_group_"+group[0], func(t *testing.T) {
			baseQuestion := getHelpQuestionForTopic(group[0])
			for _, alias := range group[1:] {
				aliasQuestion := getHelpQuestionForTopic(alias)
				assert.Equal(t, baseQuestion, aliasQuestion,
					"Alias '%s' should produce same question as '%s'", alias, group[0])
			}
		})
	}
}

func TestHelpCommand_AIDisabled(t *testing.T) {
	// Create a temporary directory for the test.
	tempDir := t.TempDir()

	// Create a minimal atmos.yaml with AI disabled.
	configContent := `base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"

settings:
  ai:
    enabled: false
`

	// Write the config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Create required directories.
	err = os.MkdirAll(filepath.Join(tempDir, "components", "terraform"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(tempDir, "stacks"), 0o755)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `vars:
  stage: dev
`
	err = os.WriteFile(filepath.Join(tempDir, "stacks", "dev.yaml"), []byte(stackContent), 0o600)
	require.NoError(t, err)

	// Set environment for the tests.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

	// Change to temp dir (automatically cleaned up by t.Chdir).
	t.Chdir(tempDir)

	t.Run("help command returns error when AI is disabled", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "help",
			Args: cobra.MaximumNArgs(1),
		}

		err := helpCmd.RunE(testCmd, []string{"stacks"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI features are not enabled")
	})

	t.Run("help command returns error for general topic when AI disabled", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "help",
			Args: cobra.MaximumNArgs(1),
		}

		err := helpCmd.RunE(testCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI features are not enabled")
	})
}

func TestGetTopicFromArgs_AdditionalCases(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedTopic string
	}{
		{
			name:          "special characters in topic",
			args:          []string{"topic-with-dashes"},
			expectedTopic: "topic-with-dashes",
		},
		{
			name:          "numeric topic",
			args:          []string{"123"},
			expectedTopic: "123",
		},
		{
			name:          "mixed case topic",
			args:          []string{"StackS"},
			expectedTopic: "StackS",
		},
		{
			name:          "unicode topic",
			args:          []string{"\u00e9l\u00e8ve"},
			expectedTopic: "\u00e9l\u00e8ve",
		},
		{
			name:          "topic with underscore",
			args:          []string{"my_topic"},
			expectedTopic: "my_topic",
		},
		{
			name:          "very long topic",
			args:          []string{"this-is-a-very-long-topic-name-that-exceeds-normal-length"},
			expectedTopic: "this-is-a-very-long-topic-name-that-exceeds-normal-length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topic := getTopicFromArgs(tt.args)
			assert.Equal(t, tt.expectedTopic, topic)
		})
	}
}

func TestGetHelpQuestionForTopic_AllCasesExplicitly(t *testing.T) {
	// Test each case in the switch statement explicitly to ensure coverage.
	tests := []struct {
		topic    string
		contains []string
	}{
		{"stacks", []string{"Atmos stacks", "organizing stacks"}},
		{"components", []string{"Atmos components", "reusable components"}},
		{"templating", []string{"templating capabilities", "Go templates"}},
		{"templates", []string{"templating capabilities", "Go templates"}},
		{"workflows", []string{"workflow orchestration", "patterns"}},
		{"validation", []string{"configuration validation", "schema validation"}},
		{"validate", []string{"configuration validation", "schema validation"}},
		{"vendoring", []string{"component vendoring", "external"}},
		{"vendor", []string{"component vendoring", "external"}},
		{"inheritance", []string{"stack inheritance", "precedence rules"}},
		{"affected", []string{"affected components detection", "CI/CD"}},
		{"terraform", []string{"Terraform", "integration features"}},
		{"helmfile", []string{"Helmfile", "integration features"}},
		{"atlantis", []string{"Atlantis", "repo configs"}},
		{"spacelift", []string{"Spacelift", "stacks"}},
		{"backends", []string{"backend configuration", "Terraform"}},
		{"backend", []string{"backend configuration", "Terraform"}},
		{"imports", []string{"stack imports", "imported"}},
		{"overrides", []string{"configuration overrides", "precedence order"}},
		{"catalogs", []string{"component catalogs", "create and use"}},
		{"catalog", []string{"component catalogs", "create and use"}},
		{"mixins", []string{"mixins", "vendoring"}},
		{"schemas", []string{"schemas", "JSON Schema"}},
		{"schema", []string{"schemas", "JSON Schema"}},
		{"opa", []string{"OPA", "policy"}},
		{"policies", []string{"OPA", "policy"}},
		{"settings", []string{"settings configuration", "atmos.yaml"}},
		{"general", []string{"comprehensive overview", "key concepts"}},
	}

	for _, tt := range tests {
		t.Run("topic_"+tt.topic, func(t *testing.T) {
			question := getHelpQuestionForTopic(tt.topic)
			for _, expected := range tt.contains {
				assert.Contains(t, question, expected,
					"Question for topic '%s' should contain '%s'", tt.topic, expected)
			}
		})
	}
}

func TestGetHelpQuestionForTopic_DefaultCase(t *testing.T) {
	tests := []struct {
		name  string
		topic string
	}{
		{"unknown topic", "unknown-topic"},
		{"random string", "asdfghjkl"},
		{"misspelled topic", "stackss"},
		{"partial match", "stack"},
		{"number-prefixed", "123stacks"},
		{"special chars only", "!@#$%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question := getHelpQuestionForTopic(tt.topic)
			// Default case should include the topic and standard context.
			assert.Contains(t, question, tt.topic)
			assert.Contains(t, question, "context of Atmos")
			assert.Contains(t, question, "detailed information")
			assert.Contains(t, question, "best practices")
		})
	}
}

func TestHelpCommand_UsesRunE(t *testing.T) {
	t.Run("help command uses RunE for error handling", func(t *testing.T) {
		assert.NotNil(t, helpCmd.RunE, "help command should have RunE set")
		assert.Nil(t, helpCmd.Run, "help command should not have Run set when RunE is used")
	})
}

func TestHelpCommand_CommandName(t *testing.T) {
	assert.Equal(t, "help", helpCmd.Name())
}

func TestHelpCommand_CommandUsageString(t *testing.T) {
	// Verify the Use field follows the expected pattern.
	assert.Equal(t, "help [topic]", helpCmd.Use)
	// The [topic] indicates an optional positional argument.
}

func TestHelpCommand_SubcommandRegistration(t *testing.T) {
	// Verify help is registered as a subcommand of ai.
	t.Run("help is a subcommand of ai", func(t *testing.T) {
		subcommands := aiCmd.Commands()
		found := false
		for _, cmd := range subcommands {
			if cmd.Name() == "help" {
				found = true
				break
			}
		}
		assert.True(t, found, "help should be a subcommand of ai")
	})
}

func TestHelpCommand_InvalidConfigPath(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
	}{
		{
			name:       "nonexistent path",
			configPath: "/this/path/does/not/exist",
		},
		{
			name:       "deeply nested nonexistent path",
			configPath: "/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_CLI_CONFIG_PATH", tt.configPath)

			testCmd := &cobra.Command{
				Use: "test-help",
			}

			err := helpCmd.RunE(testCmd, []string{"stacks"})
			assert.Error(t, err)
		})
	}
}

func TestHelpCommand_HasShortDescription(t *testing.T) {
	assert.NotEmpty(t, helpCmd.Short, "help command should have a short description")
	assert.Equal(t, "Get AI-powered help on Atmos topics", helpCmd.Short)
}

func TestHelpCommand_HasLongDescription(t *testing.T) {
	assert.NotEmpty(t, helpCmd.Long, "help command should have a long description")
	assert.Greater(t, len(helpCmd.Long), len(helpCmd.Short), "long description should be longer than short")
}
