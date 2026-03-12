# Atmos AI Assistant Example

This example demonstrates how to configure and use the Atmos AI Assistant with a realistic multi-region AWS
infrastructure project.

- **Global `--ai` flag** — Add AI-powered analysis to any Atmos command (`atmos --ai terraform plan`)
- **`--skill` flag** — Domain-specific AI analysis with skills (`atmos --ai --skill atmos-terraform terraform plan`)
- **Multi-provider configuration** — Configure multiple AI providers (Anthropic, OpenAI, Gemini, Ollama)
- **Multi-region infrastructure** — Hub-spoke Transit Gateway topology across us-east-1 and us-west-2
- **Session management** — Persistent conversation history with auto-compact
- **Tool execution** — AI-powered infrastructure inspection with component dependencies
- **Project instructions** — Persistent context via ATMOS.md

## Prerequisites

Set up at least one AI provider API key:

```bash
# Choose one or more providers
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export OPENAI_API_KEY="your-openai-api-key"
# Ollama requires no API key (runs locally)
```

## Quick Start

1. Navigate to this example directory:

   ```bash
   cd examples/ai
   ```

2. Start an interactive AI chat:

   ```bash
   atmos ai chat
   ```

3. Or ask a single question:

   ```bash
   atmos ai ask "What stacks and components do we have in the infra?"
   ```

## See It in Action

Ask a question about your infrastructure, and Atmos AI automatically inspects your stacks, components, and configuration:

```text
$ atmos ai ask "what stacks and components do we have in the infra?"

👽 Thinking...

  I'll list all the available stacks and components in your infrastructure.

  Based on the infrastructure configuration, here are the stacks and components:

  ## Stacks (4 total)

  1. **ue1-network** - Network stack in us-east-1
  2. **ue1-prod** - Production stack in us-east-1
  3. **uw2-network** - Network stack in us-west-2
  4. **uw2-prod** - Production stack in us-west-2

  ## Components by Stack

  ### ue1-network

  • tgw/attachment - Transit Gateway attachment
  • tgw/hub - Transit Gateway hub
  • vpc - Virtual Private Cloud

  ### ue1-prod

  • tgw/attachment - Transit Gateway attachment
  • vpc - Virtual Private Cloud

  ### uw2-network

  • tgw/attachment - Transit Gateway attachment
  • tgw/cross-region-hub-connector - Cross-region Transit Gateway connector
  • vpc - Virtual Private Cloud

  ### uw2-prod

  • tgw/attachment - Transit Gateway attachment
  • vpc - Virtual Private Cloud

  ## Summary

  Your infrastructure is organized across two AWS regions (us-east-1 and us-west-2),
  with dedicated network and production stacks in each region. The network stacks
  contain Transit Gateway hubs for network connectivity, while the production stacks
  use Transit Gateway attachments to connect to the network infrastructure. All stacks
  include VPC components for network isolation.
```

### AI-Powered Analysis with `--skill` (Skill Not Found)

When using `--skill` with a skill name that doesn't exist, Atmos shows a helpful error:

```text
$ atmos terraform plan vpc -s ue1-network --ai --skill terraform

   Error: AI skill not found

  ## Explanation

   The skill "terraform" is not installed or configured.

  ## Hints

   💡 No skills are installed. Install skills with: atmos ai skill install cloudposse/atmos

   💡 See https://atmos.tools/ai/agent-skills for more information.
```

### AI-Powered Analysis with `--skill` (Terraform Expertise)

When using a valid skill, the AI gets deep domain expertise. Here `atmos-terraform` gives the AI
Terraform-specific knowledge for richer plan analysis:

