
## Notes

This example provisions an S3 bucket against [Floci](https://github.com/floci-io/floci), a free, MIT-licensed AWS emulator that is a drop-in replacement for LocalStack Community Edition (which was EOL'd in March 2026). Floci listens on the same `4566` edge port and accepts the same `test`/`test` credentials.

When running Terraform with Floci, this example uses path-style S3 on `http://localhost:4566` and enables `skip_requesting_account_id` so the AWS provider does not block on account identity lookups or wildcard DNS/TLS resolution in CI. Without these settings, Terraform can hang before printing the first plan when emulator-backed provider setup is slow or incomplete.

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

Start and stop the emulator with the demo's custom commands:

```shell
atmos floci up       # start Floci in the background
atmos floci status   # show emulator status
atmos floci reset    # wipe all emulator state
atmos floci down     # stop Floci
```
