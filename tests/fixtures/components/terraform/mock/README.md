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
| <a name="input_enabled"></a> [enabled](#input\_enabled) | n/a | `bool` | `true` | no |
| <a name="input_environment"></a> [environment](#input\_environment) | n/a | `string` | `""` | no |
| <a name="input_foo"></a> [foo](#input\_foo) | n/a | `string` | `"foo"` | no |
| <a name="input_message"></a> [message](#input\_message) | n/a | `string` | `""` | no |
| <a name="input_name"></a> [name](#input\_name) | n/a | `string` | `""` | no |
| <a name="input_region"></a> [region](#input\_region) | n/a | `string` | `""` | no |
| <a name="input_secret_arns_map"></a> [secret\_arns\_map](#input\_secret\_arns\_map) | Map with keys containing special characters like slashes | `map(string)` | <pre>{<br/>  "auth0-event-stream/app/client-id": "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123",<br/>  "auth0-event-stream/app/client-secret": "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-secret-xyz789"<br/>}</pre> | no |
| <a name="input_stage"></a> [stage](#input\_stage) | n/a | `string` | `"nonprod"` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | n/a | `map(string)` | `{}` | no |
| <a name="input_test_list"></a> [test\_list](#input\_test\_list) | n/a | `list(string)` | `[]` | no |
| <a name="input_test_map"></a> [test\_map](#input\_test\_map) | n/a | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bar"></a> [bar](#output\_bar) | n/a |
| <a name="output_baz"></a> [baz](#output\_baz) | n/a |
| <a name="output_foo"></a> [foo](#output\_foo) | n/a |
| <a name="output_secret_arns_map"></a> [secret\_arns\_map](#output\_secret\_arns\_map) | Map with keys containing special characters like slashes |
| <a name="output_stage"></a> [stage](#output\_stage) | n/a |
| <a name="output_tags"></a> [tags](#output\_tags) | n/a |
| <a name="output_test_list"></a> [test\_list](#output\_test\_list) | n/a |
| <a name="output_test_map"></a> [test\_map](#output\_test\_map) | n/a |