```text
$ atmos terraform plan vpc -s ue1-network --ai --skill atmos-terraform

✓ AI analysis complete (skill: atmos-terraform)

  ## ✅ Plan Succeeded — 1 Resource to Create

   The  vpc  component in stack  ue1-network  planned successfully. Atmos initialized the backend, switched to the  ue1-network  workspace, and generated a clean plan.

  --------

  ### 📋 What Will Be Created

   Resource            │ Type            │ Key Configuration
  ─────────────────────┼─────────────────┼────────────────────────────────────────────────────
    null_resource.vpc  │  null_resource  │ VPC CIDR  10.1.0.0/16 , 3 AZs, NAT gateway enabled

   Outputs that will be created:

    •  vpc_cidr :  10.1.0.0/16
    •  availability_zones :  us-east-1a ,  us-east-1b ,  us-east-1c
    •  nat_gateway_enabled :  true
    •  subnet_count :  3
    •  tags :  Environment=network ,  Region=us-east-1

  --------

  ### ⚠️ Warnings — Undeclared Variables

   Terraform found 3 variables in the generated  tfvars.json  that have no corresponding  variable  block in the module:  environment ,  region , and one additional undeclared variable. These values exist in your Atmos stack config but are not consumed by the Terraform module.

   Root cause: Atmos passes all  vars  from the stack manifest to Terraform, but the  vpc  module hasn't declared matching  variable  blocks.

   Fix options (choose one):

    1. Declare the variables in your module (recommended if you intend to use them):
     variable "environment" {}
     variable "region" {}

    2. Remove unused vars from the stack config if they're not needed by this component:
     components:
       terraform:
         vpc:
           vars:
             # Remove environment, region if not used

    3. Suppress warnings only by passing  -compact-warnings  (doesn't fix the underlying issue):
     atmos terraform plan vpc -s ue1-network -- -compact-warnings


  --------

  ### 🚀 Next Steps

   When ready to apply the reviewed plan:

   atmos terraform apply vpc -s ue1-network --from-plan

   Or for automated deployment:

   atmos terraform deploy vpc -s ue1-network
```

### AI-Powered Plan Analysis with `--ai`

Run `terraform plan` with the `--ai` flag to get an AI-powered summary of changes, warnings, and recommendations:

```text
$ atmos terraform plan vpc -s ue1-prod --ai

Initializing the backend...
Initializing provider plugins...
- Reusing previous version of hashicorp/null from the dependency lock file
- Using previously-installed hashicorp/null v3.2.4

Terraform has been successfully initialized!
Switched to workspace "ue1-prod".

Terraform used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

Terraform will perform the following actions:

  # null_resource.vpc will be created
  + resource "null_resource" "vpc" {
      + id       = (known after apply)
      + triggers = {
          + "availability_zones"  = "us-east-1a,us-east-1b,us-east-1c"
          + "environment"         = "production"
          + "nat_gateway_enabled" = "true"
          + "vpc_cidr"            = "10.10.0.0/16"
        }
    }

Plan: 1 to add, 0 to change, 0 to destroy.

Changes to Outputs:
  + availability_zones  = [
      + "us-east-1a",
      + "us-east-1b",
      + "us-east-1c",
    ]
  + nat_gateway_enabled = true
  + subnet_count        = 3
  + tags                = {
      + Environment = "production"
      + Region      = "us-east-1"
    }
  + vpc_cidr            = "10.10.0.0/16"


👽 Analyzing with AI...


  ## Terraform Plan Summary:  vpc  in  ue1-prod

  ### ✅ Plan Succeeded — 1 Resource to Create

   Metric     │ Value
  ────────────┼───────
   To Add     │ 1
   To Change  │ 0
   To Destroy │ 0

  --------

  ### 📦 Resource Being Created

   A  null_resource.vpc  will be created with the following configuration:

   Parameter             │ Value
  ───────────────────────┼──────────────────────────────────────────
    vpc_cidr             │  10.10.0.0/16
    environment          │  production
    availability_zones   │  us-east-1a ,  us-east-1b ,  us-east-1c
    nat_gateway_enabled  │  true

  │  Note: This appears to be a placeholder/stub module using  null_resource  rather than real
  │  AWS VPC resources. This is likely a demo or skeleton component — not an actual VPC being
  │  provisioned in AWS.

  --------

  ### ⚠️ Warnings (Non-Blocking)

   Three variables passed via  ue1-prod-vpc.terraform.tfvars.json  are not declared in the
   Terraform module:

    •  region
    •  stage
    • 1 additional undeclared variable

   These warnings do not block the plan, but they indicate the variables are silently ignored.

  #### Recommended Fixes

   Option 1 — Declare the variables in your module (preferred):

   variable "region" {
     type        = string
     description = "AWS region"
   }

   variable "stage" {
     type        = string
     description = "Deployment stage (e.g. prod, staging)"
   }

   Option 2 — Use  TF_VAR_  env vars for org-wide globals to avoid passing them through
   tfvars files.

   Option 3 — Suppress warnings by running with  -compact-warnings  if the variables are
   intentionally unused.

  --------

  ### 🔑 Key Takeaway

   The plan is safe to apply, but the undeclared variable warnings suggest the VPC module is
   incomplete — it's not consuming  region  or  stage , which are likely needed for proper
   resource tagging, naming, or actual AWS resource provisioning. Review whether this module
   needs to be fleshed out before production use.
```

