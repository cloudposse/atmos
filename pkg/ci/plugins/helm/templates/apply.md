{{- if .Result.HasErrors }}
## Helm Apply Failed for `{{ .Component }}` in `{{ .Stack }}`
{{- else }}
## Helm Apply Summary for `{{ .Component }}` in `{{ .Stack }}`
{{- end }}

| Field | Value |
| --- | --- |
| Command | `{{ .Command }}` |
| Chart | `{{ .Chart }}` |
| Release | `{{ .ReleaseName }}` |
| Namespace | `{{ .Namespace }}` |
| Target | `{{ .Target }}` |
| Objects | `{{ .ObjectCount }}` |
| Manifest bytes | `{{ .ManifestBytes }}` |

{{- with .Lifecycle }}

### Release lifecycle

{{- if eq (index . "reason") "external_target" }}

| Field | Value |
| --- | --- |
| Applied | `false` |
| Target kind | `{{ index . "target_kind" }}` |
| Reason | `external_target` |
{{- else }}

| Field | Value |
| --- | --- |
| Operation | `{{ index . "operation" }}` |
| Wait strategy | `{{ index . "wait_strategy" }}` |
| Timeout | `{{ index . "timeout" }}` |
| Chart hooks enabled | `{{ index . "chart_hooks_enabled" }}` |
| Wait for Jobs | `{{ index . "wait_for_jobs" }}` |
| Rollback on failure | `{{ index . "rollback_on_failure" }}` |
{{- if eq (index . "operation") "install" }}
| Install CRDs | `{{ index . "install_crds" }}` |
{{- end }}
{{- if eq (index . "operation") "upgrade" }}
| Cleanup on failure | `{{ index . "cleanup_on_fail" }}` |
| Maximum history | `{{ index . "max_history" }}` |
{{- end }}
{{- end }}
{{- end }}

To reproduce locally:

```shell
atmos helm apply {{ .Component }} -s {{ .Stack }}
```

{{- if .ObjectKinds }}

Applied kinds: `{{ range $i, $kind := .ObjectKinds }}{{ if $i }}`, `{{ end }}{{ $kind }}{{ end }}`
{{- end }}

{{- if .Result.HasErrors }}

<details><summary>Error</summary>

```text
{{ range .Result.Errors }}{{ . }}
{{ end }}
```

</details>
{{- end }}
