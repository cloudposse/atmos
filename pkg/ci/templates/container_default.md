{{"\n"}}## 🐳 {{.Image}}{{range .Badges}}  &nbsp; `{{.}}`{{end}}

{{if .Description}}{{.Description}}

{{end}}---

| Field | Value |
|---|---|
| Tag | {{.Tag}} |
| Digest | {{.Digest}} |
| Image ID | {{.ID}} |
| Revision | {{.Revision}} |
| Source | {{.Source}} |

<details>
<summary>⚙️ Runtime</summary>

| Field | Value |
|---|---|
| Entrypoint | {{.Entrypoint}} |
| Command | {{.Command}} |
| Stop signal | {{.StopSignal}} |
| Storage driver | {{.StorageDriver}} |
| Exposed ports | {{.ExposedPorts}} |
</details>

{{if .Env}}<details>
<summary>🌱 Environment variables</summary>

| Variable | Value |
|---|---|
{{range .Env}}| {{.Name}} | {{.Value}} |
{{end}}</details>

{{end}}{{if .Labels}}<details>
<summary>🔖 Labels</summary>

| Label | Value |
|---|---|
{{range .Labels}}| {{.Name}} | {{.Value}} |
{{end}}</details>

{{end}}{{if .Layers}}<details>
<summary>📦 Layers ({{len .Layers}})</summary>

| # | Digest |
|---|---|
{{range .Layers}}| {{.Index}} | {{.Digest}} |
{{end}}</details>

{{end}}{{if .RawJSON}}<details>
<summary>📄 Raw JSON</summary>

```json
{{.RawJSON}}
```
</details>

{{end -}}
