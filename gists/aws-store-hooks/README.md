# AWS Store Hooks with SSM and Secrets Manager

This gist validates the full store-output loop:

1. Terraform emits outputs.
2. Atmos `kind: store` hooks write those outputs to AWS SSM Parameter Store and AWS Secrets Manager.
3. Other components read the values back with `!store`, `!store.get`, and `atmos.Store`.

The Terraform component creates no AWS resources. Only the Atmos store hooks write to AWS.
The gist runs Terraform through OpenTofu (`command: tofu`) and declares `opentofu`
as a component tool dependency, so Atmos installs it automatically if it is not
already on `PATH`.

## Setup

1. Replace `REGION` and `ACCOUNT_ID` in `iam/store-writer-policy.json`.
2. Attach the policy to the AWS identity you will use for Atmos.
3. Edit `atmos.yaml` if you want a region other than `us-east-1`.
4. Authenticate with AWS or configure an Atmos auth identity.

The stores use slash notation:

```yaml
stores:
  outputs/ssm:
    kind: aws/ssm
  outputs/asm:
    kind: aws/asm
```

## Run

```bash
cd gists/aws-store-hooks

atmos terraform apply output-demo -s producer
atmos describe component reader -s producer
atmos describe component reader -s consumer
atmos terraform plan reader -s consumer
```

## Verify in AWS

```bash
aws ssm get-parameter \
  --region us-east-1 \
  --name /atmos-gist/store-hooks/producer/output-demo/demo_id

aws secretsmanager get-secret-value \
  --region us-east-1 \
  --secret-id atmos-gist/store-hooks/producer/output-demo/demo_id
```

## Optional Floci E2E

For automated AWS-compatible E2E, use the repository test fixture and Floci
harness. This gist is the manual runnable example; tests should not depend on
`gists/`.

```bash
ATMOS_TEST_FLOCI=true FLOCI_ENDPOINT_URL=http://localhost:4566 go test ./tests -run Floci
```

Do not use LocalStack for this workflow.

## Cleanup

```bash
aws ssm delete-parameter --region us-east-1 \
  --name /atmos-gist/store-hooks/producer/output-demo/demo_id

aws ssm delete-parameter --region us-east-1 \
  --name /atmos-gist/store-hooks/producer/output-demo/structured_config

aws ssm delete-parameter --region us-east-1 \
  --name /atmos-gist/store-hooks/producer/output-demo/secret_like_value

aws secretsmanager delete-secret --region us-east-1 \
  --secret-id atmos-gist/store-hooks/producer/output-demo/demo_id \
  --force-delete-without-recovery

aws secretsmanager delete-secret --region us-east-1 \
  --secret-id atmos-gist/store-hooks/producer/output-demo/structured_config \
  --force-delete-without-recovery

aws secretsmanager delete-secret --region us-east-1 \
  --secret-id atmos-gist/store-hooks/producer/output-demo/secret_like_value \
  --force-delete-without-recovery
```
