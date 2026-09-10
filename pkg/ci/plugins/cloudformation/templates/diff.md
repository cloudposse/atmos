{{- if .Result.HasErrors }}
## CloudFormation Diff Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation Diff Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

{{- if gt .ResourceChanges 0 }}

Resource changes: **{{ .ResourceChanges }}**
{{- end }}

To reproduce locally:

```shell
atmos aws/cloudformation diff {{ .Component }} -s {{ .Stack }}
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
