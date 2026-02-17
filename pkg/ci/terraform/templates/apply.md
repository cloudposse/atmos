{{- $target := printf "%s-%s" .Stack .Component -}}
{{- if .Result.HasErrors }}
## Apply Failed for `{{.Component}}` in `{{.Stack}}`
{{- else }}
## Apply Succeeded for `{{.Component}}` in `{{.Stack}}`
{{- end }}

{{- if .Result.HasErrors }}
[![apply](https://shields.io/badge/APPLY-FAILED-critical?style=for-the-badge)](#user-content-apply-{{$target}})
{{- else }}
[![apply](https://shields.io/badge/APPLY-SUCCESS-success?style=for-the-badge)](#user-content-apply-{{$target}})
{{- end }}

{{- if .Result.HasErrors }}
<details><summary><a id="user-content-result-{{$target}}" />:warning: Error summary</summary>
{{- else }}
<details><summary><a id="user-content-result-{{$target}}" />{{if .HasChanges}}Resources: {{.Resources.Create}} added, {{.Resources.Change}} changed, {{.Resources.Destroy}} destroyed{{else}}No changes applied{{end}}</summary>
{{- end }}

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform apply {{.Component}} -s {{.Stack}}
```
</details>

{{- if .Result.HasErrors }}

---
{{- $first := true }}
{{- range .Result.Errors }}
{{ if not $first }}
<!-- -->
{{ end }}
{{- $first = false }}
> [!CAUTION]
> :warning: {{ . }}
{{- end }}
{{- end }}

<details><summary><a id="user-content-apply-{{$target}}" />Terraform <strong>Apply</strong> Summary</summary>

```hcl
{{ .Output }}
```

</details>

{{- if gt (len .Outputs) 0 }}

<details><summary>Terraform Outputs</summary>

| Output | Value |
|--------|-------|
{{- range $name, $output := .Outputs }}
{{- if not $output.Sensitive }}
| `{{ $name }}` | `{{ $output.Value }}` |
{{- else }}
| `{{ $name }}` | *(sensitive)* |
{{- end }}
{{- end }}

</details>
{{- end }}

<details><summary>Metadata</summary>

```json
{
  "component": "{{.Component}}",
  "stack": "{{.Stack}}",
  "commitSHA": "{{if .CI}}{{.CI.SHA}}{{end}}"
}
```
</details>
