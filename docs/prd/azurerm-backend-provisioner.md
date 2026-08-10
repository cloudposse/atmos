# PRD: Azure (`azurerm`) Backend Provisioner

**Status:** Draft for Review
**Version:** 1.0
**Last Updated:** 2026-08-09
**Author:** Andriy Knysh

---

## Executive Summary

The Azure Backend Provisioner automatically creates the Azure resources that back an `azurerm`
Terraform state backend — a resource group, a storage account, and a blob container — with
secure defaults. It is the Azure counterpart to the [S3 Backend Provisioner](./s3-backend-provisioner.md)
and eliminates the cold-start friction of having to create state storage by hand before the
first `terraform init`.

**Key Principle:** Simple, opinionated Azure state storage with Azure best practices — not a
replacement for a production-grade module.

---

## Problem Statement

Atmos already generates `backend.tf.json` for `azurerm` backends and can read `azurerm` state
in-process (`!terraform.state`). What it could **not** do — until now — is *create* the backend
itself. The native backend-provisioning subsystem (`pkg/provisioner/backend/`,
`atmos terraform backend create|update|delete`, and auto-provisioning on
`terraform init` via `provision.backend.enabled`) was **S3-only**: for `azurerm`,
`atmos terraform backend create` returned `create not implemented for backend type: azurerm`,
and `provision.backend.enabled` silently skipped.

### Current Pain Points

1. **Manual storage creation**: Users must create a resource group, storage account, and
  container before running `terraform init` against an `azurerm` backend.
2. **Inconsistent security**: Hand-created storage accounts vary in TLS version, public access,
  versioning, and auth model.
3. **Cold-start delay**: Every new subscription needs a bootstrapped state backend before any
  Terraform can run — a classic chicken-and-egg.
4. **Feature asymmetry**: AWS users get auto-provisioning; Azure users had to build a bespoke
  Terraform component (e.g. `Azure/avm-res-storage-storageaccount` + a cold-start migration)
  for what is, in dev/test, a one-line convenience on AWS.

### Target Users

- **Development teams** bootstrapping Azure state quickly.
- **New users** learning Atmos on Azure.
- **CI/CD pipelines** creating ephemeral environments.
- **POCs/demos** without infrastructure overhead.

### Non-Target Users

- **Production environments**, which should manage state storage with a full module
  (e.g. `Azure/avm-res-storage-storageaccount`) for private endpoints, customer-managed keys,
  network ACLs, geo/zone redundancy, lifecycle management, and compliance controls.

---

## Goals & Non-Goals

### Goals

1. ✅ **Automatic resource creation**: Create the resource group (if missing), storage account,
  and container.
2. ✅ **Secure defaults**: TLS 1.2 minimum, HTTPS-only, public blob access blocked, blob
  versioning + soft delete, private container — always.
3. ✅ **Idempotent operations**: Safe to run repeatedly; skips when already provisioned.
4. ✅ **Entra ID hardening when appropriate**: Disable shared-key access when the backend uses
  `use_azuread_auth: true`.
5. ✅ **Zero configuration** beyond `provision.backend.enabled: true`.
6. ✅ **Backend deletion** with safety checks (`--force` required).
7. ✅ **Registry-native**: Self-registers into the existing backend provisioner registry so the
  auto-provision hook and `atmos terraform backend` commands work with no wiring changes.

### Non-Goals

1. ❌ **State locking resource**: None is created — the `azurerm` backend locks state with
  **native Azure Blob Storage blob leases** (the DynamoDB-lock analog is built into Blob
  Storage). Nothing to provision.
2. ❌ **Customer-managed keys (CMK)**: Uses Microsoft-managed encryption (always on in Azure Storage).
3. ❌ **Private endpoints / network ACLs**: Public network access remains enabled; Entra ID / RBAC
  still gates data-plane access.
4. ❌ **Geo/zone redundancy**: Uses `Standard_LRS` (single-region), the analog of a single-region
  S3 bucket.
5. ❌ **Lifecycle management, diagnostic logging, resource locks**: Out of scope; use a module.
6. ❌ **Production features**: Not competing with a managed storage-account module.

---

## What Gets Created

When `provision.backend.enabled: true` and the backend is not fully provisioned:

### Resource group (if missing)

- Created in the location of the active Azure identity (`authContext.Azure.Location`).
- If the resource group **already exists**, its location is reused and no location is required.

### Storage account (if missing) — hardcoded best practices

