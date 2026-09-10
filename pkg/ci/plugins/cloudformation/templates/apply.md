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
atmos aws/cloudformation apply {{ .Component }} -s {{ .Stack }}
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
