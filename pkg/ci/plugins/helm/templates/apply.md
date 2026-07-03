{{- if .Result.HasErrors }}
## Helm Apply Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helm Apply Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

| Field | Value |
| --- | --- |
| Command | `{{ .Command }}` |
| Chart | `{{ .Chart }}` |
| Release | `{{ .ReleaseName }}` |
| Namespace | `{{ .Namespace }}` |
| Target | `{{ .Target }}` |
| Objects | `{{ .ObjectCount }}` |
| Manifest bytes | `{{ .ManifestBytes }}` |

To reproduce locally:

```shell
atmos helm apply {{ .Component }} -s {{ .Stack }}
```

{{- if .ObjectKinds }}

Applied kinds: `{{ range $i, $kind := .ObjectKinds }}{{ if $i }}`, `{{ end }}{{ $kind }}{{ end }}`
{{- end }}

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