### AI-Powered Error Analysis with `--ai`

When a command fails, the `--ai` flag explains the error and provides step-by-step instructions to fix it:

```text
$ atmos terraform plan vpc -s ue1-pro  --ai

👽 Analyzing with AI...


  ## ❌ Component Not Found Error

   Atmos cannot locate the  vpc  component within the  ue1-pro  stack. This is a configuration/resolution issue, not a Terraform error.

  --------

  ## 🔍 Root Causes

   This error typically occurs for one of the following reasons:

    1. Wrong component name — The component may be named differently (e.g.,  vpc/defaults ,  networking/vpc )
    2. Wrong stack name —  ue1-pro  may not exist or may use a different naming convention
    3. Missing import — The stack manifest for  ue1-pro  doesn't import the catalog entry that defines  vpc
    4. Missing component definition — The  vpc  component is never declared under  components.terraform  in the stack's resolved config

  --------

  ## 🛠️ Step-by-Step Fix

  ### Step 1 — Verify the stack exists

   atmos list stacks

   Confirm  ue1-pro  appears in the output. Common alternatives might be  ue1-prod ,  use1-pro , or  us-east-1-pro .

  --------

  ### Step 2 — List components available in that stack

   atmos list components -s ue1-pro

   Check if  vpc  is listed. If not, the component is not configured for this stack.

  --------

  ### Step 3 — Inspect the stack manifest

   Navigate to your stacks directory (typically  stacks/ ) and look for the  ue1-pro  manifest:

   # Common locations
   stacks/ue1-pro.yaml
   stacks/orgs/<org>/ue1/pro.yaml
   stacks/catalog/vpc.yaml

   Verify the stack file contains (or imports a file containing):

   components:
     terraform:
       vpc:
         vars:
           ...

  --------

  ### Step 4 — Check for missing imports

   If the  vpc  component is defined in a catalog file, ensure it's imported in the stack manifest:

   # stacks/ue1-pro.yaml
   import:
     - catalog/vpc          # ← this line must exist
     - catalog/networking

  --------

  ### Step 5 — Describe the stack to see its fully resolved config

   atmos describe stacks --stack ue1-pro

   Search the output for  vpc  to see if it resolves at all, and spot any misconfiguration.

  --------

  ### Step 6 — Validate your configuration

   atmos validate stacks

   This will catch import errors, missing files, or schema issues across all stacks.

  --------

  ## ✅ Quick Checklist

   Check                           │ Command
  ─────────────────────────────────┼────────────────────────────────────
   Stack name is correct           │  atmos list stacks
   Component is defined in stack   │  atmos list components -s ue1-pro
   Import exists in stack manifest │ Review  stacks/ue1-pro.yaml
   No YAML syntax errors           │  atmos validate stacks

  --------

  │  💡 Tip: If you're using a deep folder structure for stacks, double-check your  atmos.yaml   base_path  and  stacks.included_paths  settings to ensure the manifest is being picked up correctly.
```

