{{- if .Result.HasErrors }}
## Helm Diff Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helm Diff Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

| Field | Value |
| --- | --- |
| Command | `{{ .Command }}` |
| Chart | `{{ .Chart }}` |
| Release | `{{ .ReleaseName }}` |
| Namespace | `{{ .Namespace }}` |
| Manifest bytes | `{{ .ManifestBytes }}` |

To reproduce locally:

```shell
atmos helm diff {{ .Component }} -s {{ .Stack }}
```

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
