---
slug: azure-blob-terraform-state-support
title: Azure Blob Storage Support for !terraform.state Function
sidebar_label: Azure Blob Storage for !terraform.state
authors:
  - jamengual
tags:
  - feature
date: 2025-10-21T00:00:00.000Z
release: v1.196.0
---

Atmos now supports Azure Blob Storage backends in the `!terraform.state` YAML function. Read Terraform outputs directly from Azure-backed state files without initializing Terraform—bringing the same blazing-fast performance to Azure that S3 users already enjoy.

<!--truncate-->

## What's New

The `!terraform.state` YAML function now supports **Azure Blob Storage (azurerm)** backends, joining existing support for S3 and local backends. This means you can retrieve Terraform outputs from Azure-backed state files at lightning speed—without the overhead of Terraform initialization.

### Why This Matters

Before this feature, if you were using Azure Blob Storage as your Terraform backend, you had two options for reading remote state:

1. **`!terraform.output`** - Slow but reliable. Requires full Terraform initialization, provider downloads, and varfile generation.
2. **`!store`** - Fast but requires extra setup. You had to manually configure external secret stores.

Now you can use **`!terraform.state`** with Azure backends—getting **10-100x faster performance** compared to `!terraform.output` by reading directly from blob storage.

## How It Works

### Backend Configuration

Configure your Terraform component with an `azurerm` backend:

```yaml
components:
  terraform:
    vpc:
      backend_type: azurerm
      backend:
        azurerm:
          storage_account_name: "mystorageaccount"
          container_name: "tfstate"
          key: "vpc.terraform.tfstate"
```

### Reading State

Use the `!terraform.state` function to read outputs:

```yaml
components:
  terraform:
    eks-cluster:
      vars:
        # Get vpc_id output from vpc component in current stack
        vpc_id: !terraform.state vpc vpc_id

        # Get private subnet IDs
        subnet_ids: !terraform.state vpc private_subnet_ids

        # Get first subnet using YQ expression
        subnet_id: !terraform.state vpc .private_subnet_ids[0]
```

### Cross-Stack References

Reference components from different stacks:

```yaml
components:
  terraform:
    tgw:
      vars:
        # Get VPC ID from production stack
        vpc_id: !terraform.state vpc plat-ue2-prod vpc_id

        # Use template for dynamic stack names
        vpc_id: !terraform.state vpc {{ printf "net-%s-%s" .vars.environment .vars.stage }} vpc_id
```

## Authentication

The Azure Blob Storage integration uses **Azure DefaultAzureCredential**, which supports multiple authentication methods automatically:

1. **Environment variables** - `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`
2. **Managed Identity** - When running in Azure (AKS, VMs, Functions)
3. **Azure CLI credentials** - `az login`
4. **Visual Studio Code credentials** - Authenticated VS Code sessions

No additional configuration needed—just authenticate using your preferred method.

## Workspace Handling

Azure Blob Storage uses a specific naming convention for workspaces:

- **Default workspace**: Uses the key as-is (e.g., `terraform.tfstate`)
- **Non-default workspaces**: Appends workspace as suffix (e.g., `terraform.tfstateenv:dev`)

Atmos handles this automatically—you don't need to worry about the naming convention.

### Example

If you have:

- Key: `apimanagement.terraform.tfstate`
- Workspace: `dev-wus3-apimanagement-be`

Atmos will look for: `apimanagement.terraform.tfstateenv:dev-wus3-apimanagement-be`

## Performance Benefits

### Before: Using `!terraform.output`

```bash
$ time atmos terraform plan eks-cluster -s plat-ue2-dev

# Must initialize Terraform for each dependency
Initializing vpc component...
Downloading providers...
Generating backend config...
Generating varfiles...
Reading outputs...

real    2m34.521s
```

### After: Using `!terraform.state`

```bash
$ time atmos terraform plan eks-cluster -s plat-ue2-dev

# Direct blob storage access
Reading state from Azure Blob Storage...

real    0m3.142s
```

**~50x faster** in this example—and the speedup grows with infrastructure complexity.

## Advanced Features

### YQ Expressions

Use YQ expressions for complex data extraction:

```yaml
vars:
  # Get nested map values
  db_endpoint: !terraform.state database .config_map.endpoint

  # String concatenation
  jdbc_url: !terraform.state postgres ".master_hostname | \"jdbc:postgresql://\" + . + \":5432/events\""

  # Default values for unprovisioned components
  username: !terraform.state config ".username // \"default-user\""
```

