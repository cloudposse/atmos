---
title: CloudFormation Build and Publish Pipeline
tags: [Components, Hooks, Automation, Emulators]
cast:
  file: /casts/examples/cloudformation-advanced/lifecycle.cast
  title: CloudFormation build, publish, and deploy lifecycle
related_docs:
  - label: CloudFormation Components
    url: /stacks/components/aws-cloudformation
  - label: Publish Step
    url: /steps/type/publish
  - label: HTTP Step
    url: /steps/type/http
---

# CloudFormation Build and Publish Pipeline

Build, test, archive, and publish a Python Lambda application, deploy it through
CloudFormation, and validate its HTTP response. Everything runs locally in Floci;
no AWS account or credentials are required.

## Prerequisites

- Atmos with the `publish` step.
- Docker, running and accessible to Atmos.
- Python 3 and the AWS CLI.

Floci uses the host container runtime to execute the actual uploaded Lambda code.
This example pins Floci 2.2.0 for CloudFormation function URL support and disables
fallbacks for missing Lambda code and unsupported CloudFormation resources.
The first run downloads the emulator and Python Lambda runtime images. This
example is tested with Docker; rootful Podman requires equivalent host runtime
access and is not covered by the example test.

## Run

From this directory:

```shell
atmos test
```

The test starts Floci and creates an artifact bucket. It then runs three deployments:

| Deployment | Archive | Publish | HTTP validation |
| --- | --- | --- | --- |
| Release `v1` | Build and test application | Upload new archive | Response contains `v1` |
| Release `v1` again | Reproduce identical bytes | Skip unchanged object | Response still contains `v1` |
| Release `v2` | Build and test changed release | Upload to a new release key | Response contains `v2` |

The test also downloads each published archive and compares it with the local
build. It deletes the stacks, empties the bucket, and stops Floci, including when
a build, deployment, or validation fails.

## Pipeline

The app's `before.aws/cloudformation.apply` hook declares the pipeline in
[`stacks/catalog/app.yaml`](stacks/catalog/app.yaml):

1. Build the package and generate its release module.
2. Run unit tests against that package.
3. Create a reproducible ZIP with the native `archive` step.
4. Upload it with the native `publish` step using the named `artifacts` target.

CloudFormation receives the bucket and release-specific object key as parameters.
The template's `Code.S3Bucket` and `Code.S3Key` reference those parameters directly;
publishing does not rewrite templates. Changing the key makes the code update
visible to CloudFormation.

After deployment, the `validate` hook reads the function URL and uses the native
`http` step to require HTTP 200 and the expected message and release. A matching
stack status alone does not pass the test. Floci advertises function URLs on its
internal port; `function_url.py` routes that URL through the emulator's published
host port using Floci's `/lambda-url/` endpoint.

[`scripts/test.py`](scripts/test.py) coordinates deployments, checks incremental
publishing, and attempts every cleanup even if an earlier operation fails. The
build and publish steps remain in the component hooks so ordinary
`atmos aws cfn deploy app -s advanced` runs them too.

## Learn More

- [CloudFormation components](https://atmos.tools/stacks/components/aws-cloudformation/)
- [Publish step](https://atmos.tools/steps/type/publish/)
- [HTTP step](https://atmos.tools/steps/type/http/)
- [Basic CloudFormation example](../cloudformation/)
