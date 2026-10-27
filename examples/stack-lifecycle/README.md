---
title: Stack lifecycle
tags: [Terraform, Kubernetes]
related_docs:
  - label: Stack deployment
    url: /cli/commands/stack/stack-deploy
  - label: Lifecycle hooks
    url: /stacks/hooks
---

# Mixed component lifecycle

Preview a Terraform → Helm → Kubernetes dependency chain:

```shell
cd examples/stack-lifecycle
atmos stack plan --stack dev --dry-run
atmos stack deploy --all --stack dev --dry-run
atmos stack destroy --all --stack dev --dry-run
```

The preview shows commands, prerequisite relationships, and a conditional image
build hook. It does not run Docker, Terraform, Helm, Kubernetes, or hooks. The image
hook's condition is evaluated only during actual execution.

[Component dependencies](https://atmos.tools/stacks/dependencies/components) determine readiness. Component kinds have no fixed priority. Reversing
the dependency declarations would reverse the order; independent components can
run together with `--max-concurrency=2`. Destroy reverses the selected graph.

To calculate proposed changes, run `atmos stack plan --stack dev`. This invokes
Terraform, Helm, and Kubernetes planning operations against the available state;
it does not deploy prerequisites. Native planning requires provider and cluster
access. The `--dry-run` form above only previews commands and hooks.

Actual deployment requires Terraform/OpenTofu, Helm, kubectl, and access to a
Kubernetes cluster through `~/.kube/config`. It creates a namespace, an nginx
Deployment, and a NetworkPolicy. Each component command owns its approval behavior.
The optional hook builds a local image when `BUILD_DEMO_IMAGE=true`; it does not
publish it or change the chart's image. The build hook uses the Helm component directory, which contains its Dockerfile.

For affected selection, use a committed Git baseline:

```shell
atmos stack deploy --affected --base origin/main --stack dev --dry-run
atmos stack deploy --affected --base origin/main --include-dependencies=1 --dry-run
```

Prerequisites are not automatically executed. Explicit expansion can cross stack
boundaries. An image build belongs in a lifecycle hook; Packer `build` and other
unrelated verbs are not automatically interpreted as `deploy`.

Removed definitions are reported. Automated recovery for removed definitions is
pending the deleted-component teardown work; `--allow-destroy` fails before any
component operation when a selected deletion requires that recovery.

## Create another stack from the same scaffold engine

The `application-stack` entry under `scaffold.templates` points to a local recipe.
Preview a new environment that reuses the Terraform and Helm sources and defines
a namespace-specific Kubernetes policy inline:

```shell
atmos stack create --template application-stack --file stacks/staging.yaml \
  --set environment=staging --dry-run
```

Remove `--dry-run` to create the manifest and its independent generation record.
Creation does not deploy resources. The template declares its stack output with
`spec.stack.manifest`, and uses the standard scaffold questionnaire and file
rendering. Each environment gets its own namespace and dependency chain.
