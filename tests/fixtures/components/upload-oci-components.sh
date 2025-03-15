
#!/bin/bash


# In order to manage packages, the following scopes are required:
# - read:packages
# - write:packages
# - delete:packages
# To refresh the token, run:
#
#   gh auth refresh -h github.com -s write:packages -s read:packages -s delete:packages
#


if [ -z "$GITHUB_TOKEN" ]; then
	echo "GITHUB_TOKEN is not set, using gh auth token"
	export GITHUB_TOKEN=$(gh auth token)
fi

# NOTE:
#   Helm uses:      application/vnd.cncf.helm.chart.content.v1.tar+gzip.
#   Atmos will use: application/vnd.atmos.component.terraform.v1+tar+gzip

tar -czf mock-component.tar.gz -C terraform/mock .
# Push the artifacts to OCI registry
echo "$GITHUB_TOKEN" | oras push ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0 \
	--password-stdin \
	--username oauth \
  --artifact-type application/vnd.atmos.component.terraform.v1+tar+gzip \
  --annotation org.opencontainers.image.title="Example OCI Component: Mock" \
  --annotation org.opencontainers.image.description="Atmos Terraform component for OCI testing" \
  --annotation org.opencontainers.image.version="0.0.0" \
	--annotation org.opencontainers.image.source=https://github.com/cloudposse/atmos \
  --annotation atmos.component.kind="terraform" \
  mock-component.tar.gz

tar -czf myapp-component.tar.gz -C terraform/myapp .
echo "$GITHUB_TOKEN" | oras push ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/myapp:v0 \
	--password-stdin \
	--username oauth \
  --artifact-type application/vnd.atmos.component.terraform.v1+tar+gzip \
  --annotation org.opencontainers.image.title="Example OCI Component: MyApp" \
  --annotation org.opencontainers.image.description="Atmos Terraform component for OCI testing" \
  --annotation org.opencontainers.image.version="0.0.0" \
	--annotation org.opencontainers.image.source=https://github.com/cloudposse/atmos \
  --annotation atmos.component.kind="terraform" \
  myapp-component.tar.gz

# delete
# gh api --method DELETE --input /dev/null "/orgs/cloudposse/packages/container/atmos%2Fmock-component"
