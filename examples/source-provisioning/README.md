# Source Provisioning

This example demonstrates **source provisioning** - inline source declaration for Just-in-Time (JIT) component vendoring.

## Usage

```bash
cd examples/source-provisioning

# Describe source configuration
atmos terraform source describe myapp --stack dev

# Pull (vendor) the component source
atmos terraform source pull myapp --stack dev

# Run terraform (source is auto-provisioned if missing)
atmos terraform plan myapp --stack dev
```

## Configuration

The `source` field in `stacks/deploy/dev.yaml` specifies where to vendor the component from:

```yaml
components:
  terraform:
    myapp:
      source:
        uri: "github.com/cloudposse/terraform-null-label?ref=0.25.0"
      provision:
        workdir:
          enabled: true
      vars:
        enabled: true
```

## Cleanup

```bash
rm -rf components/ .workdir/
```