### Validate Stacks

Ask the AI to validate all stacks and present the results in a table:

```text
$ atmos ai ask "validate all stacks, show issues in a table"

👽 Thinking...

  I'll validate all Atmos stacks right away!

  Here are the validation results for all stacks:

   # | Check                                | Status    | Details
  ---|--------------------------------------|-----------|---------------------------------------
   1 | Stack Schema Validation (jsonschema) | ✅ Passed | All stacks conform to the JSON schema

  --------

  **Summary:** All stacks passed validation with **0 issues found**. Your Atmos stack
  configurations are valid and well-formed. 🎉

  If you'd like deeper validation, I can also:

  • 🔍 **Validate with OPA policies** (opa schema type) for policy-as-code checks
  • 📋 **List all stacks and components** to review configurations manually
  • 🔄 **Check affected components** based on recent git changes
```

### Automation with JSON Output

Use `atmos ai exec` for scripting and CI/CD pipelines. The `--format json` flag returns structured output
with tool call details, token usage, and metadata:

```text
$ atmos ai exec "validate stacks" --format json
{
  "success": true,
  "response": "I'll validate your Atmos stack configurations right away!\n\n✅ **Stack Validation Passed!**\n\nAll Atmos stack configurations are valid. The stacks were validated against the **JSON Schema** (`jsonschema`) and no issues were found.\n\nYour stack configurations are well-formed and ready to use",
  "tool_calls": [
    {
      "tool": "atmos_validate_stacks",
      "duration_ms": 15,
      "success": true,
      "result": {
        "schema_type": "jsonschema"
      }
    }
  ],
  "tokens": {
    "prompt": 7077,
    "completion": 188,
    "total": 7265
  },
  "metadata": {
    "model": "claude-sonnet-4-6",
    "provider": "anthropic",
    "duration_ms": 5852,
    "timestamp": "2026-03-07T22:36:24.167201-05:00",
    "tools_enabled": true,
    "stop_reason": "end_turn"
  }
}
```

## Features Demonstrated

### Multi-Region Hub-Spoke Architecture

