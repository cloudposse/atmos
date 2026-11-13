---
title: Backstage Developer Portal
tags: [Automation, Integrations]
cast:
  file: /casts/examples/backstage/service-catalog.cast
  title: Backstage service catalog with Atmos and Gitea
related_docs:
  - label: Container components
    url: /stacks/components/container
  - label: Git commands
    url: /cli/commands/git/usage
  - label: Embedded Starlark
    url: /steps/type/script
---

# Backstage with Atmos and local Git

A local service catalog with a real infrastructure request flow:

```text
Backstage form → Atmos scaffold + Git target → Gitea PR → review and merge
                                                        ↓
                                    Atmos plan/apply → local S3 bucket
```

Atmos runs Backstage as a native container component, Gitea as its **Git emulator**,
and Floci as its **AWS emulator**. A Software Template opens a PR in Gitea.
Gitea Actions validates and plans the request; after review and merge, its runner
uses an Atmos Starlark module to provision the bucket and register the service. No cloud account or GitHub token is needed.

## Start

Requirements: Docker, Git, Go, Terraform, and Atmos built from this checkout
(includes `ports.host` IP:port bindings and noninteractive container exec support). Ports **3000,
3001, 7007, and 4567** must be free. Allow several minutes, several GB of disk,
and at least 4 GB of Docker memory for Backstage's first build. Internet access
is needed to download images, Node packages, and the Terraform AWS provider.
Node is only required inside the Backstage image.

From the repository root:

```shell
go build -o build/atmos .
export PATH="$PWD/build:$PATH"
cd examples/backstage
atmos portal up
atmos portal status
atmos portal open
```

`atmos portal open` launches the dashboard using the native Starlark `browser.open()` function.
You can also use `atmos portal up --open` to request a browser launch after startup. Plain
`portal up` does not launch a browser.

Wait for Backstage to become **healthy**, then open:

- **Backstage:** http://localhost:3000 — enter as a guest.
- **Gitea:** http://localhost:3001 — local demo login **atmos / atmos**.
- **Infrastructure repository:** http://localhost:3001/atmos/deployments.

All published ports bind to `127.0.0.1`. Containers communicate over the native
Atmos stack network. `portal up` seeds the Git repository only once; later starts
do not overwrite reviewed infrastructure configuration.

## Appearance

The portal includes Geist Light and Geist Dark themes. Open **Settings → Appearance**
to switch between them or follow your system preference. Geist Sans and Geist Mono
are bundled in the image; the browser does not fetch fonts from a CDN.

The theme uses neutral surfaces, flat headers, subtle borders, and an Atmos portal
wordmark. Status indicators retain their semantic colors. Customize
`backstage/themes.tsx` for Material UI components and `backstage/geist.css` for
Backstage UI components. `backstage/portalModules.tsx` registers the themes,
navigation, Gitea entity content, and TechDocs addon through the new frontend
extension system. `backstage/PortalNav.tsx` supplies the Atmos-branded navigation;
`backstage/GeistDocs.tsx` applies readable typography inside the TechDocs reader's
shadow DOM. `backstage/PortalCatalog.tsx` customizes the native catalog page
through a plugin extension override, retaining compact responsive columns and
favorites. Search and filters are enabled; sorting is disabled because the
upstream table renders nested sort buttons. Catalog, entity pages, and scaffolder
routes come from the installed frontend plugins, rather than a manually assembled
React router.


After changing the theme, rebuild and replace only the Backstage container:

```shell
atmos container build backstage -s local
atmos container down backstage -s local
atmos container up backstage -s local
```

The Git and AWS emulators and the Actions runner remain running.

## Request and provision a service

1. In Backstage, select **Create → Request a service with an S3 bucket**.
2. Enter a unique service name such as `orders`, review, and click **Create**.
3. Open the pull request linked in the task output. It adds:
    - `stacks/deploy/services/orders.yaml`: an Atmos `dev-orders` stack using the
      shared `bucket` Terraform component.
    - `catalog/orders.yaml`: a Backstage `Component` and an S3 `Resource`, with
      ownership, an infrastructure source link, and a dependency relationship.
4. Wait for the **plan** Actions job, then review and merge the PR in Gitea.
    The **deploy** job provisions and registers the service automatically.
