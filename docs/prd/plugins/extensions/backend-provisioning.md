# Backend provisioning extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. New plugin/configuration APIs below are proposed. [All extension types](README.md).

A backend provisioner creates and manages infrastructure needed to store component state, such as an enterprise-managed object-storage backend. It is distinct from reading/writing values through a store and from deploying a component's resources.

## Consumer experience

```shell
atmos plugin install acme/backends --as backends --version '~1.0'
atmos terraform backend create app -s production
atmos terraform backend describe app -s production
atmos terraform init app -s production
```

Existing backend commands route to the selected registered provisioner. Automatic provisioning remains explicitly enabled through the component's provisioning configuration.

Proposed selection example:

```yaml
components:
  terraform:
    app:
      backend_type: s3
      backend:
        bucket: acme-production-state
        key: app/terraform.tfstate
        region: us-east-1
      provision:
        backend:
          enabled: true
          kind: acme/managed-state
          options:
            service_tier: standard
```

The new `provision.backend.kind` selects the provisioner independently of `backend_type`, which remains the state protocol understood by Terraform. The plugin could create the bucket through a corporate service without replacing Terraform's S3 backend implementation. Legacy configurations without `kind` retain existing backend-type dispatch.

The example describes provisioning intent, not credentials. Options use the plugin's schema and are separate from the generated backend configuration. Adding a provisioner cannot teach Terraform/OpenTofu an unsupported state protocol.

## Author contract

```shell
atmos plugin create acme-backends --capability provisioner.backend --runtime go --kind acme/managed-state
```

Starlark supports the same extension. Scaffold schemas, mocked service calls, operation fixtures, lifecycle configuration, and platform publication.

Adapt the existing [backend operations registry](../../../../pkg/provisioner/backend/backend.go) and [provisioner lifecycle registry](../../../../pkg/provisioner/registry.go). Public requests contain typed configuration, backend identity, scoped auth, operation, cancellation, and output services; internal closures and managers do not cross the protocol.

- Declare supported component/backend types and operations. Create/ensure, existence inspection, and stable resource identity are required; update, delete, and listing support is explicit. Unsupported operations fail clearly.
- Distinguish absent, present/compatible, present/incompatible, denied, and unknown states. An unavailable existence check must not be interpreted as absence. This requires stronger external-adapter behavior than the current missing-checker fallback.
- Return resource identifiers, operation outcomes, and warnings. Never report an uncertain or partially completed operation as success.
- Make ensure behavior idempotent where the backend supports it. Document ownership/adoption of existing resources and concurrent creation behavior; Atmos never silently retries a write after a crash.
- Explicit backend commands and enabled pre-init lifecycle provisioning use the same handler/configuration/auth path. Failure before init prevents dependent initialization.
- Deletion is explicit and subject to Atmos command's destructive-operation/force policy. Plugins cannot infer permission to delete a nonempty state backend. Removing a plugin package never deletes provisioned infrastructure.
- Atmos credentials, pinned tool dependencies, masking, streams, cancellation, and deadlines apply identically to both runtimes. Bootstrap cannot require the backend it is trying to create.
- Reject names/kinds that collide with existing registrations; a configured custom provisioner may target a supported backend protocol without replacing its built-in registration.

Artifact-delivery targets such as Git/Kubernetes publication use a separate [target registry](../../../../pkg/provisioner/target/target.go). They remain a follow-on contract; this extension does not promise arbitrary delivery behavior.

## Integration

| Aspect | Today |
|---|---|
| Registry | Four parallel maps in [`backend.go`](../../../../pkg/provisioner/backend/backend.go): `RegisterBackendCreate`, `RegisterBackendDelete`, `RegisterBackendExists`, and `RegisterBackendName`. Lifecycle registration is [`provisioner.RegisterProvisioner`](../../../../pkg/provisioner/registry.go). |
| Duplicate policy | The four backend maps replace silently. `RegisterProvisioner` appends to the per-event list without deduplication. |
| Context on the interface | Yes. Create, delete, exists, and `ProvisionerFunc` take `ctx`. |
| Reachable from outside the module | Yes. |
| Closed lists to open | Only `s3` and `azurerm` register, and there is no `gcs` provisioner. A missing exists-checker falls back to "assume the backend does not exist" rather than reporting unknown. The [`terraform_backend`](../../../../internal/terraform_backend/terraform_backend_registry.go) state readers (local, s3, gcs, azurerm) are closed but are not required by this extension. |
| Boundary leaks | `ProvisionerFunc` receives `*schema.AtmosConfiguration`, `*schema.AuthContext`, `OutputWriters`, and `*TerraformExecContext`. |

The new `provision.backend.kind` selector falls back to `backend_type`; it is not in any schema today. [Targets](../../../../pkg/provisioner/target/target.go) replace on duplicate and remain a follow-on.

**Adapter:** `pkg/plugin/adapter/backend` implements the unified provisioner interface (N7 in the gap analysis: `Ensure`, `Exists`, `Delete`, `Name`, `Describe`) and calls the `BackendProvisioner` wire service. It consumes the `api.Credentials`, `api.HTTP`, and `api.Log` API services.

**Injection points:**
- Backend lookup: resolve `provision.backend.kind` first, then fall back to `backend_type`.
- Explicit `atmos terraform backend` commands and pre-init lifecycle provisioning: both use the same resolution path.
- `Exists`: return absent, present-compatible, present-incompatible, denied, or unknown; the missing-checker fallback must not apply to external adapters.

**Prerequisites:** P0.3 Duplicate policy; P0.10 Backend provisioner interface with existence states; Phase 3 work package WP3.9 (backend provisioners). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `*schema.AtmosConfiguration`: replaced by the validated options map and backend configuration.
- `*schema.AuthContext`: replaced by the `api.Credentials` service.
- `OutputWriters`: replaced by streamed output chunks and the `api.Log` service.
- `*TerraformExecContext`: replaced by component and stack coordinates.

## Acceptance

A plugin provisions a supported state backend through both explicit commands and configured lifecycle execution, then the component initializes against it. Tests cover existing/absent/incompatible resources, denied/unknown existence, unsupported operations, ownership, concurrent creation, partial failure, deletion policy, cancellation, auth cycles, and secret redaction.

Go and Starlark implementations pass the same applicable contract fixtures. Updates/rollback change provisioner code without rebuilding Atmos or recreating resources. Existing built-in provisioning remains compatible.
