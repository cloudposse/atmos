{{- if .Result.HasErrors }}
## CloudFormation {{ .VerbTitle }} Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation {{ .VerbTitle }} Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

{{- if .ChangeSetName }}

Changeset: **{{ .ChangeSetName }}**
{{- end }}

{{- if and .NoOp (not .Result.HasErrors) }}

No changes
{{- end }}

To reproduce locally:

```shell
atmos aws cloudformation {{ .Verb }} {{ .Component }} -s {{ .Stack }}
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
