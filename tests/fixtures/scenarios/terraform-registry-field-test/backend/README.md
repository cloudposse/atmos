# Terraform registry backend field-test fixture

Derived from `terraform-component-mocks-fallback` and `aws-store-hooks-floci`.
Requires an isolated AWS emulator and dummy credentials. Set `FIELD_ENGINE`,
`FIELD_BUCKET`, `FIELD_STORE_PREFIX`, and `AWS_ENDPOINT_URL`. Run a copied fixture.
The fixture uses real `terraform_data` state in S3, distinct dev/prod workspaces,
state/output references, fallback mocks, an AWS user identity, and an SSM output hook.

The field pass also copies `modules/source-marker` into a source-provisioned variant:
`source: {uri: ./modules/source-marker, ttl: 1h}` plus
`provision.workdir.enabled: true`. That variant uses null provider 3.2.4 from an
isolated filesystem mirror and an absolute `components.terraform.plugin_cache_dir`.
Refresh source explicitly with `terraform source pull producer -s dev --force`.

## Reproduce dry-run backend mutation

Use a new unique bucket name for each repetition. With a copied fixture configured
against the disposable emulator:

```shell
aws s3api head-bucket --bucket "$FIELD_BUCKET"
atmos terraform plan producer -s dev --dry-run
aws s3api head-bucket --bucket "$FIELD_BUCKET"
```

The first command returns 404. Both tested revisions create the bucket during
`plan --dry-run`, print `Provisioned S3 backend`, and return 0. The final command
returns 0. This behavior is inherited, not introduced by the registry extraction.

## Reproduce source refresh losing a saved plan

In the source variant, change `source.ttl` to `0s`, then run:

```shell
atmos terraform plan producer -s dev -detailed-exitcode
atmos terraform apply producer -s dev --from-plan
```

Both tested revisions return 2 from plan and 1 from apply, reporting a missing
`dev-producer.planfile` after source refresh. TTL `1h` preserves the saved plan;
force-refreshing source before a new plan preserves the apply workflow.

Manual backend CRUD currently requires literal bucket/endpoint values: its host
lookup disables YAML function processing. This is separate from lifecycle commands,
which resolve the `!env` values correctly. Do not use real cloud credentials.
