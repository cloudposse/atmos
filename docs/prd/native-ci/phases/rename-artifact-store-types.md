# Rename Artifact Store Types to Namespaced Convention

> Related: [Artifact Storage](../framework/artifact-storage.md) | [Configuration](../framework/configuration.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Unify Artifact Stores](unify-artifact-stores.md)

## Problem Statement

The current artifact store type names use flat, inconsistent naming:

| Current Name | Backend |
|-------------|---------|
| `s3` | AWS S3 |
| `github-artifacts` | GitHub Actions Artifacts API |
| `azure-blob` | Azure Blob Storage (not yet implemented) |
| `gcs` | Google Cloud Storage (not yet implemented) |
| `local` | Local filesystem |

This naming has several problems:

1. **No provider grouping.** `s3` doesn't indicate it's an AWS service. `gcs` doesn't indicate it's a Google service. As we add more backends (e.g., AWS CodeArtifact, Google Artifact Registry), flat names become ambiguous and hard to discover.

2. **Inconsistent naming style.** `github-artifacts` uses `vendor-product` format, `azure-blob` uses `vendor-product`, but `s3` and `gcs` use product-only abbreviations. `local` has no qualifier at all.

3. **No room for sub-variants.** If we add `s3` variants (e.g., S3 with different auth mechanisms) or multiple local backends (directory vs. tarball), flat names require awkward suffixes like `s3-iam`, `local-dir`, `local-tar`.

## Desired State

Rename all store types to a `{provider}/{backend}` namespaced convention:

| Current Name | New Name | Provider | Backend |
|-------------|----------|----------|---------|
| `s3` | `aws/s3` | `aws` | `s3` |
| `github-artifacts` | `github/artifacts` | `github` | `artifacts` |
| `azure-blob` | `azure/blob` | `azure` | `blob` |
| `gcs` | `google/gcs` | `google` | `gcs` |
| `local` | `local/dir` | `local` | `dir` |

### Scope

Only **implemented** backends are renamed in code: `aws/s3`, `github/artifacts`, `local/dir`. The `azure/blob` and `google/gcs` names are established as the convention in PRD documentation only — they will use these names when implemented.

### No Backward Compatibility

This is a clean rename with no aliases, no deprecation warnings, and no backward-compatible fallbacks. Old type names (`s3`, `github-artifacts`, `local`) stop working immediately. This is acceptable because the feature is experimental and not yet released to users.

### Package Directories Unchanged

Go package directories (`pkg/ci/artifact/s3/`, `pkg/ci/artifact/github/`, `pkg/ci/artifact/local/`) are **not** renamed. Only the registry keys (the `storeName` constant used in `artifact.Register()`) change. The package directory structure is an internal implementation detail.

### Benefits

- **Discoverable**: `aws/s3` immediately tells users this is an AWS backend. Tab completion can suggest all `aws/*` backends.
- **Extensible**: Adding `aws/codecommit`, `google/artifact-registry`, or `local/tar` is natural.
- **Consistent**: Every type follows the same `{provider}/{backend}` pattern.

## Implementation

### Step 1: Update store name constants

Update the `storeName` constant in each implemented store:

**`pkg/ci/artifact/s3/store.go`**
```go
// Before
const storeName = "s3"

// After
const storeName = "aws/s3"
```

**`pkg/ci/artifact/github/store.go`**
```go
// Before
const storeName = "github-artifacts"

// After
const storeName = "github/artifacts"
```

**`pkg/ci/artifact/local/store.go`**
```go
// Before
const storeName = "local"

// After
const storeName = "local/dir"
```

Each store's `init()` function calls `artifact.Register(storeName, NewStore)`, so changing the constant automatically updates the registry key. No changes to `init()` or `registry.go` needed.

### Step 2: Update test fixtures

