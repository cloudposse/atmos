
## Artifacts

5 operations: 4 succeeded, 1 failed.

<!-- markdownlint-disable MD055 MD056 -->
| # | Operation | Status | Artifact | Repository | Destination | Digest | Size | Duration |
| ---: | --- | --- | --- | --- | --- | --- | ---: | ---: |
| 1 | Publish | ✅ Published | `components/vpc:1.2.0` | `shared` | `oci://ghcr.io/acme/components/vpc:1.2.0` | `sha256:aaaaaaaaaaaa…` (resolved) | 48 KiB | 1.2 s |
| 2 | Sign | ✅ Signed | `components/vpc:1.2.0` | `shared` |  | `sha256:bbbbbbbbbbbb…` (record) |  | 300 ms |
| 3 | Verify | ✅ Verified | `components/vpc:1.2.0` | `shared` |  | `sha256:bbbbbbbbbbbb…` (record) |  |  |
| 4 | Copy | ✅ Copied | `components/vpc:1.2.0` | `mirror` | `s3://backups/components/vpc/1.2.0` | `sha256:aaaaaaaaaaaa…` (resolved) | 1.0 MiB | 2.5 s |
| 5 | Publish | ❌ Failed | `components/vpc:1.2.0` | `shared` |  |  |  |  |
<!-- markdownlint-enable MD055 MD056 -->

### ✅ 1. Publish `components/vpc:1.2.0`

Published `components/vpc:1.2.0` in repository `shared`.

- **Status:** Published
- **Step:** publish
- **Repository:** `shared`
- **Reference:** `components/vpc:1.2.0`
- **Destination:** `oci://ghcr.io/acme/components/vpc:1.2.0`
- **Size:** 48 KiB (49,152 bytes)
- **Duration:** 1.2 s
- **Lock:** host scope. Writers on this host were serialized. Writers on other hosts are not excluded.

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

### ✅ 2. Sign `components/vpc:1.2.0`

Signed `components/vpc:1.2.0` in repository `shared`.

- **Status:** Signed
- **Step:** sign
- **Repository:** `shared`
- **Reference:** `components/vpc:1.2.0`
- **Duration:** 300 ms
- **Lock:** repository scope. Writers on every host were serialized through the repository's own coordination.
- **Signature:** signed. Signature evidence was stored for this subject. Signing alone does not authenticate the signer: verify the artifact against a trust policy to do that.
  - Subject: record `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`
  - Evidence: sigstore-bundle `sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee`

<details>
<summary>Digests (1)</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
| Record | `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` | canonical bytes of the package Record |

Digests prove that the content is intact. They do not authenticate the publisher, and this operation did not report a verified signature.

</details>

### ✅ 3. Verify `components/vpc:1.2.0`

Verified `components/vpc:1.2.0` in repository `shared`.

- **Status:** Verified
- **Step:** verify
- **Repository:** `shared`
- **Reference:** `components/vpc:1.2.0`
- **Signature:** verified. A valid signature from a signer permitted by the trust policy covers this subject, so the publisher is authenticated.
  - Subject: record `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`
  - Policy fingerprint: `sha256:ffffffffffff…`
  - Verified signer: identity `https://github.com/acme/infra/.github/workflows/release.yaml@refs/heads/main` issuer `https://token.actions.githubusercontent.com`
  - Verified signer: key `SHA256:abcdef`

<details>
<summary>Digests (1)</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
| Record | `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` | canonical bytes of the package Record |

The signature was verified against the configured trust policy, so the signer is authenticated. The digests identify the verified content.

</details>

### ✅ 4. Copy `components/vpc:1.2.0`

Copied `components/vpc:1.2.0` in repository `mirror`.

- **Status:** Copied
- **Step:** copy
- **Repository:** `mirror`
- **Reference:** `components/vpc:1.2.0`
- **Destination:** `s3://backups/components/vpc/1.2.0`
- **Size:** 1.0 MiB (1,048,576 bytes)
- **Duration:** 2.5 s
- **Lock:** repository scope. Writers on every host were serialized through the repository's own coordination.

<details>
<summary>Digests (1)</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
| Resolved | `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` | OCI manifest or index digest; the record digest for local and S3 repositories |

Digests prove that the content is intact. They do not authenticate the publisher, and this operation did not report a verified signature.

</details>

### ❌ 5. Publish `components/vpc:1.2.0`

The publish operation for `components/vpc:1.2.0` failed.

- **Status:** Failed
- **Step:** publish
- **Repository:** `shared`

**Error**

```text
registry unavailable
```