This example models a real-world multi-region AWS networking setup using mock Terraform components
that don't create any real cloud resources. For production infrastructure, use the
[Cloud Posse Terraform components](https://github.com/cloudposse-terraform-components):

- [`aws-vpc`](https://github.com/cloudposse-terraform-components/aws-vpc)
- [`aws-tgw-hub`](https://github.com/cloudposse-terraform-components/aws-tgw-hub)
- [`aws-tgw-attachment`](https://github.com/cloudposse-terraform-components/aws-tgw-attachment)
- [`aws-tgw-hub-connector`](https://github.com/cloudposse-terraform-components/aws-tgw-hub-connector)
- [`aws-tgw-routes`](https://github.com/cloudposse-terraform-components/aws-tgw-routes)

The example architecture:

- **us-east-1 (hub)** — Transit Gateway hub with VPC and attachments
- **us-west-2 (spoke)** — Cross-region connector peering back to the hub
- **Component dependencies** — Attachments depend on VPCs and hubs across stacks
- **Mixins** — Region and stage configuration reused via imports

### Multi-Provider Configuration

Four AI providers are configured in `atmos.yaml`:

```yaml
ai:
  enabled: true
  default_provider: "anthropic"
  providers:
    anthropic:
      model: "claude-sonnet-4-6"
    openai:
      model: "gpt-5.4"
    gemini:
      model: "gemini-2.5-flash"
    ollama:
      model: "llama4"
```

Switch between providers during a chat session by pressing `Ctrl+P`.

### Tool Execution

The AI can inspect your infrastructure using built-in tools:

- `atmos_describe_component` — Describe component configuration
- `atmos_list_stacks` — List available stacks
- `atmos_validate_stacks` — Validate stack configurations

Example conversation:

```text
You: Describe the VPC component in the ue1-network stack
AI: [Uses atmos_describe_component tool]
    The VPC in ue1-network uses CIDR 10.1.0.0/16 with 3 availability zones
    (us-east-1a, us-east-1b, us-east-1c) and NAT Gateways enabled...
```

### Project Instructions (ATMOS.md)

The `ATMOS.md` file provides persistent project instructions to the AI:

- Architecture overview (hub-spoke topology)
- Stack naming conventions (`ue1-network`, `uw2-prod`, etc.)
- Component descriptions and dependencies
- Common operations

The AI reads this file automatically to provide context-aware responses.

## Directory Structure

```text
examples/ai/
├── README.md                                     # This file
├── atmos.yaml                                    # Atmos configuration with AI settings
├── ATMOS.md                                      # Project instructions for AI context
├── stacks/
│   ├── deploy/
│   │   ├── network/
│   │   │   ├── us-east-1.yaml                    # Network stack (hub region)
│   │   │   └── us-west-2.yaml                    # Network stack (spoke region)
│   │   └── prod/
│   │       ├── us-east-1.yaml                    # Production stack
│   │       └── us-west-2.yaml                    # Production stack
│   └── mixins/
│       ├── region/
│       │   ├── us-east-1.yaml                    # Region: ue1
│       │   └── us-west-2.yaml                    # Region: uw2
│       └── stage/
│           ├── network.yaml                      # Stage: network
│           └── prod.yaml                         # Stage: prod
├── components/
│   └── terraform/
│       ├── vpc/                                  # VPC component
│       │   ├── main.tf
│       │   ├── variables.tf
│       │   └── outputs.tf
│       └── tgw/
│           ├── hub/                              # Transit Gateway hub
│           │   ├── main.tf
│           │   ├── variables.tf
│           │   └── outputs.tf
│           ├── attachment/                       # Transit Gateway attachment
│           │   ├── main.tf
│           │   ├── variables.tf
│           │   └── outputs.tf
│           └── cross-region-hub-connector/       # Cross-region peering
│               ├── main.tf
│               ├── variables.tf
│               └── outputs.tf
└── workflows/
    └── ai-demo.yaml                             # Workflow demonstrating AI usage
```

## The `--ai` Flag

The global `--ai` flag adds AI-powered analysis to **any** Atmos command. When set, command output
(stdout and stderr) is automatically captured and sent to the configured AI provider for analysis.

- If the command **succeeds**, the AI provides a concise summary and key observations
- If the command **fails**, the AI explains the error and provides step-by-step instructions to fix it

The flag works with all commands — terraform, helmfile, describe, validate, list, and more.

### `--ai` Flag Examples

```bash
# AI analyzes terraform plan output and summarizes changes
atmos --ai terraform plan vpc -s ue1-network

# AI explains any errors from terraform apply
atmos --ai terraform apply vpc -s ue1-prod

# AI summarizes the component configuration
atmos --ai describe component vpc -s ue1-network

# AI analyzes validation results
atmos --ai validate stacks

# AI summarizes the list of stacks and components
atmos --ai list stacks
atmos --ai list components

# Enable via environment variable (applies to all commands)
export ATMOS_AI=true
atmos terraform plan vpc -s ue1-network
atmos describe stacks

# Combine with other flags
atmos --ai --logs-level=Debug terraform plan vpc -s ue1-prod
```

### `--skill` Flag Examples

Use `--skill` with `--ai` to give the AI domain-specific expertise for more accurate analysis:

```bash
# Terraform expertise for plan analysis
atmos --ai --skill atmos-terraform terraform plan vpc -s ue1-prod

# Stacks expertise for stack description
atmos --ai --skill atmos-stacks describe stacks

# Validation expertise for policy checks
atmos --ai --skill atmos-validation validate stacks

# Helmfile expertise for Kubernetes deployments
atmos --ai --skill atmos-helmfile helmfile diff echo-server -s ue1-prod

# Use environment variables for CI/CD
ATMOS_AI=true ATMOS_SKILL=atmos-terraform atmos terraform plan vpc -s ue1-prod
export ATMOS_SKILL=atmos-terraform
atmos --ai terraform plan vpc -s ue1-network
```

Skills are loaded from marketplace installations (`~/.atmos/skills/`) and custom skills in `atmos.yaml`.
If the specified skill is not found, Atmos shows an error listing all available skills.

### How It Works

1. You run any Atmos command with `--ai` (e.g., `atmos --ai terraform plan vpc -s ue1-network`)
2. The command runs normally — output is displayed to the terminal in real-time
3. Output is also captured in the background
4. After the command completes, the captured output is sent to the configured AI provider
5. The AI analysis is rendered as markdown below the command output

### Configuration Required

The `--ai` flag requires AI to be configured in `atmos.yaml`. This example already has it configured.
If AI is not configured, Atmos shows a helpful error with configuration instructions.

```yaml
# Minimum required in atmos.yaml:
ai:
  enabled: true
  default_provider: "anthropic"
  providers:
    anthropic:
      model: "claude-sonnet-4-6"
      api_key: !env ANTHROPIC_API_KEY
```

If AI is not configured, Atmos shows a helpful error with the exact YAML to add:

```text
$ atmos list stacks --ai

   Error

   Error: AI features are not enabled

  ## Explanation

   The --ai flag requires AI to be enabled in your atmos.yaml configuration.

  ## Hints

   💡 Add the following to your atmos.yaml:

   ai:
     enabled: true
     default_provider: anthropic
     providers:
       anthropic:
         model: claude-sonnet-4-5-20250514
         api_key: !env ANTHROPIC_API_KEY

   💡 See https://atmos.tools/cli/configuration/ai for full configuration options.
```

## Example Commands

```bash
# Interactive chat
atmos ai chat

# Single question
atmos ai ask "List all stacks"

# Describe a component
atmos ai ask "Describe the VPC in ue1-network"

# Analyze dependencies
atmos ai ask "What are the component dependencies in ue1-network?"

# AI-powered analysis of any command output
atmos --ai describe component vpc -s ue1-network
atmos --ai validate stacks
atmos --ai terraform plan vpc -s ue1-prod

# Named session
atmos ai chat --session infrastructure-review

# List sessions
atmos ai sessions list

# Run workflow
atmos workflow ai-demo
```

## Provider-Specific Notes

### Anthropic (Claude)

- Best for complex reasoning and infrastructure analysis
- Requires `ANTHROPIC_API_KEY`
- Token caching enabled by default for cost savings

### OpenAI (GPT)

- Strong general-purpose capabilities
- Requires `OPENAI_API_KEY`
- Automatic prompt caching

### Ollama (Local)

- 100% local processing, no data leaves your machine
- Requires Ollama installed and running: `ollama serve`
- Pull a model first: `ollama pull llama4`

## Keyboard Shortcuts (in chat)

| Key      | Action             |
|----------|--------------------|
| `Ctrl+P` | Switch AI provider |
| `Ctrl+A` | Switch AI skill    |
| `Ctrl+N` | Create new session |
| `Ctrl+L` | List sessions      |
| `Ctrl+C` | Exit chat          |
| `Enter`  | Send message       |
| `Ctrl+J` | New line           |

## Learn More

- [AI Assistant Documentation](https://atmos.tools/ai)
- [AI Configuration](https://atmos.tools/cli/configuration/ai)
- [AI Providers](https://atmos.tools/cli/configuration/ai/providers)
- [AI Skills](https://atmos.tools/cli/configuration/ai/skills)
- [Session Management](https://atmos.tools/cli/configuration/ai/sessions)
