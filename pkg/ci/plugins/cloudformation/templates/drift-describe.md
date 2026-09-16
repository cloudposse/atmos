{{- if .Result.HasErrors }}
## CloudFormation Drift Describe Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation Drift Describe Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

{{- if .DriftStatus }}

Drift status: **{{ .DriftStatus }}**
{{- end }}

{{- if gt .DriftedCount 0 }}

Drifted resources: **{{ .DriftedCount }}**
{{- end }}

To reproduce locally:

```shell
atmos aws/cloudformation drift describe {{ .Component }} -s {{ .Stack }}
```

{{- if .Output }}

<details><summary>CloudFormation output</summary>

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
