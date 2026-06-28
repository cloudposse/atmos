{{- $target := .Target -}}
{{- $test := .TestResult -}}
{{- if .Result.HasErrors }}
## Tests Failed for `{{.Component}}` in `{{.Stack}}`
{{- else }}
## Tests Passed for `{{.Component}}` in `{{.Stack}}`
{{- end }}

<a href="https://atmos.tools/ci"><img src="https://atmos.tools/img/atmos-ci-gradient.svg" alt="Atmos CI" width="220px" align="right"/></a>

{{- if $test }}
{{- if gt $test.Total 0 }} [![total](https://shields.io/badge/TESTS-{{$test.Total}}-blue?style=for-the-badge)](#user-content-result-{{$target}}){{- end }}
{{- if gt $test.Pass 0 }} [![passed](https://shields.io/badge/PASSED-{{$test.Pass}}-success?style=for-the-badge)](#user-content-result-{{$target}}){{- end }}
{{- if gt $test.Fail 0 }} [![failed](https://shields.io/badge/FAILED-{{$test.Fail}}-critical?style=for-the-badge)](#user-content-result-{{$target}}){{- end }}
{{- if gt $test.Skip 0 }} [![skipped](https://shields.io/badge/SKIPPED-{{$test.Skip}}-inactive?style=for-the-badge)](#user-content-result-{{$target}}){{- end }}
{{- else }} [![failed](https://shields.io/badge/TESTS-FAILED-ff0000?style=for-the-badge)](#user-content-result-{{$target}})
{{- end }}

<details><summary><a id="result-{{$target}}" />Test results</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform test {{.Component}} -s {{.Stack}}
```

---

{{- if and $test (gt (len $test.Runs) 0) }}

| Result | Run |
|--------|-----|
{{- range $test.Runs }}
{{- if eq .Status "pass" }}
| :white_check_mark: pass | `{{ .Name }}` |
{{- else if eq .Status "fail" }}
| :x: fail | `{{ .Name }}` |
{{- else }}
| :fast_forward: skip | `{{ .Name }}` |
{{- end }}
{{- end }}
{{- end }}

{{- if .Result.HasErrors }}
{{- if gt (len .Result.Errors) 0 }}

```hcl
{{ range .Result.Errors }}
{{ . }}
{{ end }}
```
{{- end }}
{{- end }}
</details>

{{- if gt (len .Output) 0 }}

<details><summary>Terraform <strong>Test</strong> Output</summary>

```hcl
{{ .Output }}
```

</details>
{{- end }}
