# Mock Component

This is a mock Terraform component used exclusively for testing purposes. It provides a simple configuration with input variables and corresponding outputs, making it ideal for testing Atmos component functionality and behavior.

## Usage

This component is intended for testing only and should not be used in production environments. It is typically used in conjunction with test fixtures and validation scenarios.

Example stack usage

```yaml
components:
  terraform:
    mock:
      vars:
        foo: "default value"
        bar: "default value"
        baz: "default value"
```

## Requirements

No requirements.

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bar"></a> [bar](#input\_bar) | n/a | `string` | `"bar"` | no |
| <a name="input_baz"></a> [baz](#input\_baz) | n/a | `string` | `"baz"` | no |
| <a name="input_foo"></a> [foo](#input\_foo) | n/a | `string` | `"foo"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bar"></a> [bar](#output\_bar) | n/a |
| <a name="output_baz"></a> [baz](#output\_baz) | n/a |
| <a name="output_foo"></a> [foo](#output\_foo) | n/a |
