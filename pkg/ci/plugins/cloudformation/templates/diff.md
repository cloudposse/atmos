{{- if .Result.HasErrors }}
## CloudFormation {{ .VerbTitle }} Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## CloudFormation {{ .VerbTitle }} Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

{{- if .StackName }}

Stack: **{{ .StackName }}**
{{- end }}

{{- if and .NoOp (not .Result.HasErrors) }}

No changes
{{- else if gt .ResourceChanges 0 }}

Resource changes: **{{ .ResourceChanges }}**
{{- end }}

To reproduce locally:

```shell
atmos aws cloudformation {{ .Verb }} {{ .Component }} -s {{ .Stack }}
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