5. Open [Actions](http://localhost:3001/atmos/deployments/actions) to inspect job logs,
    then open the service in Backstage.

### Optional manual runner

The commands below are an alternative for a new service when the Gitea runner
is stopped **before merging**. CI and the manual runner use separate local state;
do not apply the same service through both.

From this example directory, run:

```shell
atmos portal plan --service orders
atmos portal apply --service orders
```

The runner pulls the managed repository's `main` branch, requires the requested
stack to exist in that commit, and executes:

```shell
atmos terraform plan bucket -s dev-orders --identity local-aws
atmos terraform apply bucket -s dev-orders --identity local-aws -auto-approve
```

These commands run inside `.atmos/git/deployments`. The `local-aws` identity
resolves **only this example's local AWS emulator** and configures the provider
endpoints. The result is the S3 bucket **`atmos-backstage-dev-orders`**. Terraform
state stays in the managed clone, separated by Atmos's stack workspaces.

After a successful apply, the runner registers the catalog YAML's Git URL with
Backstage. Open **Catalog → orders**: it has an owner, a link to the Atmos stack
in Git, and a dependency on **orders-bucket**. Catalog ingestion is asynchronous;
allow a few seconds for the service and resource to appear.

The catalog reads the YAML from Git rather than storing a separate copy of the
service definition. This example registers entities after provisioning, but the
catalog is not a live Terraform-state monitor. Later infrastructure drift or
manual deletion is not automatically reflected in the catalog.

## Scaffold and Git delivery

`scaffolds/service` is an `AtmosScaffoldConfig` template. Its validated
`service_name` field generates both the Atmos stack and Backstage catalog YAML.
The Backstage action invokes `scripts/request.star` in an isolated task workspace:

```python
atmos.scaffold("generate", "/demo/scaffolds/service", "generated", flags = {
    "defaults": True,
    "set": ["service_name=" + name],
})
steps.publish(
    source = "generated/catalog/" + name + ".yaml",
    target = {"kind": "git", "repository": "request", "path": "catalog"},
)
```

The script publishes the stack and catalog to the managed repository's request
branch. Atmos handles cloning, path-scoped commits, and pushing. Each task owns
its checkout; retries skip files already on the branch to preserve reviewer edits.
Scaffold bookkeeping stays in the task workspace.

Gitea branch and PR creation remain small API calls in the Backstage adapter.
The file-publish target currently does not support PR creation; component Git
targets support it through GitHub and Azure DevOps providers, but no Gitea provider
is registered. The demo does not pretend that `scaffold generate --target` is a
Git destination: scaffold's target argument is a local directory, followed by
explicit publication through `steps.publish`.

## Gitea Actions and TechDocs

The native `backstage-runner` container runs Gitea's `act_runner` with the
`atmos-local:host` label. Here “host” means execution inside that container, not
on your workstation. No Docker socket is mounted. Runner capacity is one.
`portal up` cross-compiles Atmos from this repository into `.atmos/bin/atmos`
for the running container architecture and includes it in the runner image.
This requires Go and ensures the runner has the native Git/CI features under
development, rather than the older release used to bootstrap Backstage.

The seeded `.gitea/workflows/atmos.yaml` uses `atmos git clone --ref <event SHA>`
with full history to check out the exact PR or push revision and retain the base
commit needed for the diff. It then
uses `module.load("infrastructure.star")` to run Atmos validation and Terraform:

- Pull requests: plan added or modified service stacks.
- Pushes to `main`: apply added or modified service stacks and register their
  Git-backed catalog entries with Backstage.

Deleted stack files are not automatically destroyed. The deploy job produces a
fresh plan from the merged commit; it does not apply the PR's saved plan. Branch
protection and required checks are not configured by this demo.

CI state persists in `.atmos/ci/state`, separated by Atmos Terraform workspace.
Runner registration and job workspaces live in `.atmos/ci/runner`. Only the local
throwaway infrastructure repository should use this runner. For shared infrastructure,
use separate plan/apply permissions, protected branches, and remote state with locking.

The **CI/CD** tab links to real Gitea Actions runs and pull requests. The **Docs**
tab on `payments` serves a basic handbook; newly scaffolded services reference the
same handbook. Markdown lives in `catalog/techdocs/` and is built with MkDocs during the
image build, then served by Backstage's local TechDocs publisher. No runtime docs
builder or Docker socket is needed.

```shell
atmos container logs backstage-runner -s local
```

## Inspect configuration without provisioning

The original **Create → Inspect an Atmos stack** template remains available. It
runs `atmos describe component service -s dev --format json` inside Backstage.
Select `dev` to see 2 replicas, or `prod` to see 4. Both inherit `platform-team`
from the shared component catalog. This uses the bundled read-only demo stacks;
the provisioning workflow uses the separate infrastructure repository in Gitea.

## Verify

Reusable libraries in `scripts/modules/` are imported with `module.load(...)`.
The entry scripts pass arguments explicitly to their exported functions; importing
a module does not provision resources or publish files. For example:

```python
infrastructure = module.load("modules/infrastructure.star")
catalog = module.load("modules/catalog.star")
name = infrastructure.run_terraform("apply", ctx.flags["service"])
catalog.register_service(name)
```

Those modules use Atmos's built-in APIs: `atmos.git`,
`atmos.terraform`, `atmos.portal`, `steps.http`, and `steps.sleep`. HTTP polling
uses native step retries; there is no curl, sleep, or date subprocess. Lifecycle
commands use `atmos.container` and `atmos.emulator`.

Run the native Atmos test suites:

```shell
atmos portal test
# Opt-in end-to-end test: creates and merges its own PR, waits for Gitea to provision a bucket,
# checks the real S3 endpoint, and verifies the Git-backed catalog relationship.
atmos portal test --provision
```

`portal test` uses a native `type: test` group: backend readiness, dashboard,
catalog/templates, TechDocs metadata and content, dev/prod stack inheritance,
and rejection of invalid stacks and service names. Each check has a named result;
failures show their buffered diagnostics and make the command exit nonzero.
These checks may create inspection tasks, but do not create infrastructure or merge PRs.
Reusable assertions live in `scripts/modules/backstage_tests.star` and are loaded
with `module.load(...)`.

`--provision` adds a native `steps.test(...)` group for the dependent end-to-end
sequence. It runs only after the integration group passes. It waits for the PR
plan before merging, then waits for deployment and verifies the actual S3 bucket
and catalog resource relationship.

The full test leaves its `smoke-<commit>` service, PR, and bucket available for
inspection. It never merges another user's request. Run `portal down` to reset
the entire demo.

## Recorded demo

The terminal cast shows the running portal and native tests, including the real
Gitea PR → plan → merge → provision → catalog flow. The commands and their
results, including the wait for CI, are recorded from the live demo.

[Download the cast](../../website/static/casts/examples/backstage/service-catalog.cast)

From the repository root, with the portal already running:

```shell
atmos --chdir=demo/casts casts generate examples backstage service-catalog
atmos --chdir=demo/casts casts validate examples backstage service-catalog
atmos cast play website/static/casts/examples/backstage/service-catalog.cast
```

Generation creates and merges a new demo PR and leaves its service available.
Validation only reads the saved recording; it does not need Docker or a running
portal. Generation is intentionally opt-in, while bulk cast validation includes it.
The recording is declared with `cast.recorder` in
`demo/casts/scripts/casts/backstage-service-catalog.star`.

## Operate and clean up

```shell
atmos portal logs
atmos portal status
atmos portal down
```

**`portal down` resets this demo:** it removes Backstage, the CI runner, both ephemeral emulators
(and their repositories/buckets), CI state, and the managed Git clone with its local Terraform
state. It does not stop other Atmos emulators. The next `up` starts a fresh Git
repository. Backstage's in-memory catalog and task history also reset on restart.
The built image is retained for faster starts; optionally remove it afterwards
with `docker image rm atmos-backstage:local`.

After editing Backstage action code or configuration, recreate only the portal
so the Git repository and emulator resources remain available:

```shell
atmos container down backstage -s local
atmos portal up
```

`up` alone reuses a running container. Changes to `seed/` are only copied into a
fresh Git repository; update existing infrastructure through Git or reset the demo.

## Files

| File | Purpose |
| --- | --- |
| `atmos.yaml` | Native lifecycle commands and the managed Git repository |
| `stacks/deploy/local.yaml` | Backstage container and Git/AWS emulators |
| `backstage/request-service.ts` | Backstage action that opens infrastructure PRs |
| `templates/request-service/template.yaml` | Service-request form and review instructions |
| `scaffolds/service/` | Atmos scaffold for stack and catalog files |
| `scripts/request.star` | Loads the infrastructure module to generate and publish a request |
| `scripts/modules/infrastructure.star` | Reusable scaffold, Git delivery, seed, and Terraform operations |
| `scripts/modules/catalog.star` | Reusable Backstage HTTP and catalog registration operations |
| `seed/` | Initial infrastructure repository, Gitea workflow, and CI Starlark modules |
| `runner/` | Runner image, configuration, and local registration |
| `catalog/techdocs/` | MkDocs handbook for the Docs tab |
| `scripts/seed.star` | Seed the repository without overwriting later changes |
| `scripts/plan.star`, `scripts/apply.star` | Local runner for merged requests |
| `scripts/modules/backstage_tests.star` | Reusable checks for the native test groups |
| `scripts/test-provision.star` | Opt-in native provisioning test group |
| `backstage/atmos.ts` | Read-only configuration action and module registration |
| `backstage/app-config.yaml` | Local catalog locations, extension configuration, and narrowly allowed Git reads |
| `backstage/App.tsx` | New frontend app with explicit plugin installation |
| `backstage/portalModules.tsx` | Theme, navigation, CI/CD entity content, and TechDocs extensions |

The small `.mjs` files are specific to the Node image: `configure.mjs` patches
the generated app at build time, and `healthcheck.mjs` probes the container's two
servers. All host-side orchestration and verification use Atmos/Starlark.
`backstage/App.tsx` explicitly installs the frontend plugins and our extension
modules. Backstage plugin actions use TypeScript as required by its backend API.

The image is generated by a pinned `@backstage/create-app` release, using its
new frontend system and current backend module API. No legacy app conversion
or generated router or entity-page patches are used. It is a development image, not
a production deployment. The generated app and lockfile live in the image;
transitive dependencies are not frozen in this example repository.

## References

- [Backstage custom actions](https://backstage.io/docs/features/software-templates/writing-custom-actions/)
- [Backstage catalog configuration](https://backstage.io/docs/features/software-catalog/configuration/)
- [Atmos local GitOps example](../local-gitops/)
- [Atmos AWS emulator example](../emulator-aws/)
