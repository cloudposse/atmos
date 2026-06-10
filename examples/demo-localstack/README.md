
## Notes

When running Terraform with LocalStack, this example uses path-style S3 on `http://localhost:4566` and enables `skip_requesting_account_id` so the AWS provider does not block on account identity lookups or wildcard DNS/TLS resolution in CI. Without these settings, Terraform can hang before printing the first plan when LocalStack-backed provider setup is slow or incomplete.

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
