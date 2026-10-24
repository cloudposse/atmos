
## 🐳 registry.example.com/app:sha-abc  &nbsp; `27.1 MB`  &nbsp; `Apache-2.0`  &nbsp; `amd64`  &nbsp; `linux`

Deploy app

---

| Field | Value |
|---|---|
| Tag | `sha-abc` |
| Digest | `sha256:pushed` |
| Image ID | `sha256:image-id` |
| Revision | `abc` |
| Source | https://github.com/example/app |

<details>
<summary>⚙️ Runtime</summary>

| Field | Value |
|---|---|
| Entrypoint | `/entrypoint.sh` |
| Command | `./app` |
| Stop signal | `SIGTERM` |
| Storage driver | `overlay2` |
| Exposed ports | `8080/tcp` |
</details>

<details>
<summary>🌱 Environment variables</summary>

| Variable | Value |
|---|---|
| `APP_ENV` | `test` |
| `PATH` | `/bin` |
</details>

<details>
<summary>🔖 Labels</summary>

| Label | Value |
|---|---|
| `a` | `first\|pipe` |
| `backtick` | `run 'cmd'` |
| `org.opencontainers.image.description` | `Deploy app` |
| `org.opencontainers.image.licenses` | `Apache-2.0` |
| `org.opencontainers.image.revision` | `abc` |
| `org.opencontainers.image.source` | `https://github.com/example/app` |
| `org.opencontainers.image.version` | `sha-abc` |
| `z` | `last` |
</details>

<details>
<summary>📦 Layers (2)</summary>

| # | Digest |
|---|---|
| 1 | `sha256:l1` |
| 2 | `sha256:l2` |
</details>

<details>
<summary>📄 Raw JSON</summary>

```json
[
  {
    "Id": "sha256:image-id"
  }
]
```
</details>
