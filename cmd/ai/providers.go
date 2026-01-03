// Package ai provides the AI CLI commands.
// This file imports all AI providers to trigger their registration via init() functions.
package ai

import (
	// Import providers to trigger registration.
	_ "github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/azureopenai"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/bedrock"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/gemini"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/grok"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/ollama"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/openai"
)
