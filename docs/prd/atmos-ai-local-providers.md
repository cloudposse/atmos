# Atmos AI Local Providers — Use Claude Code, Gemini CLI, and OpenAI Codex Instead of API Tokens

**Status:** Draft
**Version:** 0.1
**Last Updated:** 2026-03-31

---

## Executive Summary

Atmos AI currently requires users to purchase API tokens from providers (Anthropic, OpenAI,
Google, etc.) to use AI features like `atmos ai chat` or `--ai` flag analysis. Many users
already have Claude Code or Gemini CLI installed with active subscriptions (Claude Max at
$100-200/mo, or Gemini's free tier with Google account).

This PRD proposes adding **local CLI providers** that invoke the user's installed `claude`
or `gemini` binary as a subprocess, reusing their existing subscription instead of requiring
separate API tokens.

**Key Finding:** Both Claude Code (`claude -p`) and Gemini CLI (`gemini -p`) support
non-interactive mode with structured JSON output, making subprocess integration
straightforward. No new protocols or SDKs needed — just `exec.Command` + JSON parsing.

### Why This Matters

1. **No API tokens to buy** — Users with Claude Max or Google accounts use their existing
   subscription. Zero additional cost.
2. **Familiar auth** — Users already authenticated with `claude` or `gemini` on their
   system. No API key configuration in `atmos.yaml`.
3. **Latest models** — CLI tools auto-update. Users always get the latest models without
   Atmos needing to update provider code.
4. **Free tier** — Gemini CLI offers 1,000 requests/day free with just a Google account.
5. **Simplicity** — New users can `brew install claude` + `atmos ai chat` with zero
   configuration. The current flow requires: create API account → generate key →
   configure `atmos.yaml` → set env var.

---

## Feasibility Analysis

### Claude Code CLI (`claude -p`)

**Feasibility: YES — HIGH**

Claude Code supports a non-interactive print mode that accepts a prompt and returns
structured output:

```bash
# Basic usage.
claude -p "Explain this terraform plan"

# Structured JSON output.
claude -p "Analyze this" --output-format json

# Schema-validated output.
claude -p "List issues" --json-schema '{"type":"object","properties":{"issues":{"type":"array"}}}'

# Pipe context via stdin.
cat plan.txt | claude -p "Analyze this terraform plan"

# Control tool access and budget.
claude -p "query" --max-turns 3 --max-budget-usd 0.50 --allowedTools "Read,Glob,Grep"

# Custom system prompt.
claude -p "query" --append-system-prompt "You are an Atmos infrastructure expert"

# Load MCP servers (Atmos can provide its own MCP config).
claude -p "query" --mcp-config ./atmos-mcp.json

# Continue a conversation.
claude -p "follow up" --resume <session-id>
```

**Output format (`--output-format json`):**
```json
{
  "type": "result",
  "subtype": "success",
  "cost_usd": 0.003,
  "duration_ms": 1250,
  "duration_api_ms": 980,
  "is_error": false,
  "num_turns": 1,
  "result": "The terraform plan shows 3 resources will be created...",
  "session_id": "abc123",
  "total_cost_usd": 0.003
}
```

**Authentication:** Uses the user's Claude Code OAuth session (Claude Pro/Max subscription).
No API key needed. The user authenticates once with `claude auth login`.

**Pricing:** Included in Claude Pro ($20/mo) or Claude Max ($100-200/mo) subscription.
No per-token charges.

### Gemini CLI (`gemini -p`)

**Feasibility: YES — HIGH**

Gemini CLI supports non-interactive mode:

```bash
# Basic usage.
gemini -p "Explain this infrastructure"

# JSON output.
gemini -p "Analyze" --output-format json

# Streaming JSON events.
gemini -p "query" --output-format stream-json

# Model selection.
gemini -p "query" -m gemini-2.5-flash

# Include directory context.
gemini -p "Review this component" --include-directories ../components
```

**Authentication:** Google Sign-In (OAuth) via browser. No API key required for free tier.

**Pricing:**
- Free tier: 60 requests/min, 1,000 requests/day (with Google account)
- Paid tier: Higher rate limits with AI Studio API key

### OpenAI Codex CLI (`codex exec`)

**Feasibility: YES — HIGH**

OpenAI Codex CLI is a full-featured coding agent comparable to Claude Code. It supports
non-interactive execution via the `codex exec` subcommand:

```bash
# Basic non-interactive usage.
codex exec "Explain this terraform plan"

# Structured JSONL output (streaming events).
codex exec --json "Analyze this infrastructure"

# JSON Schema validated output.
codex exec --output-schema ./response-schema.json "List issues"

# Pipe context via stdin.
cat plan.txt | codex exec -

# Full-auto mode (no approval prompts).
codex exec --full-auto "Fix the linting errors"

# Save final response to file.
codex exec -o result.txt "Summarize the changes"

# Select model.
codex exec -m gpt-5.4-mini "Quick analysis"

# Resume a previous session.
codex exec resume --last

# Load MCP servers.
# Configured via ~/.codex/config.toml [mcp_servers.<name>] section.
```

**Output format (`--json`):**

Codex CLI emits JSONL (newline-delimited JSON) events:
```json
{"type":"thread.started","session_id":"abc123"}
{"type":"item.completed","item":{"type":"message","content":[{"type":"text","text":"Analysis..."}]}}
{"type":"turn.completed","usage":{"input_tokens":1200,"output_tokens":450}}
```

**Authentication — dual model:**
- **ChatGPT subscription** (default): `codex login` — usage counts against plan limits
  - Plus ($20/mo): 30-150 messages per 5 hours
  - Pro ($200/mo): 300-1,500 messages per 5 hours
  - Team/Business/Enterprise: included
- **API key**: `CODEX_API_KEY` env var — billed per token

**MCP support:** Full MCP client AND server. Can load MCP servers from config and also
act as an MCP server itself (`codex mcp-server`).

**Installation:**
- npm: `npm install -g @openai/codex`
- Homebrew: `brew install --cask codex`
- Binary: GitHub Releases (macOS/Linux ARM/x86)

**Models:** gpt-5.4 (default), gpt-5.4-mini, gpt-5.3-codex, local models via `--oss` (Ollama).

**Unique features vs Claude Code/Gemini CLI:**
- `--output-schema` for JSON Schema validated output
- `codex cloud exec` for remote/cloud execution
- `--oss` flag for local Ollama models (no cloud needed)
- TypeScript SDK (`@openai/codex-sdk`) for programmatic embedding
- Open source (Apache 2.0)
- Can act as MCP server (`codex mcp-server`)

### Summary of All Local AI Tools

| Tool | Non-Interactive Mode | Structured Output | MCP | Subscription Auth | Free Tier | Feasibility |
|------|---------------------|-------------------|-----|-------------------|-----------|-------------|
| Claude Code | `claude -p` | JSON | Client only | Claude Pro/Max | No | **YES — HIGH** |
| Codex CLI | `codex exec` | JSONL + Schema | Client + Server | ChatGPT Plus/Pro | No | **YES — HIGH** |
| Gemini CLI | `gemini -p` | JSON | Client | Google account | Yes (1K/day) | **YES — HIGH** |
| GitHub Copilot CLI | Retired | N/A | N/A | N/A | N/A | NO |
| Cursor CLI | No programmatic API | N/A | N/A | N/A | N/A | NO |

### Claude Agent SDK / Codex SDK — Why NOT to Use Them

Both Claude Agent SDK (Python/TypeScript) and Codex SDK (TypeScript) exist but are
**not suitable** for direct Atmos integration:

1. **Language mismatch** — Both SDKs are Python/TypeScript, Atmos is Go. Would require
   bundling a runtime.
2. **Licensing restriction (Claude)** — Anthropic explicitly states: "Unless previously
   approved, Anthropic does not allow third party developers to offer claude.ai login or
   rate limits for their products."
3. **Unnecessary** — The CLI tools (`claude -p`, `codex exec`, `gemini -p`) provide
   everything the SDKs do, with simpler integration (subprocess vs. FFI).

**Important distinction:** When Atmos invokes the user's locally installed CLI binary,
the user is running their own tool — Atmos is not "offering" any provider's login. This is
the same pattern as Atmos invoking `terraform` — it uses the user's installation, not a
bundled copy.

---

## Architecture

### Provider Registration

Three new providers join the existing 7-provider registry:

```
pkg/ai/agent/
├── anthropic/       # (existing) Direct Anthropic API
├── openai/          # (existing) Direct OpenAI API
├── gemini/          # (existing) Direct Google AI API
├── grok/            # (existing) Direct xAI API
├── ollama/          # (existing) Local Ollama API
├── bedrock/         # (existing) AWS Bedrock API
├── azureopenai/     # (existing) Azure OpenAI API
├── claudecode/      # (NEW) Claude Code CLI subprocess
├── codexcli/        # (NEW) OpenAI Codex CLI subprocess
└── geminicli/       # (NEW) Gemini CLI subprocess
```

Each new provider implements the existing `registry.Client` interface by shelling out to
the CLI binary and parsing JSON/JSONL responses.

### How It Works

```
User: atmos ai chat  (with provider: claude-code)

1. Atmos checks: is `claude` on PATH?
   exec.LookPath("claude") → /usr/local/bin/claude ✓

2. Atmos builds the prompt with context:
   - Stack info, component details, ATMOS.md instructions
   - Skill system prompts (if --skill used)
   - Previous conversation (if --continue)

3. Atmos invokes Claude Code as subprocess:
   cmd := exec.Command("claude", "-p",
       "--output-format", "json",
       "--append-system-prompt", systemPrompt,
       "--max-turns", "5",
   )
   cmd.Stdin = strings.NewReader(prompt)

4. Claude Code uses the user's subscription:
   - No API key needed
   - Claude Max/Pro auth via OAuth session
   - Rate limits from their subscription tier

5. Atmos parses the JSON response:
   {
     "result": "Analysis text...",
     "cost_usd": 0.003,
     "session_id": "abc123"
   }

6. Atmos displays the result with markdown rendering.
```

### Configuration

**Zero-config mode** (auto-detect installed CLI):

```yaml
# atmos.yaml
ai:
  provider: claude-code
  # No api_key, no api_base_url, no model needed.
  # Atmos auto-detects the installed claude binary and uses
  # the user's subscription.
```

**Explicit configuration:**

```yaml
ai:
  provider: claude-code
  claude_code:
    # Path to claude binary (optional, defaults to exec.LookPath).
    binary: /usr/local/bin/claude
    # Max agentic turns per invocation.
    max_turns: 5
    # Budget cap per invocation (USD).
    max_budget_usd: 1.00
    # Allowed tools for Claude Code to use.
    allowed_tools:
      - Read
      - Glob
      - Grep
    # Fallback model when primary is overloaded.
    fallback_model: sonnet
```

```yaml
ai:
  provider: codex-cli
  codex_cli:
    # Path to codex binary (optional, defaults to exec.LookPath).
    binary: /usr/local/bin/codex
    # Model selection.
    model: gpt-5.4-mini
    # Approval policy: full-auto runs without prompts.
    full_auto: true
    # JSON Schema for structured output validation.
    output_schema: ""
```

```yaml
ai:
  provider: gemini-cli
  gemini_cli:
    binary: /usr/local/bin/gemini
    model: gemini-2.5-flash
    # Include component directories for context.
    include_directories: true
```

### Provider Implementations

#### Claude Code Provider

```go
// pkg/ai/agent/claudecode/client.go

type Client struct {
    binaryPath string
    maxTurns   int
    maxBudget  float64
    tools      []string
    fallback   string
}

func (c *Client) SendMessage(ctx context.Context, prompt string) (string, error) {
    args := []string{
        "-p",
        "--output-format", "json",
        "--max-turns", strconv.Itoa(c.maxTurns),
    }
    if c.maxBudget > 0 {
        args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", c.maxBudget))
    }
    if c.fallback != "" {
        args = append(args, "--fallback-model", c.fallback)
    }

    cmd := exec.CommandContext(ctx, c.binaryPath, args...)
    cmd.Stdin = strings.NewReader(prompt)

    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("claude-code execution failed: %w", err)
    }

    var response claudeCodeResponse
    if err := json.Unmarshal(output, &response); err != nil {
        return "", fmt.Errorf("failed to parse claude-code response: %w", err)
    }

    if response.IsError {
        return "", fmt.Errorf("claude-code error: %s", response.Result)
    }

    return response.Result, nil
}

type claudeCodeResponse struct {
    Type         string  `json:"type"`
    Result       string  `json:"result"`
    CostUSD      float64 `json:"cost_usd"`
    TotalCostUSD float64 `json:"total_cost_usd"`
    DurationMS   int     `json:"duration_ms"`
    IsError      bool    `json:"is_error"`
    SessionID    string  `json:"session_id"`
    NumTurns     int     `json:"num_turns"`
}
```

#### OpenAI Codex CLI Provider

```go
// pkg/ai/agent/codexcli/client.go

type Client struct {
    binaryPath   string
    model        string
    fullAuto     bool
    outputSchema string
}

func (c *Client) SendMessage(ctx context.Context, prompt string) (string, error) {
    args := []string{
        "exec",
        "--json",
    }
    if c.model != "" {
        args = append(args, "-m", c.model)
    }
    if c.fullAuto {
        args = append(args, "--full-auto")
    }
    if c.outputSchema != "" {
        args = append(args, "--output-schema", c.outputSchema)
    }

    cmd := exec.CommandContext(ctx, c.binaryPath, args...)
    cmd.Stdin = strings.NewReader(prompt)

    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("codex-cli execution failed: %w", err)
    }

    // Codex outputs JSONL events. Extract the final message from the last
    // item.completed event.
    return extractCodexResult(output)
}

// extractCodexResult parses JSONL output and extracts the final text response.
func extractCodexResult(output []byte) (string, error) {
    var lastText string
    scanner := bufio.NewScanner(bytes.NewReader(output))
    for scanner.Scan() {
        var event codexEvent
        if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
            continue
        }
        if event.Type == "item.completed" && event.Item.Type == "message" {
            for _, content := range event.Item.Content {
                if content.Type == "text" {
                    lastText = content.Text
                }
            }
        }
    }
    if lastText == "" {
        return "", fmt.Errorf("no text response found in codex output")
    }
    return lastText, nil
}

type codexEvent struct {
    Type  string    `json:"type"`
    Item  codexItem `json:"item"`
}

type codexItem struct {
    Type    string         `json:"type"`
    Content []codexContent `json:"content"`
}

type codexContent struct {
    Type string `json:"type"`
    Text string `json:"text"`
}
```

#### Gemini CLI Provider

```go
// pkg/ai/agent/geminicli/client.go

type Client struct {
    binaryPath string
    model      string
    includeDirs bool
}

func (c *Client) SendMessage(ctx context.Context, prompt string) (string, error) {
    args := []string{
        "-p",
        "--output-format", "json",
    }
    if c.model != "" {
        args = append(args, "-m", c.model)
    }

    cmd := exec.CommandContext(ctx, c.binaryPath, args...)
    cmd.Stdin = strings.NewReader(prompt)

    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("gemini-cli execution failed: %w", err)
    }

    var response geminiCLIResponse
    if err := json.Unmarshal(output, &response); err != nil {
        // Gemini CLI may return plain text in some modes.
        return string(output), nil
    }

    return response.Result, nil
}

type geminiCLIResponse struct {
    Result    string `json:"result"`
    ModelUsed string `json:"model"`
}
```

**Gemini CLI output format (`--output-format json`):**

Gemini CLI returns structured JSON with the model response. When `--output-format stream-json`
is used, it emits newline-delimited JSON events for incremental processing.

**Gemini CLI key differences from Claude Code:**

| Feature | Claude Code (`claude -p`) | Gemini CLI (`gemini -p`) |
|---------|--------------------------|-------------------------|
| **Auth** | OAuth (Claude Pro/Max) | Google Sign-In (free tier available) |
| **Cost** | $20-200/mo subscription | Free (1K req/day) or API key |
| **Structured output** | `--json-schema` for validated output | `--output-format json` |
| **Tool control** | `--allowedTools` flag | N/A |
| **Budget cap** | `--max-budget-usd` | N/A |
| **Session resume** | `--resume <session-id>` | N/A |
| **MCP config** | `--mcp-config file.json` | N/A |
| **System prompt** | `--append-system-prompt` | N/A (via prompt engineering) |
| **Model selection** | `--fallback-model` | `-m gemini-2.5-flash` |
| **Directory context** | N/A (uses MCP/tools) | `--include-directories` |
| **Max turns** | `--max-turns N` | N/A |

**When to use which:**

- **Claude Code** — Best for complex infrastructure analysis, tool-use workflows, MCP
  integration, and users with Claude Max subscriptions. Richest feature set.
- **Codex CLI** — Best for OpenAI users with ChatGPT Plus/Pro subscriptions. Full MCP
  support (client + server), JSON Schema output validation, open source, local model
  support via Ollama.
- **Gemini CLI** — Best for cost-conscious users (free tier), quick queries, and
  environments where Google auth is already available. Simpler but effective.

### MCP Integration — Best of Both Worlds

The most powerful usage combines local providers with MCP:

```yaml
ai:
  provider: claude-code
  mcp:
    integrations:
      aws-cost-explorer:
        command: "uvx"
        args: ["awslabs.cost-explorer-mcp-server@latest"]
        auth_identity: "billing-readonly"
```

When the user runs `atmos ai chat`:

1. Atmos starts the AWS Cost Explorer MCP server (with auth credentials).
2. Atmos generates a temporary `mcp.json` config pointing to the running MCP server.
3. Atmos invokes `claude -p --mcp-config /tmp/atmos-mcp.json "query"`.
4. Claude Code can use both its built-in tools AND the Atmos MCP tools AND the AWS MCP
   tools — all through the user's Claude Max subscription.

```
User's Claude Max subscription
         │
    claude -p --mcp-config atmos-mcp.json
         │
    ┌────┴────────────────────────────┐
    │         Claude Code              │
    │  ┌──────────┐  ┌──────────────┐ │
    │  │ Built-in │  │ MCP Clients  │ │
    │  │ Tools    │  │              │ │
    │  │ (Read,   │  │ ┌──────────┐ │ │
    │  │  Edit,   │  │ │ Atmos    │ │ │
    │  │  Bash)   │  │ │ MCP Srv  │ │ │
    │  └──────────┘  │ ├──────────┤ │ │
    │                │ │ AWS Cost │ │ │
    │                │ │ Explorer │ │ │
    │                │ └──────────┘ │ │
    │                └──────────────┘ │
    └─────────────────────────────────┘
```

---

## User Experience Comparison

### Current Flow (API Tokens)

```
1. Sign up for Anthropic Console account
2. Add payment method
3. Generate API key
4. Configure atmos.yaml:
   ai:
     provider: anthropic
     api_key_env_var: ANTHROPIC_API_KEY
5. Set env var: export ANTHROPIC_API_KEY=sk-ant-...
6. Run: atmos ai chat
   → Pay per token ($3-15 per million tokens)
```

### New Flow (Local Provider)

```
1. Install Claude Code: brew install claude  (already done by most users)
2. Authenticate: claude auth login  (already done by most users)
3. Configure atmos.yaml:
   ai:
     provider: claude-code
4. Run: atmos ai chat
   → Uses existing Claude Max subscription
   → No additional cost
```

### Even Simpler — Auto-Detection

```
# If claude is on PATH and no provider is configured:
ai:
  enabled: true
  # provider auto-detected: claude-code (found /usr/local/bin/claude)
```

---

## Comparison Matrix

| Feature | API Providers (Current) | Claude Code | Codex CLI | Gemini CLI |
|---------|------------------------|-------------|-----------|------------|
| **Setup** | API account + key + config | `brew install claude` | `npm i -g @openai/codex` | `npm i -g @google/gemini-cli` |
| **Auth** | API key in env var | OAuth (subscription) | OAuth or API key | Google Sign-In |
| **Cost** | Per-token ($3-15/M tokens) | $20-200/mo subscription | $20-200/mo or per-token | Free (1K/day) |
| **Models** | Configurable | Latest (auto-updates) | gpt-5.4, gpt-5.4-mini | gemini-2.5-flash |
| **Tool use** | Atmos tools only | Claude tools + MCP | Codex tools + MCP | Gemini tools |
| **MCP** | N/A | Client only | Client + Server | Client |
| **Structured output** | Provider-dependent | JSON + schema | JSONL + JSON Schema | JSON |
| **Session** | Atmos-managed SQLite | Claude-managed | Codex-managed | N/A |
| **Offline** | No (except Ollama) | No | Yes (`--oss` Ollama) | No |
| **Rate limits** | API-specific | Subscription tier | Subscription or API | 60/min, 1K/day (free) |
| **Open source** | N/A | No | Yes (Apache 2.0) | Yes (Apache 2.0) |

---

## Phased Implementation

### Phase 1: Claude Code Provider (MVP)

- Implement `pkg/ai/agent/claudecode/` with `registry.Client` interface.
- Auto-detect `claude` binary via `exec.LookPath`.
- Support `claude -p --output-format json` invocation.
- Parse `claudeCodeResponse` JSON.
- Configuration in `atmos.yaml` under `ai.provider: claude-code`.
- Error handling: binary not found, auth expired, rate limited.

### Phase 2: Codex CLI + Gemini CLI Providers

- Implement `pkg/ai/agent/codexcli/` with `registry.Client` interface.
- Auto-detect `codex` binary. Support `codex exec --json` invocation.
- Parse JSONL event stream to extract final response.
- Implement `pkg/ai/agent/geminicli/` with `registry.Client` interface.
- Auto-detect `gemini` binary. Support `gemini -p --output-format json` invocation.
- Configuration in `atmos.yaml` under `ai.provider: codex-cli` / `gemini-cli`.

### Phase 3: MCP Pass-Through

- Generate temporary MCP config from `ai.mcp.servers`.
- Pass `--mcp-config` to `claude -p` invocations (Claude Code).
- Pass MCP config via `~/.codex/config.toml` for Codex CLI.
- All three local providers gain access to configured MCP servers (AWS, Atmos, custom).
- This combines the MCP Integrations PRD with local providers.

### Phase 4: Auto-Detection and Smart Defaults

- Auto-detect installed CLI tools and suggest/use the best available provider.
- Fallback chain: `claude-code` → `codex-cli` → `gemini-cli` → prompt for API key.
- Display provider and cost info in `atmos ai` output.
- Session continuity via `--resume` for Claude Code and Codex CLI.

---

## Limitations and Trade-offs

### Limitations

1. **No tool-use loop** — Claude Code's `-p` mode runs its own tool loop internally.
   Atmos cannot inject custom tools mid-conversation (but can provide them via MCP).
2. **No streaming to Atmos TUI** — The subprocess completes before output is available
   (unless `stream-json` is parsed incrementally).
3. **Binary dependency** — Users must have `claude` or `gemini` installed. Not all
   environments (CI/CD containers) will have them.
4. **Version coupling** — Claude Code's `-p` output format could change between versions.
   Atmos needs to handle format evolution gracefully.
5. **Rate limits** — Subscription rate limits may be lower than API rate limits for
   high-volume usage.

### Trade-offs

| | API Providers | Local Providers |
|---|---|---|
| **Control** | Full control over prompts, tools, tokens | Limited to CLI flags |
| **Cost predictability** | Pay-per-use (variable) | Fixed subscription (predictable) |
| **CI/CD** | Works everywhere with env var | Requires CLI installation + auth |
| **Tool execution** | Atmos tools run in-process | Tools run inside Claude Code process |
| **Latency** | Direct API call | Subprocess spawn + API call |

### Recommended Usage

- **Interactive development** → Claude Code provider (subscription, rich features)
- **CI/CD pipelines** → API providers (env var auth, no interactive login)
- **Cost-conscious users** → Gemini CLI provider (free tier)
- **Enterprise** → API providers or Bedrock (compliance, audit trails)

---

## References

- [Claude Code CLI Reference](https://docs.anthropic.com/en/docs/claude-code/cli-reference)
- [Claude Code Non-Interactive Mode](https://docs.anthropic.com/en/docs/claude-code/cli-usage#non-interactive-mode)
- [OpenAI Codex CLI](https://github.com/openai/codex)
- [Codex CLI Non-Interactive Mode](https://developers.openai.com/codex/noninteractive)
- [Codex CLI Reference](https://developers.openai.com/codex/cli/reference)
- [Codex CLI MCP](https://developers.openai.com/codex/mcp)
- [Codex SDK](https://developers.openai.com/codex/sdk)
- [Gemini CLI](https://github.com/google-gemini/gemini-cli)
- [Claude Agent SDK](https://docs.anthropic.com/en/docs/claude-code/claude-agent-sdk)
- [Atmos AI PRD](./atmos-ai.md) — Current AI architecture
- [Atmos MCP Integrations PRD](./atmos-mcp-integrations.md) — External MCP servers
- [Atmos AI Global Flag PRD](./atmos-ai-global-flag.md) — `--ai` flag design
