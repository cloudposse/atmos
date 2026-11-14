{{"\n"}}## {{.Heading}}

{{.Overview}}

<!-- markdownlint-disable MD055 MD056 -->
| # | Operation | Status | Artifact | Repository | Destination | Digest | Size | Duration |
| ---: | --- | --- | --- | --- | --- | --- | ---: | ---: |
{{range .Rows -}}
| {{.Number}} | {{.Operation}} | {{.Icon}} {{.Status}} | {{.Artifact}} | {{.Repository}} | {{.Destination}} | {{.Digest}} | {{.Size}} | {{.Duration}} |
{{end -}}
<!-- markdownlint-enable MD055 MD056 -->
{{range .Sections}}
{{.}}
{{end -}}
{{if .Notice}}
> [!WARNING]
> {{.Notice}}
{{end}}
