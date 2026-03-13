# Project Instructions

This file provides persistent context to the Atmos AI Assistant about this project.

## Project Overview

This is a multi-region AWS infrastructure project demonstrating Atmos AI features. It contains:
- VPC networking components across two AWS regions
- Transit Gateway hub-spoke topology for cross-region connectivity
- Network and production stacks per region

## Architecture

- **Hub-spoke networking** — us-east-1 hosts the Transit Gateway hub; us-west-2 connects via cross-region peering
- **Two regions** — us-east-1 (primary) and us-west-2 (secondary)
- **Two stages** — `network` (shared networking) and `prod` (production workloads)

## Stack Naming Convention

Stacks follow the pattern: `{environment}-{stage}`
- `ue1-network` — Network stack in us-east-1
- `ue1-prod` — Production stack in us-east-1
- `uw2-network` — Network stack in us-west-2
- `uw2-prod` — Production stack in us-west-2

## Components

### vpc
Virtual Private Cloud with CIDR allocation, availability zones, and NAT Gateways.
- Network stacks use `10.1.0.0/16` (ue1) and `10.2.0.0/16` (uw2)
- Production stacks use `10.10.0.0/16` (ue1) and `10.20.0.0/16` (uw2)

### tgw/hub
Transit Gateway hub in us-east-1 network stack. Central routing for cross-region and cross-account connectivity.

### tgw/attachment
Transit Gateway VPC attachment. Connects a VPC to the Transit Gateway. Present in all stacks.

### tgw/cross-region-hub-connector
Cross-region Transit Gateway peering. Only in uw2-network, connects us-west-2 to the us-east-1 hub.

## Component Dependencies

- `tgw/hub` depends on `vpc` in the same stack
- `tgw/attachment` depends on `vpc` (same stack) and `tgw/hub` (network stack)
- `tgw/cross-region-hub-connector` depends on `tgw/hub` in ue1-network

## Common Operations

### Describe a component
```bash
atmos describe component vpc -s ue1-network
```

### List all stacks
```bash
atmos list stacks
```

### Validate configuration
```bash
atmos validate stacks
```

### Check affected stacks
```bash
atmos describe affected
```

## AI-Powered Analysis with `--ai`

The global `--ai` flag adds AI analysis to any command. The AI summarizes output on success and
explains errors with fix instructions on failure.

### Analyze command output
```bash
atmos terraform plan vpc -s ue1-network --ai
atmos describe component vpc -s ue1-prod --ai
atmos validate stacks --ai
atmos list stacks --ai
```

### With domain-specific skills
```bash
# Single skill
atmos terraform plan vpc -s ue1-network --ai --skill atmos-terraform

# Multiple skills (comma-separated)
atmos terraform plan vpc -s ue1-prod --ai --skill atmos-terraform,atmos-stacks

# Multiple skills (repeated flag)
atmos terraform plan vpc -s ue1-prod --ai --skill atmos-terraform --skill atmos-stacks
```

### Enable globally via environment variable
```bash
export ATMOS_AI=true
atmos terraform plan vpc -s ue1-network

# With skills via env var
ATMOS_AI=true ATMOS_SKILL=atmos-terraform,atmos-stacks atmos terraform plan vpc -s ue1-prod
```
