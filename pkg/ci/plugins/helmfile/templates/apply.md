{{- if .Result.HasErrors }}
## Helmfile Apply Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helmfile Apply Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

To reproduce locally:

```shell
atmos helmfile apply {{ .Component }} -s {{ .Stack }}
```

{{- if .Output }}

<details><summary>Helmfile output</summary>

```text
{{ .Output }}
```

</details>
{{- end }}

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
