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

{{- if eq (index . "reason") "external_target" }}

| Field | Value |
| --- | --- |
| Deleted | `false` |
| Target kind | `{{ index . "target_kind" }}` |
| Reason | `external_target` |

{{- else }}

| Field | Value |
| --- | --- |
| Operation | `{{ index . "operation" }}` |
| Wait strategy | `{{ index (index . "wait") "strategy" }}` |
| Timeout | `{{ index . "timeout" }}` |
| Chart hooks enabled | `{{ index . "chart_hooks" }}` |

{{- end }}

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
