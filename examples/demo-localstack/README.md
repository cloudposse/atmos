
## Notes

When running Terraform with LocalStack, if you get the following warning from Terraform, you should not enable `skip_requesting_account_id`. Older versions of LocalStack required this, but not anymore.

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