| Setting | Value |
|---|---|
| Kind | `StorageV2` |
| SKU | `Standard_LRS` |
| Minimum TLS | `TLS1_2` |
| HTTPS-only traffic | enabled |
| Public blob access | **blocked** (`AllowBlobPublicAccess=false`) |
| Shared-key access | **disabled** when the backend sets `use_azuread_auth: true`; otherwise left at the Azure default |
| Tags | `Name=<account>`, `ManagedBy=Atmos` |

### Blob data protection (always, idempotent)

- **Blob versioning enabled** — the direct analog of S3 versioning; every state write is recoverable.
- **Soft delete** (blob + container) with 30-day retention — a safety net for accidental deletes.

### Container (if missing)

- The configured `container_name`, with **no public access** (private).

### NOT created

- ❌ A lock table / lock resource (blob leases handle locking natively).
- ❌ Customer-managed keys, private endpoints, network rules, lifecycle rules, diagnostic settings.

---

## State Locking (Key Difference From AWS)

Cloud Posse's AWS provisioner and `aws-tfstate-backend` rely on Terraform ≥1.10 **native S3
lockfiles** (previously a separate DynamoDB table) to serialize concurrent applies. On Azure
there is **nothing to provision**: the `azurerm` backend acquires an exclusive **blob lease** on
the state blob for the duration of an operation and releases it when done; a concurrent run gets
a `409 Conflict` and backs off. This is the same mutual-exclusion guarantee, built into the blob
container this provisioner already creates. Leasing is a data-plane operation authenticated with
the same identity/RBAC that reads and writes state, so no extra resource and no extra permission
are required.

---

## Configuration

### Stack manifest example

```yaml
components:
  terraform:
    vpc:
      # Component authentication (Atmos Auth / Azure identity)
      auth:
        providers:
          azure:
            type: azure/interactive
            identity: platform

      # Backend configuration (standard Terraform azurerm)
      backend_type: azurerm
      backend:
        resource_group_name: rg-tfstate-cus
        storage_account_name: stexampletfstateplatformcus
        container_name: tfstate
        key: vpc.terraform.tfstate
        use_azuread_auth: true            # → provisioner disables shared-key access

      # Provisioning configuration (Atmos-only)
      provision:
        backend:
          enabled: true                   # Enable automatic azurerm backend creation
```

### Where inputs come from

| Input | Source | Notes |
|---|---|---|
| `resource_group_name` | backend config | required |
| `storage_account_name` | backend config | required; the S3-bucket analog / backend "name" |
| `container_name` | backend config | required |
| `subscription_id` | backend config → else active Azure identity | required (one of the two) |
| location | active Azure identity (`Location`) → else existing resource group's location | only required when the resource group must be created |
| `use_azuread_auth` | backend config | drives shared-key hardening |

**Why location is not a backend attribute:** Atmos writes the `backend` config verbatim into
`backend.tf.json`, and `location` is not a valid `azurerm` backend argument (Terraform would
reject it). It is therefore sourced from the Azure identity, or inherited from the resource group
when it already exists — never from the backend block.

### Inheritance

`provision.backend` participates in Atmos's deep-merge, so provisioning can be enabled at the
org/tenant/environment level and overridden per component (enable in dev/qa, disable in prod
where state storage is module-managed) — identical to the S3 provisioner's inheritance model.

---

## Implementation

### Package structure

```text
pkg/provisioner/backend/
  ├── azurerm.go            # azurerm backend provisioner (create, exists, name, registration)
  ├── azurerm_delete.go     # azurerm backend deletion (force-guarded)
  ├── azurerm_test.go       # unit tests (mocked azureBackendAPI)
```

### Design

- A narrow `azureBackendAPI` interface hides the Azure Resource Manager SDK's pollers and paged
  responses behind simple synchronous methods (`resourceGroupExists`, `createStorageAccount`,
  `applyBlobDataProtection`, `containerExists`, `createContainer`, `deleteStorageAccount`, …).
  This mirrors the wrapper pattern already used by
  `internal/terraform_backend/terraform_backend_azurerm.go` and makes the orchestration logic
  trivially mockable. A test-overridable client factory (`SetAzureBackendClientFactory` /
  `ResetAzureBackendClientFactory`) injects fakes, mirroring `SetS3ClientFactory`.
- The production implementation (`azureBackendClient`) is built from
  `armresources.ResourceGroupsClient` + `armstorage` client factory
  (`AccountsClient`, `BlobServicesClient`, `BlobContainersClient`), authenticated with
  `azidentity.NewDefaultAzureCredential` — the same credential chain the state reader uses.
