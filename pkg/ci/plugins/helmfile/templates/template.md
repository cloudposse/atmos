{{- if .Result.HasErrors }}
## Helmfile Template Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helmfile Template Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

To reproduce locally:

```shell
atmos helmfile template {{ .Component }} -s {{ .Stack }}
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
