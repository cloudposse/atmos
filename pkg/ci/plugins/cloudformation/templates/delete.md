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
atmos aws/cloudformation delete {{ .Component }} -s {{ .Stack }}
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
