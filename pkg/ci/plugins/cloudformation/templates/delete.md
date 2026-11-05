{{- if .Result.HasErrors }}
## CloudFormation Delete Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation Delete Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

To reproduce locally:

```shell
atmos aws cloudformation delete {{ .Component }} -s {{ .Stack }}
```

{{- if .Output }}

<details><summary>CloudFormation output</summary>

{{ .CodeFence }}text
{{ .Output }}
{{ .CodeFence }}

</details>
{{- end }}

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

{{ .CodeFence }}text
{{ range .Result.Errors }}{{ . }}
{{ end }}
{{ .CodeFence }}

</details>
{{- end }}
