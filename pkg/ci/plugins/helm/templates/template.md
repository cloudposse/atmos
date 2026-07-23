{{- if .Result.HasErrors }}
## Helm Template Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helm Template Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

| Field | Value |
| --- | --- |
| Command | `{{ .Command }}` |
| Chart | `{{ .Chart }}` |
| Release | `{{ .ReleaseName }}` |
| Namespace | `{{ .Namespace }}` |
| Objects | `{{ .ObjectCount }}` |
| Manifest bytes | `{{ .ManifestBytes }}` |

To reproduce locally:

```shell
atmos helm template {{ .Component }} -s {{ .Stack }}
```

{{- if .ObjectKinds }}

Rendered kinds: `{{ range $i, $kind := .ObjectKinds }}{{ if $i }}`, `{{ end }}{{ $kind }}{{ end }}`
{{- end }}

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