### Caching

Results are cached in memory per CLI execution:

```yaml
vars:
  # All three calls use the same cached result
  sg_id_1: !terraform.state vpc security_group_id
  sg_id_2: !terraform.state vpc security_group_id
  sg_id_3: !terraform.state vpc {{ .stack }} security_group_id
```

The first call reads from Azure; subsequent calls return cached data instantly.

### Error Handling

- **Blob not found (404)**: Returns `null` (component not provisioned yet)
- **Permission denied (403)**: Returns clear error message
- **Network errors**: Automatically retries up to 2 times with exponential backoff

## Technical Details

### Implementation Highlights

- **Azure SDK for Go** - Uses official `github.com/Azure/azure-sdk-for-go/sdk/storage/azblob` package
- **Client caching** - Azure Blob clients are cached per storage account/container
- **Retry logic** - Automatic retry with exponential backoff for transient failures
- **Nil safety** - Robust error handling prevents panics
- **Test coverage** - Comprehensive unit tests with mocked Azure SDK
- **Cross-platform** - Works on Linux, macOS, and Windows

### Backend Configuration Options

All standard Azure backend options are supported:

```yaml
backend:
  azurerm:
    storage_account_name: "mystorageaccount"  # Required
    container_name: "tfstate"                  # Required
    key: "terraform.tfstate"                   # Optional (default: terraform.tfstate)
    # Authentication happens via DefaultAzureCredential
```

## Migration Guide

### From `!terraform.output` to `!terraform.state`

The syntax is identical—just replace `!terraform.output` with `!terraform.state`:

```yaml
# Before
vpc_id: !terraform.output vpc vpc_id

# After
vpc_id: !terraform.state vpc vpc_id
```

### From `!store` to `!terraform.state`

Simplify your configuration by removing store setup:

```yaml
# Before: Required store configuration
vpc_id: !store azurekeyvault plat-ue2-dev vpc vpc_id

# After: Direct state access
vpc_id: !terraform.state vpc vpc_id
```

## Examples

### Basic Usage

```yaml
components:
  terraform:
    app:
      vars:
        # String output
        security_group_id: !terraform.state security-group id

        # List output
        subnet_ids: !terraform.state vpc private_subnet_ids

        # Map output
        config: !terraform.state config config_map
```

### Cross-Region References

```yaml
components:
  terraform:
    replication:
      vars:
        # Reference component from different region
        primary_db: !terraform.state database {{ printf "%s-use1-%s" .vars.tenant .vars.stage }} endpoint
```

### Disaster Recovery Scenarios

```yaml
components:
  terraform:
    failover:
      vars:
        # Primary region
        primary_vpc: !terraform.state vpc plat-ue2-prod vpc_id

        # DR region with default fallback
        dr_vpc: !terraform.state vpc plat-uw2-prod ".vpc_id // \"vpc-mock-dr\""
```

## Considerations

- **Secrets exposure**: Using `!terraform.state` with secrets will expose them in `atmos describe` output
- **Permission scoping**: Ensure your Azure credentials have access to all referenced storage accounts
- **Cross-region access**: Consider latency when reading state across regions
- **Cold starts**: Components not yet provisioned return `null` (use YQ default values to handle this)

## Try It Now

Upgrade to the latest Atmos release and start using Azure Blob Storage backends:

```bash
# Check your version
atmos version

# Describe a component using Azure backend
atmos describe component vpc -s plat-ue2-dev

# Use !terraform.state in your stack configs
# (See examples above)
```

## Documentation

- **[!terraform.state Function Reference](/functions/yaml/terraform.state)** - Complete usage documentation
- **[Terraform Backends](/core-concepts/components/terraform/backends)** - Backend configuration guide
- **[Remote State](/core-concepts/share-data/remote-state)** - Data sharing patterns

## Get Involved

We're building Atmos in the open and welcome your feedback:

- 💬 **Discuss** - Share thoughts in [GitHub Discussions](https://github.com/orgs/cloudposse/discussions).
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues).
- 🚀 **Contribute** - Want to add features? Review our [contribution guide](https://atmos.tools/community/contributing).

---

**Next up**: Google Cloud Storage (GCS) backend support for `!terraform.state`. Stay tuned!
