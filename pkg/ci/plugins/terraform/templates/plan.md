{{- $target := .Target -}}
{{- if .Result.HasErrors }}
## Plan Failed for `{{.Component}}` in `{{.Stack}}`
{{- else if .HasChanges }}
## Changes Found for `{{.Component}}` in `{{.Stack}}`
{{- else }}
## No Changes for `{{.Component}}` in `{{.Stack}}`
{{- end }}

<a href="https://cloudposse.com/"><img src="https://cloudposse.com/logo-300x69.svg" width="100px" align="right"/></a>

{{- if .Result.HasErrors }}
[![failed](https://shields.io/badge/PLAN-FAILED-ff0000?style=for-the-badge)](#user-content-result-{{$target}})
{{- else }}
{{- if gt .Resources.Create 0 }}
[![create](https://shields.io/badge/PLAN-CREATE-success?style=for-the-badge)](#user-content-create-{{$target}})
{{- end }}
{{- if gt .Resources.Change 0 }}
[![change](https://shields.io/badge/PLAN-CHANGE-important?style=for-the-badge)](#user-content-change-{{$target}})
{{- end }}
{{- if gt .Resources.Replace 0 }}
[![replace](https://shields.io/badge/PLAN-REPLACE-critical?style=for-the-badge)](#user-content-replace-{{$target}})
{{- end }}
{{- if gt .Resources.Destroy 0 }}
[![destroy](https://shields.io/badge/PLAN-DESTROY-critical?style=for-the-badge)](#user-content-destroy-{{$target}})
{{- end }}
{{- if not .HasChanges }}
[![no changes](https://shields.io/badge/-NO_CHANGE-inactive?style=for-the-badge)](#user-content-result-{{$target}})
{{- end }}
{{- end }}

{{- if .HasDestroy }}

> [!CAUTION]
> **Terraform will delete resources!**
> This plan contains resource delete operations. Please check the plan result very carefully.
{{- end }}

{{- if .Result.HasErrors }}
<details><summary><a id="result-{{$target}}" />:warning: Error summary</summary>
{{- else }}
<details><summary><a id="result-{{$target}}" />Plan: {{.Resources.Create}} to add, {{.Resources.Change}} to change, {{.Resources.Destroy}} to destroy.</summary>
{{- end }}

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan {{.Component}} -s {{.Stack}}
```

---

{{- if .Result.HasErrors }}
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

{{- if not .Result.HasErrors }}
{{- if gt (len .CreatedResources) 0 }}

### <a id="create-{{$target}}" />Create
```diff
{{- range .CreatedResources }}
+ {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .UpdatedResources) 0 }}
### <a id="change-{{$target}}" />Change
```diff
{{- range .UpdatedResources }}
~ {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .ReplacedResources) 0 }}
### <a id="replace-{{$target}}" />Replace
```diff
{{- range .ReplacedResources }}
-/+ {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .DeletedResources) 0 }}
### <a id="destroy-{{$target}}" />Destroy
```diff
{{- range .DeletedResources }}
- {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .ImportedResources) 0 }}
### <a id="import-{{$target}}" />Import
```diff
{{- range .ImportedResources }}
<= {{ . }}
{{- end }}
```
{{- end }}
{{- end }}
</details>

{{- if gt (len .Output) 0 }}

<details><summary>Terraform <strong>Plan</strong> Summary</summary>

```hcl
{{ .Output }}
```

</details>
{{- end }}

{{- if gt (len .Warnings) 0 }}
{{ range .Warnings }}

> [!WARNING]
> ```
{{ . }}
> ```
{{- end }}
{{- end }}
