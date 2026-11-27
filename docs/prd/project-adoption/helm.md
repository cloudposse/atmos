# Adopt an existing Helm chart

**Status** — Proposed own-project import. The [existing example](../../../examples/helm/README.md) already renders a local chart without cluster credentials. Uses the [shared adoption contract](README.md).

## Outcome

An engineer who can run Helm against a local chart can reproduce its rendered resources through Atmos and save the release as a component. The first success is a recognizable Kubernetes resource rendered with their own values.

```shell
# Existing workflow from the project root
helm template web ./charts/web --namespace preview -f values/base.yaml -f values/dev.yaml

# Proposed registration, followed by the existing render command
atmos stack add ./charts/web -s dev --release-name web --namespace preview \
  --values ./values/base.yaml --values ./values/dev.yaml
atmos helm template web -s dev
```

## What can ship first

Register one unpacked chart with its local dependencies already available, its release name, namespace and zero or more local, single-document values files. Save their paths relative to the chart and render offline. Existing fields cover this case. The user should recognize the same Service, Deployment and image before choosing a cluster or installing a release.

| Input | Evidence from the renderer | Import decision |
| --- | --- | --- |
| Local chart and two single-document files | Comparing Helm 4.2.3 with Atmos using the same demo chart, release, namespace and files produced matching parsed resources, including the later port and image overrides. | First candidate. Compare rendered objects keyed by kind, namespace and name, including lists, null overrides and the project's configured list-merge behavior. |
| Multiple YAML documents in a values file | With port 82 in the first document and image tag `1.27` in the second, native Helm used both. The saved Atmos entry applied port 82 but retained image tag `1.24` from the earlier file. | Report the file and document boundary during preview until saved-file loading passes the native fixtures. Reuse Helm's values loader while retaining existing inline-value precedence and testing compatibility. |
| Chart with a `crds/` directory | Atmos sets `IncludeCRDs` when constructing the chart spec. Native template output depends on `--include-crds`. | Compare both native flag cases. Admit a chart only when the selected render has the same resource set. |
| `--set*`, capabilities, post-renderers or remote chart dependencies | CLI override handling already uses Helm's parser. Persistence would have to preserve types, file references, render inputs and tool versions across future runs. | Show the unrepresented argument or dependency. Defer a saved equivalent until a fixture exercises that feature. |

The renderer uses Atmos merge for saved `values_files` and Helm's loader for command-line overrides. Registration must account for inherited inline `values`, which win over saved files. Keep an ordinary single-document chart eligible while adding coverage for other cases. Offline render parity establishes no cluster identity or upgrade behavior. [Values implementation](../../../pkg/component/helm/values.go) · [Render implementation](../../../pkg/component/helm/chart.go) · [Native template options](https://helm.sh/docs/helm/helm_template/)

## Working with the saved stack

For the chart at `charts/web`, a new project's `atmos.yaml` sets `components.helm.base_path` to `charts` and includes the stack manifests under `stacks/`. The proposed importer writes the following entry using existing Helm component fields:

```yaml
# stacks/dev.yaml
components:
  helm:
    web:
      chart: "."
      name: web
      namespace: preview
      values_files:
        - ../../values/base.yaml
        - ../../values/dev.yaml
```

The component directory is `charts/web`. Helm values paths in the saved entry resolve from that directory, so the two paths above refer to the same files supplied to the registration command from the repository root. Run `atmos stack instances -s dev`, then `atmos helm template web -s dev` to inspect just this release.

To register a second release of the same chart in dev, add a sibling component such as `web-canary`, set its `metadata.component` to `web` and its Helm `name` to `web-canary`. For prod, create or extend `stacks/prod.yaml` with the selected release, namespace and prod values. Verify that selecting each component/stack pair renders the intended resource names and values. Show the selected cluster context before a live operation since the Atmos stack name alone does not identify a cluster.

## Requirements

- Detect a selected local chart by its Chart.yaml. For a directory with several charts, offer project-owned chart roots. Treat packaged dependencies under charts/ as dependencies unless explicitly selected.
- Preserve chart location, release name, namespace, ordered values files and existing chart defaults. For proposed Helm flags, resolve explicit values paths relative to the caller's directory as in the native example. Save relocatable references. Verify the effective overlays against native Helm for the supported input forms.
- Keep the Atmos stack name independent of the release and namespace. If an existing command or project definition provides those values, show their source. Ask for a missing release identity when necessary. Multiple named releases of one chart may coexist.
- Offer an offline render when all required chart inputs are available. Identify missing dependencies, external values or scripts affecting output. Cluster access and release installation are later operations.
- First delivery covers a local chart and values files. Repository or OCI chart resolution, post-renderers and environment-dependent wrappers require independent parity fixtures before being labeled reproducible.

## Acceptance

Compare the normalized Kubernetes manifests from Helm and Atmos for a local chart with two values files and an explicit namespace. Verify resource names, namespace and overridden values. Reverse the file order and verify the same change in both renders. Repeat with a second release name, a missing dependency, a conflicting component and an idempotent re-import. Observe a first-time user identifying the expected Service without a cluster or a manifest edit.
