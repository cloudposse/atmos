{{- if .Result.HasErrors }}
## Helm Delete Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helm Delete Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

| Field | Value |
| --- | --- |
| Command | `{{ .Command }}` |
| Release | `{{ .ReleaseName }}` |
| Namespace | `{{ .Namespace }}` |

{{- with .Lifecycle }}

### Release lifecycle

| Field | Value |
| --- | --- |
| Operation | `{{ index . "operation" }}` |
| Wait strategy | `{{ index . "wait_strategy" }}` |
| Timeout | `{{ index . "timeout" }}` |
| Chart hooks enabled | `{{ index . "chart_hooks_enabled" }}` |
{{- end }}

To reproduce locally:

```shell
atmos helm delete {{ .Component }} -s {{ .Stack }}
```

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
