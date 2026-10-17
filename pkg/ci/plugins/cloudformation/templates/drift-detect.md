{{- if .Result.HasErrors }}
## CloudFormation Drift Detect Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation Drift Detect Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

{{- if .DriftStatus }}

Drift status: **{{ .DriftStatus }}**
{{- end }}

{{- if or .DriftStatus (gt .DriftedCount 0) }}

Drifted resources: **{{ .DriftedCount }}**
{{- end }}

To reproduce locally:

```shell
atmos aws cloudformation drift detect {{ .Component }} -s {{ .Stack }}
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
