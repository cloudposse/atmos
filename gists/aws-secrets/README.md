# AWS Declared Secrets

Runnable example for declared secrets backed by AWS SSM Parameter Store and AWS
Secrets Manager.

## Covers

- `kind: aws/ssm` with `secret: true`
- `kind: aws/asm` with `secret: true`
- `atmos secret set/get/list/validate/delete`
- instance, stack, and global scopes
- `!secret` consumption and masked describe output
- automatic Terraform `TF_VAR_*` injection for secret-bearing `vars`

## Setup

1. Replace `REGION` and `ACCOUNT_ID` in `iam/secrets-policy.json`.
2. Attach the policy to the AWS identity used by Atmos.
3. Configure AWS credentials or an Atmos auth identity.

## Set Values

```bash
cd gists/aws-secrets

atmos secret set SSM_INSTANCE_TOKEN=dev-instance-token -s dev -c secret-consumer --force
atmos secret set SSM_STACK_TOKEN=dev-stack-token -s dev -c secret-consumer --force
atmos secret set GLOBAL_SHARED_TOKEN=shared-token -s dev -c secret-consumer --force
atmos secret set 'ASM_DATABASE_CONFIG={"username":"demo","password":"dev-db-password","host":"db.dev.local"}' -s dev -c secret-consumer --force

atmos secret set SSM_INSTANCE_TOKEN=prod-instance-token -s prod -c secret-consumer --force
atmos secret set SSM_STACK_TOKEN=prod-stack-token -s prod -c secret-consumer --force
atmos secret set 'ASM_DATABASE_CONFIG={"username":"demo","password":"prod-db-password","host":"db.prod.local"}' -s prod -c secret-consumer --force
```

## Use Values

```bash
atmos secret list -s dev -c secret-consumer
atmos secret validate -s dev -c secret-consumer
atmos secret get ASM_DATABASE_CONFIG -s dev -c secret-consumer --path '.password' --mask=false

atmos describe component secret-consumer -s dev
atmos terraform plan secret-consumer -s dev
```

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
