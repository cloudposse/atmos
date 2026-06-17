# AWS Declared Secrets with SSM and Secrets Manager

This gist validates declared AWS secrets without Terraform output hooks.
The gist runs Terraform through OpenTofu (`command: tofu`) and declares `opentofu`
as a component tool dependency, so Atmos installs it automatically if it is not
already on `PATH`.

It covers:

- `kind: aws/ssm` with `secret: true` writing SSM `SecureString`
- `kind: aws/asm` with `secret: true`
- `atmos secret set/get/list/validate/delete`
- `scope: instance`, `scope: stack`, and `scope: global`
- `!secret` masking during inspection
- passing secrets to Terraform through `env` as `TF_VAR_*`

## Setup

1. Replace `REGION` and `ACCOUNT_ID` in `iam/secrets-policy.json`.
2. Attach the policy to the AWS identity you will use for Atmos.
3. Edit `atmos.yaml` if you want a region other than `us-east-1`.
4. Authenticate with AWS or configure an Atmos auth identity.

## Initialize Values

```bash
cd gists/aws-secrets

atmos secret set SSM_INSTANCE_TOKEN=dev-instance-token -s dev -c secret-consumer --force
atmos secret set GLOBAL_SHARED_TOKEN=shared-token -s dev -c secret-consumer --force

aws ssm put-parameter \
  --region us-east-1 \
  --name /atmos-gist/secrets/dev/SSM_STACK_TOKEN \
  --type SecureString \
  --overwrite \
  --value dev-stack-token

aws secretsmanager create-secret \
  --region us-east-1 \
  --name atmos-gist/secrets/dev/secret-consumer/ASM_DATABASE_CONFIG \
  --secret-string '{"username":"demo","password":"dev-db-password","host":"db.dev.local"}'
```

Initialize `prod` to prove stack and instance scopes are independent while `GLOBAL_SHARED_TOKEN` is shared:

```bash
atmos secret set SSM_INSTANCE_TOKEN=prod-instance-token -s prod -c secret-consumer --force

aws ssm put-parameter \
  --region us-east-1 \
  --name /atmos-gist/secrets/prod/SSM_STACK_TOKEN \
  --type SecureString \
  --overwrite \
  --value prod-stack-token

aws secretsmanager create-secret \
  --region us-east-1 \
  --name atmos-gist/secrets/prod/secret-consumer/ASM_DATABASE_CONFIG \
  --secret-string '{"username":"demo","password":"prod-db-password","host":"db.prod.local"}'
```

## Inspect and Consume

```bash
atmos secret list -s dev -c secret-consumer
atmos secret validate -s dev -c secret-consumer

# Masked by default.
atmos describe component secret-consumer -s dev

# Reveals values and requires backend access.
atmos describe component secret-consumer -s dev --mask=false

atmos secret get ASM_DATABASE_CONFIG -s dev -c secret-consumer --path '.password' --mask=false
atmos terraform plan secret-consumer -s dev
```

## Verify in AWS

```bash
aws ssm get-parameter \
  --region us-east-1 \
  --name /atmos-gist/secrets/dev/secret-consumer/SSM_INSTANCE_TOKEN \
  --with-decryption

aws ssm get-parameter \
  --region us-east-1 \
  --name /atmos-gist/secrets/dev/SSM_STACK_TOKEN \
  --with-decryption

aws ssm get-parameter \
  --region us-east-1 \
  --name /atmos-gist/secrets/GLOBAL_SHARED_TOKEN \
  --with-decryption

aws secretsmanager get-secret-value \
  --region us-east-1 \
  --secret-id atmos-gist/secrets/dev/secret-consumer/ASM_DATABASE_CONFIG
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
atmos secret delete SSM_INSTANCE_TOKEN -s dev -c secret-consumer --force
atmos secret delete SSM_STACK_TOKEN -s dev -c secret-consumer --force
atmos secret delete GLOBAL_SHARED_TOKEN -s dev -c secret-consumer --force
atmos secret delete ASM_DATABASE_CONFIG -s dev -c secret-consumer --force

atmos secret delete SSM_INSTANCE_TOKEN -s prod -c secret-consumer --force
atmos secret delete SSM_STACK_TOKEN -s prod -c secret-consumer --force
atmos secret delete ASM_DATABASE_CONFIG -s prod -c secret-consumer --force
```
