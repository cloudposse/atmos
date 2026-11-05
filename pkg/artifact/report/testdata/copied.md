
## Artifacts

1 operation: 1 succeeded.

<!-- markdownlint-disable MD055 MD056 -->
| # | Operation | Status | Artifact | Repository | Destination | Digest | Size | Duration |
| ---: | --- | --- | --- | --- | --- | --- | ---: | ---: |
| 1 | Copy | ✅ Copied | `components/vpc:1.2.0` | `mirror` | `s3://backups/components/vpc/1.2.0` | `sha256:aaaaaaaaaaaa…` (resolved) | 1.0 MiB | 2.5 s |
<!-- markdownlint-enable MD055 MD056 -->

### ✅ 1. Copy `components/vpc:1.2.0`

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
