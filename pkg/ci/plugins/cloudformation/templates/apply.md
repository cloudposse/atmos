{{- if .Result.HasErrors }}
## CloudFormation Apply Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation Apply Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

{{- if .ChangeSetName }}

Changeset: **{{ .ChangeSetName }}**
{{- end }}

To reproduce locally:

```shell
atmos aws cloudformation apply {{ .Component }} -s {{ .Stack }}
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
