
## Notes

When running Terraform with LocalStack, this example enables `skip_requesting_account_id` so the AWS provider does not block on account identity lookups in CI. Without it, Terraform can hang before printing the first plan when the LocalStack-backed identity request is slow or incomplete.

The following Terraform warning is expected with this setting:

```console
Warning: AWS account ID not found for provider
│
│   with provider["registry.terraform.io/hashicorp/aws"],
│   on providers.tf line 1, in provider "aws":
│    1: provider "aws" {
│
│ See
│ https://registry.terraform.io/providers/hashicorp/aws/latest/docs#skip_requesting_account_id
│ for implications.
```
