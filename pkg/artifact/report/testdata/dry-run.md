
## Artifacts

1 operation: 1 succeeded.

<!-- markdownlint-disable MD055 MD056 -->
| # | Operation | Status | Artifact | Repository | Destination | Digest | Size | Duration |
| ---: | --- | --- | --- | --- | --- | --- | ---: | ---: |
| 1 | Publish | 🔍 Dry run | `components/vpc:1.2.0` | `shared` | `oci://ghcr.io/acme/components/vpc:1.2.0` | `sha256:aaaaaaaaaaaa…` (resolved) | 48 KiB | 1.2 s |
<!-- markdownlint-enable MD055 MD056 -->

### 🔍 1. Publish `components/vpc:1.2.0`

Dry run `components/vpc:1.2.0` in repository `shared`.

- **Status:** Dry run
- **Step:** publish
- **Repository:** `shared`
- **Reference:** `components/vpc:1.2.0`
- **Destination:** `oci://ghcr.io/acme/components/vpc:1.2.0`
- **Size:** 48 KiB (49,152 bytes)
- **Duration:** 1.2 s

<details>
<summary>Digests (4)</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
| Resolved | `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` | OCI manifest or index digest; the record digest for local and S3 repositories |
| Record | `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` | canonical bytes of the package Record |
| Content | `sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc` | portable content identity of the inventory and documents |
| Representation | `sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd` | checksum of the transferred payload; it proves integrity, not the publisher |

Digests prove that the content is intact. They do not authenticate the publisher, and this operation did not report a verified signature.

</details>

<details>
<summary>Inventory (2 entries, 3.0 KiB)</summary>

- `main.tf` (1.0 KiB)
- `variables.tf` (2.0 KiB)

</details>

<details>
<summary>Metadata</summary>

VPC component

- `team`: `net`
- `tier`: `1`

</details>