- `init()` registers create/delete/exists/name in the shared registry, so the generic
  `before.terraform.init` hook (`pkg/provisioner/backend_hook.go`) and the
  `atmos terraform backend` commands pick up `azurerm` with **no changes** to the hook or CLI.

### SDK dependencies added

- `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources`
- `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage`

(`azcore`, `azidentity`, and `azblob` were already dependencies.)

---

## Testing Strategy

### Unit tests (`azurerm_test.go`)

Table-driven, using a manual mock of `azureBackendAPI` (matching `mockS3Client`), covering:

- Config extraction & validation (required fields, subscription precedence, location sourcing,
  `use_azuread_auth` bool/string parsing).
- Full create path (resource group + account + container created; correct location; shared-key
  hardening flag).
- Resource group already exists → its location is reused; no location required.
- Existing storage account → warning returned; data protection re-applied idempotently.
- Location-required error when the resource group must be created but none is available.
- Every error path (each `ensure*` step) maps to its typed sentinel error.
- Existence check (account missing → false; account present but container missing → false; both
  present → true).
- Deletion (force required; not-found; success; error propagation).
- Registry wiring and offline client construction.

Coverage of the provisioner logic is 84–100% per function; the thin ARM SDK passthrough wrappers
are covered by integration testing against real Azure (Azure Resource Manager has no
management-plane emulator, unlike gofakes3/Azurite for the data plane) — the same standard applied
to `s3.go`'s real client construction.

### Manual testing checklist

- [ ] Fresh subscription: resource group + storage account + container created; secure defaults verified.
- [ ] Existing resource group: account created in the group's location without a configured location.
- [ ] `use_azuread_auth: true`: account created with shared-key access disabled.
- [ ] Idempotent re-run: no changes, no errors, skip on `terraform init`.
- [ ] Cold-start: `provision.backend.enabled: true` on the backend's own component, then
      `atmos terraform apply` → `init`, auto-provision, apply, state written to the new backend.
- [ ] `atmos terraform backend delete <component> -s <stack> --force`: account removed; resource
      group preserved.
- [ ] Permission denied (missing RBAC): clear, actionable error.

---

## Security

### Required RBAC

The active identity (or CI managed identity) needs permission to manage the resource group and
storage account, e.g. **Contributor** on the resource group (or a scoped custom role with
`Microsoft.Resources/subscriptions/resourceGroups/*`,
`Microsoft.Storage/storageAccounts/*`, and `Microsoft.Storage/storageAccounts/blobServices/*`).
Reading/writing state additionally needs **Storage Blob Data Contributor** when the backend uses
`use_azuread_auth: true` — the same grant the state data plane already requires.

### Hardened defaults (always)

TLS 1.2 minimum, HTTPS-only, public blob access blocked, private container, blob versioning +
soft delete. Shared-key access is disabled when the backend authenticates with Entra ID.

### Not included (use a module for production)

Customer-managed keys, private endpoints, network ACLs, geo/zone redundancy, lifecycle rules,
diagnostic settings, resource locks.

---

## Migration to a production backend

The provisioned resources are standard Azure resources. To graduate to a managed module, import
the resource group / storage account / container into a Terraform module (e.g.
`Azure/avm-res-storage-storageaccount`), add production settings (CMK, private endpoints, network
rules), and continue using the same `azurerm` backend — no state migration required, because the
storage account keeps its name and contents. The provisioner is idempotent, so
`provision.backend.enabled: true` can be left in place (it detects the resources exist and skips).

---

## FAQ

**Q: Why no lock table?** Azure's `azurerm` backend locks with native blob leases — locking is
built into Blob Storage, so there is nothing to provision (unlike DynamoDB on AWS).

**Q: Is the auto-provisioned account production-ready?** No. It's for dev/test/bootstrap. Use a
managed module for production (CMK, private endpoints, redundancy, compliance).

**Q: What if the resource group already exists?** It's reused, and the storage account is created
in the group's location — no `location` needs to be configured.

**Q: Where does `location` come from?** From the active Azure identity, or the existing resource
group. It is intentionally *not* read from the backend block (Terraform would reject it there).

**Q: Does deletion remove the resource group?** No. It deletes the storage account (and thus all
state within it, like deleting an S3 bucket) and leaves the resource group, which commonly holds
unrelated resources. `--force` is required.

---

## Related Documents

- **[S3 Backend Provisioner](./s3-backend-provisioner.md)** — the reference implementation.
- **[Backend Provisioner](./backend-provisioner.md)** — backend provisioner interface.
- **[Provisioner System](./provisioner-system.md)** — generic provisioner infrastructure.

---

**End of PRD**
