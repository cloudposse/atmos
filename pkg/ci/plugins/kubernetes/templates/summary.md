## Kubernetes {{.Command}} Summary for `{{.Component}}` in `{{.Stack}}`

Status: **{{.Status}}**

{{- if gt .ObjectsTotal 0 }}

Objects: **{{.ObjectsTotal}}**
{{- end }}

{{- if gt (len .Counts) 0 }}

<!-- markdownlint-disable MD055 MD056 -->
| Action | Count |
| --- | ---: |
{{range .Counts }}
| {{.Action}} | {{.Count}} |
{{end }}
<!-- markdownlint-enable MD055 MD056 -->
{{- end }}

{{- if gt (len .Actions) 0 }}

<details><summary>Object results</summary>

<!-- markdownlint-disable MD055 MD056 -->
| Action | Resource | Namespace | Name |
| --- | --- | --- | --- |
{{range .Actions }}
| {{.Action}} | {{.Resource}} | {{if .Namespace}}{{.Namespace}}{{else}}-{{end}} | {{.Name}} |
{{end }}
<!-- markdownlint-enable MD055 MD056 -->

</details>
{{- end }}

{{- if .Error }}

<details><summary>Error</summary>

```text
{{.Error}}
```

</details>
{{- end }}
