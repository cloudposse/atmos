# Multi-File Packer Component Test

This component tests Atmos support for directory-based Packer templates.

## Structure

```text
aws/multi-file/
├── variables.pkr.hcl  # Variable declarations
├── main.pkr.hcl       # Source and build blocks
├── manifest.json      # Pre-generated manifest for output tests
└── README.md          # This file
```

## Purpose

When users organize Packer configurations across multiple files (a recommended practice),
Atmos should pass the component directory (`.`) to Packer instead of requiring a specific
template file. This allows Packer to automatically load all `*.pkr.hcl` files.

## Usage

```bash
# Directory mode (no --template flag) - loads all *.pkr.hcl files
atmos packer validate aws/multi-file -s prod

# Explicit directory mode
atmos packer validate aws/multi-file -s prod --template .
```

## Related

- GitHub Issue: [cloudposse/atmos#1937](https://github.com/cloudposse/atmos/issues/1937)
- Packer HCL Templates: [HCL Templates](https://developer.hashicorp.com/packer/docs/templates/hcl_templates)
