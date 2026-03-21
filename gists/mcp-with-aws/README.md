# AWS MCP Servers with Atmos

Run 21 AWS MCP servers with automatic authentication using a single pattern. No manual credential management — `atmos auth exec` handles everything.

## The Problem

AWS publishes [MCP servers](https://github.com/awslabs/mcp) for services like Cost Explorer, CloudWatch, IAM, ECS, and more. Each server needs authenticated AWS credentials to function. Setting this up manually for 20+ servers is tedious and error-prone — you end up juggling SSO sessions, environment variables, and credential files.

## The Solution

Use Atmos to wrap everything:

- **Custom Commands** define `atmos mcp aws install/start/test` subcommands
- **Auth** wraps each MCP server process with `atmos auth exec`, injecting authenticated credentials
- **Toolchain** ensures `uv` (the Python package manager) is available for installing MCP packages
- **`.mcp.json`** tells Claude Code to start each server via `atmos mcp aws start <name>`

The result: every MCP server gets proper AWS auth automatically.

## Features Used

- [Custom Commands](https://atmos.tools/cli/configuration/commands) — nested subcommands for install, start, and test
- [Auth](https://atmos.tools/stacks/auth) — `atmos auth exec` wraps processes with authenticated AWS credentials
- [Toolchain](https://atmos.tools/cli/configuration/toolchain) — ensures `uv` is available via toolchain aliases
- [AI/MCP](https://atmos.tools/ai/mcp-server) — enables MCP server support

## How It Works

```
Claude Code → .mcp.json → atmos mcp aws start pricing
                                    ↓
                          resolve package name
                          (pricing → awslabs.aws-pricing-mcp-server)
                                    ↓
                          atmos auth exec -i core-root/terraform
                                    ↓
                          AWS SSO authentication
                                    ↓
                          uvx --python 3.13 awslabs.aws-pricing-mcp-server@latest
                          (runs with authenticated AWS credentials)
```

1. Claude Code reads `.mcp.json` and starts each configured server
2. Each server entry calls `atmos mcp aws start <server-name>`
3. The custom command resolves the short name to the full Python package name
4. `atmos auth exec -i core-root/terraform` handles AWS SSO authentication
5. The MCP server process inherits the authenticated AWS credentials via `exec`

## Getting Started

### Prerequisites

- [Atmos](https://atmos.tools/quick-start/install-atmos) installed
- AWS account with SSO configured
- Python 3.13 (for MCP server packages)

### Setup

1. Copy the configuration files from this gist into your project
2. Adjust the identity (`-i core-root/terraform`) and profile (`ATMOS_PROFILE=managers`) to match your environment
3. Install all MCP server packages:

```bash
atmos mcp aws install all
```

4. Test that authentication works:

```bash
atmos mcp aws test all
```

5. Start using MCP servers with Claude Code — the `.mcp.json` file handles the rest.

## Configuration Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Imports configuration from `.atmos.d/` |
| `.atmos.d/mcp.yaml` | Custom commands for `atmos mcp aws install/start/test` |
| `.atmos.d/toolchain.yaml` | Toolchain alias for `uv` package manager |
| `.atmos.d/ai.yaml` | Enables AI/MCP support in Atmos |
| `.mcp.json` | Claude Code MCP server configuration |

## Usage

```bash
# Install a specific MCP server package
atmos mcp aws install pricing

# Install all 21 AWS MCP server packages
atmos mcp aws install all

# Start a specific server with automatic AWS auth
atmos mcp aws start pricing

# Test that authentication is working
atmos mcp aws test all
```

## Available Servers

This gist includes 21 AWS MCP servers:

| Server | Package |
|--------|---------|
| billing-cost-management | awslabs.billing-cost-management-mcp-server |
| cost-explorer | awslabs.cost-explorer-mcp-server |
| pricing | awslabs.aws-pricing-mcp-server |
| terraform | awslabs.terraform-mcp-server |
| cfn | awslabs.cfn-mcp-server |
| cdk | awslabs.cdk-mcp-server |
| iac | awslabs.aws-iac-mcp-server |
| ecs | awslabs.ecs-mcp-server |
| eks | awslabs.eks-mcp-server |
| serverless | awslabs.aws-serverless-mcp-server |
| cloudwatch | awslabs.cloudwatch-mcp-server |
| cloudtrail | awslabs.cloudtrail-mcp-server |
| iam | awslabs.iam-mcp-server |
| well-architected-security | awslabs.well-architected-security-mcp-server |
| network | awslabs.aws-network-mcp-server |
| dynamodb | awslabs.dynamodb-mcp-server |
| s3-tables | awslabs.s3-tables-mcp-server |
| documentation | awslabs.aws-documentation-mcp-server |
| support | awslabs.aws-support-mcp-server |
| lambda-tool | awslabs.lambda-tool-mcp-server |
| stepfunctions-tool | awslabs.stepfunctions-tool-mcp-server |

## Customization

### Different AWS Account/Identity

Change the identity flag in `.atmos.d/mcp.yaml`:

```yaml
# Before
exec env ATMOS_PROFILE=managers atmos auth exec -i core-root/terraform -- \

# After (your identity)
exec env ATMOS_PROFILE=your-profile atmos auth exec -i your-stack/terraform -- \
```

### Different AWS Region

Update `AWS_REGION` in `.mcp.json` for each server entry:

```json
"env": { "AWS_REGION": "us-west-2" }
```

### Adding New Servers

1. Add the server name to the `ALL_SERVERS` array in the `install` command
2. Add the package name resolution logic if it follows a non-standard naming pattern
3. Add a new entry to `.mcp.json`

## The Key Insight

`atmos auth exec` is the glue that makes this work. It wraps any command with authenticated credentials using `exec`, which replaces the current process — so the MCP server inherits the credentials directly. No temp files, no environment variable juggling, no credential expiration headaches.

Combined with Custom Commands for the install/start/test workflow and Toolchain for dependency management, you get a complete, self-contained solution for running AWS MCP servers.
