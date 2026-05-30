package analyze

// Import all AI providers to trigger their registration via init() functions.
// This ensures providers are available when analyze.AnalyzeOutput creates an AI client.
import (
	_ "github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/azureopenai"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/bedrock"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/claudecode"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/codexcli"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/gemini"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/geminicli"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/grok"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/ollama"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/openai"
)
