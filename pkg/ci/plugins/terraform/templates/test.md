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
{{- if gt $test.Error 0 }} [![errored](https://shields.io/badge/ERRORED-{{$test.Error}}-ff0000?style=for-the-badge)](#user-content-result-{{$target}}){{- end }}
{{- if gt $test.Skip 0 }} [![skipped](https://shields.io/badge/SKIPPED-{{$test.Skip}}-inactive?style=for-the-badge)](#user-content-result-{{$target}}){{- end }}
{{- else }} [![failed](https://shields.io/badge/TESTS-FAILED-ff0000?style=for-the-badge)](#user-content-result-{{$target}})
{{- end }}

<a id="result-{{$target}}"></a>

{{- if and $test (gt (len $test.Runs) 0) }}

| Result | File | Run | Duration | Details |
|--------|------|-----|----------|---------|
{{- range $test.Runs }}
{{- if eq .Status "pass" }}
| :white_check_mark: pass | {{ if .File }}`{{ .File }}`{{ end }} | `{{ .Name }}` | {{ if gt .Duration 0.0 }}{{ printf "%.2fs" .Duration }}{{ end }} | |
{{- else if eq .Status "fail" }}
| :x: fail | {{ if .File }}`{{ .File }}`{{ end }} | `{{ .Name }}` | {{ if gt .Duration 0.0 }}{{ printf "%.2fs" .Duration }}{{ end }} | {{ if .File }}`{{ .File }}{{ if gt .Line 0 }}:{{ .Line }}{{ end }}` {{ end }}{{ .Error }} |
{{- else if eq .Status "error" }}
| :boom: error | {{ if .File }}`{{ .File }}`{{ end }} | `{{ .Name }}` | {{ if gt .Duration 0.0 }}{{ printf "%.2fs" .Duration }}{{ end }} | {{ if .File }}`{{ .File }}{{ if gt .Line 0 }}:{{ .Line }}{{ end }}` {{ end }}{{ .Error }} |
{{- else }}
| :fast_forward: skip | {{ if .File }}`{{ .File }}`{{ end }} | `{{ .Name }}` | {{ if gt .Duration 0.0 }}{{ printf "%.2fs" .Duration }}{{ end }} | |
{{- end }}
{{- end }}
{{- end }}

{{- if and $test (or (gt (len $test.Files) 0) (gt (len $test.CleanupFailures) 0)) }}

<details><summary>Detailed test results</summary>

{{- if gt (len $test.Files) 0 }}

| File | Status | Passed | Failed | Errored | Skipped |
|------|--------|--------|--------|---------|---------|
{{- range $test.Files }}
| `{{ .Path }}` | {{ if eq .Status "pass" }}:white_check_mark: pass{{ else if eq .Status "fail" }}:x: fail{{ else if eq .Status "error" }}:boom: error{{ else }}:fast_forward: skip{{ end }} | {{ .Pass }} | {{ .Fail }} | {{ .Error }} | {{ .Skip }} |
{{- end }}
{{- end }}

{{- if gt (len $test.CleanupFailures) 0 }}

### Cleanup failures

{{ range $test.CleanupFailures }}

**{{ if .Run }}`{{ .Run }}`{{ else }}Test cleanup{{ end }}**{{ if .File }} in `{{ .File }}`{{ end }}

{{ range .Resources }}
- `{{ . }}`
{{ end }}
{{ end }}
{{- end }}

</details>
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

<details><summary>Reproduce locally</summary>

```shell
atmos terraform test {{.Component}} -s {{.Stack}}
```

</details>
