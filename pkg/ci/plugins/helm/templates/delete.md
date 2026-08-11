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
