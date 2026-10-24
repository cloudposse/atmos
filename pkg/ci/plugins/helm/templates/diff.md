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

To reproduce locally:

```shell
atmos helm diff {{ .Component }} -s {{ .Stack }}
```

{{- if .Diff }}

<details><summary>Helm Diff</summary>

```diff
{{ .Diff }}
```

</details>
{{- else if not .Result.HasErrors }}

No changes. The rendered chart matches the baseline.
{{- end }}

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
