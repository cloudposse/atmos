package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// aiHelpCmd represents the ai help command.
var aiHelpCmd = &cobra.Command{
	Use:   "help [topic]",
	Short: "Get AI-powered help on Atmos topics",
	Long: `Get intelligent help on specific Atmos topics from the AI assistant.

The AI can provide detailed explanations, examples, and best practices for various
Atmos concepts and workflows.

Common topics:
Core Concepts:
- stacks: Stack configuration and organization
- components: Component architecture and best practices
- inheritance: Configuration inheritance and precedence
- imports: Stack imports and composition
- overrides: Configuration overrides

Features:
- templating/templates: Go templating and functions
- workflows: Workflow orchestration
- validation/validate: Schema and policy validation
- vendoring/vendor: Component vendoring and mixins
- affected: Affected components detection
- catalogs: Component catalogs
- schemas/schema: JSON Schema validation
- opa/policies: OPA policy validation
- settings: Atmos settings configuration

Integrations:
- terraform: Terraform integration and best practices
- helmfile: Helmfile integration
- atlantis: Atlantis integration
- spacelift: Spacelift integration
- backends/backend: Terraform backend configuration

Examples:
  atmos ai help stacks
  atmos ai help inheritance
  atmos ai help affected
  atmos ai help terraform`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled.
		if !isAIEnabled(&atmosConfig) {
			return fmt.Errorf("%w: AI features are not enabled. Set 'ai.enabled: true' in your atmos.yaml configuration",
				errUtils.ErrAINotEnabled)
		}

		var topic string
		if len(args) > 0 {
			topic = args[0]
		} else {
			topic = "general"
		}

		// Prepare help question based on topic.
		var question string
		switch strings.ToLower(topic) {
		case "stacks":
			question = "Explain Atmos stacks in detail. What are they, how do they work, and what are best practices for organizing stacks?"
		case "components":
			question = "Explain Atmos components in detail. What are they, how do they relate to stacks, and what are best practices for creating reusable components?"
		case "templating", "templates":
			question = "Explain Atmos templating capabilities. How do Go templates work in Atmos, what functions are available, and how can I use them effectively?"
		case "workflows":
			question = "Explain Atmos workflow orchestration. How do workflows work, when should I use them, and what are some common patterns?"
		case "validation", "validate":
			question = "Explain Atmos configuration validation. How does schema validation work, how can I validate my configurations, and what are common validation issues?"
		case "vendoring", "vendor":
			question = "Explain Atmos component vendoring. How does vendoring work, when should I use it, and what are best practices for managing external components?"
		case "inheritance":
			question = "Explain Atmos stack inheritance. How does configuration inheritance work, what are the precedence rules, and what are best practices for using inheritance?"
		case "affected":
			question = "Explain Atmos affected components detection. How does it work, when should I use it, and how can I integrate it into CI/CD pipelines?"
		case "terraform":
			question = "Explain how Atmos works with Terraform. What are the key integration features, best practices, and how do I manage Terraform components in Atmos?"
		case "helmfile":
			question = "Explain how Atmos works with Helmfile. What are the key integration features, best practices, and how do I manage Helmfile components in Atmos?"
		case "atlantis":
			question = "Explain Atmos integration with Atlantis. How do I configure Atlantis for Atmos, generate repo configs, and what are best practices?"
		case "spacelift":
			question = "Explain Atmos integration with Spacelift. How do I configure Spacelift for Atmos, set up stacks, and what are best practices?"
		case "backends", "backend":
			question = "Explain Terraform backend configuration in Atmos. How are backends configured, how do I generate backend configs, and what are best practices?"
		case "imports":
			question = "Explain Atmos stack imports. How do imports work, what can be imported, and what are best practices for organizing imports?"
		case "overrides":
			question = "Explain Atmos configuration overrides. How do overrides work, what is the precedence order, and what are best practices for using overrides?"
		case "catalogs", "catalog":
			question = "Explain Atmos component catalogs. What are they, how do they work, and how can I create and use component catalogs?"
		case "mixins":
			question = "Explain Atmos mixins. What are they, how do they work with vendoring, and what are best practices for using mixins?"
		case "schemas", "schema":
			question = "Explain Atmos schemas and JSON Schema validation. How do I define schemas, validate configurations, and what are common schema patterns?"
		case "opa", "policies":
			question = "Explain OPA (Open Policy Agent) integration in Atmos. How do I write policies, validate with OPA, and what are common policy patterns?"
		case "settings":
			question = "Explain Atmos settings configuration. What settings are available in atmos.yaml, how do they affect behavior, and what are best practices?"
		case "general":
			question = "Provide a comprehensive overview of Atmos. Explain the key concepts, architecture, and how all the pieces fit together."
		default:
			question = fmt.Sprintf("Explain '%s' in the context of Atmos. Provide detailed information, examples, and best practices.", topic)
		}

		log.Debug("Getting AI help", "topic", topic)

		// Create AI client using factory.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Create context with timeout (default 60 seconds if not configured).
		timeoutSeconds := 60
		if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Send question and get response.
		utils.PrintfMessageToTUI("👽 Thinking...\n")
		response, err := client.SendMessage(ctx, question)
		if err != nil {
			return fmt.Errorf("failed to get AI response: %w", err)
		}

		// Print response.
		fmt.Println(response)

		return nil
	},
}

func init() {
	aiCmd.AddCommand(aiHelpCmd)
}
