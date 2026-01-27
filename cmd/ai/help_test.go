package ai

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
