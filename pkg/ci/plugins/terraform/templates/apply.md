{{- $target := .Target -}}
{{- if .Result.HasErrors }}
## Apply Failed for `{{.Component}}` in `{{.Stack}}`
{{- else if .HasChanges }}
## Apply Succeeded for `{{.Component}}` in `{{.Stack}}`
{{- else }}
## No Changes Applied for `{{.Component}}` in `{{.Stack}}`
{{- end }}

<a href="https://cloudposse.com/"><img src="https://cloudposse.com/logo-300x69.svg" width="100px" align="right"/></a>

{{- if .Result.HasErrors }}
[![failed](https://shields.io/badge/APPLY-FAILED-ff0000?style=for-the-badge)](#user-content-result-{{$target}})
{{- else -}}
{{- if gt .Resources.Create 0 }} [![create](https://shields.io/badge/APPLY-CREATE-success?style=for-the-badge)](#user-content-create-{{$target}})
{{- end -}}
{{- if gt .Resources.Change 0 }} [![change](https://shields.io/badge/APPLY-CHANGE-important?style=for-the-badge)](#user-content-change-{{$target}})
{{- end -}}
{{- if gt .Resources.Destroy 0 }} [![destroy](https://shields.io/badge/APPLY-DESTROY-critical?style=for-the-badge)](#user-content-destroy-{{$target}})
{{- end -}}
{{- if not .HasChanges }} [![no changes](https://shields.io/badge/-NO_CHANGE-inactive?style=for-the-badge)](#user-content-result-{{$target}})
{{- end }}
{{- end }}

{{- if .HasDestroy }}

> [!CAUTION]
> **Terraform destroyed resources!**
> This apply deleted resources. Please verify the result carefully.
{{- end }}

{{- if .Result.HasErrors }}
<details><summary><a id="result-{{$target}}" />:warning: Error summary</summary>
{{- else if .HasChanges }}
<details><summary><a id="result-{{$target}}" />Resources: {{.Resources.Create}} added, {{.Resources.Change}} changed, {{.Resources.Destroy}} destroyed</summary>
{{- else }}
<details><summary><a id="result-{{$target}}" />No changes applied</summary>
{{- end }}

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform apply {{.Component}} -s {{.Stack}}
```

---

{{- if not .Result.HasErrors }}
{{- if gt (len .CreatedResources) 0 }}

### <a id="create-{{$target}}" />Created
```diff
{{- range .CreatedResources }}
+ {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .UpdatedResources) 0 }}
### <a id="change-{{$target}}" />Changed
```diff
{{- range .UpdatedResources }}
~ {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .DeletedResources) 0 }}
### <a id="destroy-{{$target}}" />Destroyed
```diff
{{- range .DeletedResources }}
- {{ . }}
{{- end }}
```
{{- end }}
{{- else }}
```hcl
{{ range .Result.Errors }}
{{ . }}
{{ end }}
```
{{- end }}
</details>

{{- if not .Result.HasErrors }}
{{- if gt (len .Output) 0 }}

<details><summary>Terraform <strong>Apply</strong> Summary</summary>

```hcl
{{ .Output }}
```

</details>
{{- end }}

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
{{- end }}
