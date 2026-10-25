{{- $target := .Target -}}
{{- if .Result.HasErrors }}
## Destroy Failed for `{{.Component}}` in `{{.Stack}}`
{{- else if .HasResourceChanges }}
## Destroy Succeeded for `{{.Component}}` in `{{.Stack}}`
{{- else }}
## No Resources Destroyed for `{{.Component}}` in `{{.Stack}}`
{{- end }}

<a href="https://atmos.tools/ci"><picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://atmos.tools/img/atmos-ci-gradient.svg?v={{.AtmosVersion}}">
  <source media="(prefers-color-scheme: light)" srcset="https://atmos.tools/img/atmos-ci-gradient-on-light.svg?v={{.AtmosVersion}}">
  <img src="https://atmos.tools/img/atmos-ci-gradient-on-light.svg?v={{.AtmosVersion}}" alt="Atmos CI" height="32" align="right">
</picture></a>

{{ if .Result.HasErrors -}}
[![failed](https://shields.io/badge/DESTROY-FAILED-ff0000?style=for-the-badge)](#result-{{$target}})
{{- else if .HasResourceChanges }}
[![destroyed](https://shields.io/badge/DESTROY-SUCCEEDED-success?style=for-the-badge)](#result-{{$target}})
{{- else }}
[![no changes](https://shields.io/badge/-NO_CHANGE-inactive?style=for-the-badge)](#result-{{$target}})
{{- end }}

{{- if .Metrics }}

📊 **Resource usage:** **{{ .Metrics.WallTime }}** wall · {{ .Metrics.CPUUser }} user / {{ .Metrics.CPUSys }} sys CPU{{ if .Metrics.PeakMemory }} · **{{ .Metrics.PeakMemory }}** peak memory (largest process){{ end }}
{{- end }}

<details><summary><a id="result-{{$target}}" />{{ if .Result.HasErrors }}Error summary{{ else }}Resources: {{.Resources.Destroy}} destroyed{{ end }}</summary>

{{ if .Result.HasErrors -}}
```hcl
{{ range .Result.Errors }}
{{ . }}
{{ end }}
```
{{- else }}
{{- if gt (len .DeletedResources) 0 }}
```diff
{{- range .DeletedResources }}
- {{ . }}
{{- end }}
```
{{- end }}
{{- if gt (len .Output) 0 }}
```hcl
{{ .Output }}
```
{{- end }}
{{- end }}
</details>

{{- if gt (len .Warnings) 0 }}
{{ range .Warnings }}

> [!WARNING]
> ```
{{ . }}
> ```
{{- end }}
{{- end }}
