# Adopt an existing Helm chart

**Status** — Single local chart registration and ordered values files implemented on this branch. The [existing example](../../../examples/helm/README.md) already renders a local chart without cluster credentials. Uses the [shared adoption contract](README.md).

## Outcome

An engineer who can run Helm against a local chart can reproduce its rendered resources through Atmos and save the release as a component. The first success is a recognizable Kubernetes resource rendered with their own values.

```shell
# Existing workflow from the project root
helm template web ./charts/web --namespace preview -f values/base.yaml -f values/dev.yaml

# Register and render the same release through Atmos
atmos stack add ./charts/web -s dev --release-name web --namespace preview \
  --values ./values/base.yaml --values ./values/dev.yaml
atmos helm template web -s dev
```

## Local chart registration

`atmos stack add` accepts an unpacked local application chart with its dependencies already present. It records the chart directory, release name, namespace and ordered values file paths. Registration and dry-run preview read the chart and validate every overlay without contacting a cluster or fetching dependencies.

| Input | Behavior |
| --- | --- |
| Two or more values files | `-f` and `--values` accept repeated paths relative to the caller’s directory. Saving makes each reference relative to the chart. Later files take precedence in the default merge configuration. |
| Multiple YAML documents in one values file | Helm’s values loader merges the documents in order before Atmos applies the next file and any inline `values`. CLI overrides still apply last. |
| Missing dependency | Registration names the missing chart and suggests the native `helm dependency build` command. |
| Install-only CRDs | Atmos includes these resources in a named render. A chart supplying them needs separate handling to reproduce the customary `helm template` output, so registration reports the CRD path. |
| Render flags and remote sources | Persisting `--set*`, custom capabilities, post-renderers and remote chart sources requires further parity tests. The first release registers a local chart with file inputs. |

Multi-document overlays follow Helm 4 semantics. Helm 3 reads only the first document passed to a single `-f`. For matching results on Helm 3 and Atmos, keep each document in its own file and supply those files in order.

The existing renderer continues to use the project’s configured list-merge strategy across files and inline values. The CLI acceptance test compares all resource fields for two releases whose ordered overlays set different image tags and Service ports. [CLI parity test](../../../tests/cli_helm_adoption_test.go) · [Saved values tests](../../../pkg/component/helm/values_documents_test.go)

## Working with the saved stack

For the chart at `charts/web`, a new project's `atmos.yaml` sets `components.helm.base_path` to `charts` and includes the stack manifests under `stacks/`. The stack entry keeps the release identity and values files together.

```yaml
# stacks/dev.yaml
components:
  helm:
    web:
      metadata:
        component: web
      chart: "."
      name: web
      namespace: preview
      values_files:
        - ../../values/base.yaml
        - ../../values/dev.yaml
```

The component directory is `charts/web`. Helm values paths in the saved entry resolve from that directory, so the two paths above refer to the same files supplied to the registration command from the repository root. Run `atmos stack instances -s dev`, then `atmos helm template web -s dev` to inspect just this release.

Register a second release of the same chart in the same Atmos stack with a different release name.

```shell
atmos stack add ./charts/web -s dev --release-name web-canary --namespace preview \
  -f ./values/base.yaml -f ./values/canary.yaml
atmos helm template web-canary -s dev
```

Use `--name` to give the Atmos component its own name. When both names are omitted, Atmos uses the chart directory’s basename. A new stack such as `prod` gets its own release entry and values paths in `stacks/prod.yaml`. The namespace comes from `--namespace` or Helm’s local namespace configuration, which falls back to `default`. The namespace and cluster are independent of the Atmos stack name.

## Requirements

- Detect a selected local chart by its Chart.yaml. For a directory with several charts, offer project-owned chart roots. Treat packaged dependencies under charts/ as dependencies unless explicitly selected.
- Preserve chart location, release name, namespace, ordered values files and existing chart defaults. For Helm flags, resolve explicit values paths relative to the caller's directory as in the native example. Save relocatable references. Verify the effective overlays against native Helm for the supported input forms.
- Keep the Atmos stack name independent of the release and namespace. If an existing command or project definition provides those values, show their source. Ask for a missing release identity when necessary. Multiple named releases of one chart may coexist.
- Offer an offline render when all required chart inputs are available. Identify missing dependencies, external values or scripts affecting output. Cluster access and release installation are later operations.
- First delivery covers a local chart and values files. Repository or OCI chart resolution, post-renderers and environment-dependent wrappers require independent parity fixtures before being labeled reproducible.

## Acceptance

Compare the normalized Kubernetes manifests from Helm and Atmos for a local chart with two values files and an explicit namespace. Verify resource names, namespace and overridden values. Reverse the file order and verify the same change in both renders. Repeat with a second release name, a missing dependency, a conflicting component and an idempotent re-import. Observe a first-time user identifying the expected Service without a cluster or a manifest edit.