**`tests/fixtures/scenarios/native-ci/atmos.yaml`**
```yaml
stores:
  s3:
    type: aws/s3
    options:
      bucket: "my-terraform-planfiles"
      # ...

  github:
    type: github/artifacts
    options:
      retention_days: 7
      # ...

  azure:
    type: azure/blob
    options:
      account: "mystorageaccount"
      # ...

  gcs:
    type: google/gcs
    options:
      bucket: "my-gcs-bucket"
      # ...

  local:
    type: local/dir
    options:
      path: ".atmos/planfiles"
      # ...
```

Note: `azure` and `gcs` entries use the new naming convention in config but have no backend implementation — they will fail at runtime if selected (same as today).

### Step 3: Update configuration schema

**`pkg/datafetcher/schema/config/global/1.0.json`** and other schema files:
- Replace old type names with new names in any enum lists that validate store types.

### Step 4: Update tests

**`pkg/ci/artifact/selector_test.go`** — Update test type names from `"s3"`, `"gcs"` to `"aws/s3"`, `"google/gcs"`.

**`pkg/ci/artifact/s3/store_test.go`**, **`pkg/ci/artifact/github/store_test.go`**, **`pkg/ci/artifact/local/store_test.go`** — Update any assertions on `Name()` return values.

### Step 5: Update PRD documentation

Update all references to old type names in shipped PRDs:

- `docs/prd/native-ci/framework/configuration.md` — config examples
- `docs/prd/native-ci/framework/artifact-storage.md` — backend tables
- `docs/prd/native-ci/terraform-plugin/planfile-storage.md` — backend table and references
- `docs/prd/native-ci/phases/unify-artifact-stores.md` — architecture diagrams and registry entries

### Step 6: Update user-facing documentation

- `website/docs/ci/planfile-storage.mdx` — storage backends table and config examples
- `website/docs/cli/configuration/ci/index.mdx` — if it references store types

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/ci/artifact/s3/store.go` | Change `storeName` to `"aws/s3"` |
| `pkg/ci/artifact/github/store.go` | Change `storeName` to `"github/artifacts"` |
| `pkg/ci/artifact/local/store.go` | Change `storeName` to `"local/dir"` |
| `pkg/ci/artifact/selector_test.go` | Update test type names |
| `pkg/ci/artifact/s3/store_test.go` | Update `Name()` assertions |
| `pkg/ci/artifact/github/store_test.go` | Update `Name()` assertions |
| `pkg/ci/artifact/local/store_test.go` | Update `Name()` assertions |
| `tests/fixtures/scenarios/native-ci/atmos.yaml` | Update store type values |
| `docs/prd/native-ci/framework/configuration.md` | Update type names in examples |
| `docs/prd/native-ci/framework/artifact-storage.md` | Update backend tables |
| `docs/prd/native-ci/terraform-plugin/planfile-storage.md` | Update backend table |
| `docs/prd/native-ci/phases/unify-artifact-stores.md` | Update architecture diagrams |
| `website/docs/ci/planfile-storage.mdx` | Update user-facing type names |
| `pkg/datafetcher/schema/config/global/1.0.json` | Replace old type names with new |
| `pkg/datafetcher/schema/stacks/stack-config/1.0.json` | Replace if applicable |
| `pkg/datafetcher/schema/atmos/manifest/1.0.json` | Replace if applicable |

## Notes

### No data migration needed

Store type names are configuration-only — they don't appear in stored artifacts or metadata. Changing the type name doesn't affect existing stored planfiles.

### Priority list names vs. store type names

The `priority` list in config uses store *names* (keys in the `stores` map), not store *types*. These are unaffected by this change:

```yaml
planfiles:
  priority:
    - "github"    # This is the store NAME, not the type
    - "s3"        # This is the store NAME, not the type
    - "local"     # This is the store NAME, not the type
  stores:
    s3:
      type: aws/s3  # This is the store TYPE (what we're renaming)
```

Users can name their stores anything. The `type` field is what maps to the registered backend.

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/artifact/...` — all store tests pass with new names
3. `go test ./pkg/ci/plugins/terraform/...` — planfile integration tests pass
4. `make lint`
5. Manual: config with `type: aws/s3` works correctly
6. Manual: `atmos terraform planfile list` works with new type names
