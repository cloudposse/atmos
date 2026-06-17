## Kubernetes {{.Command}} Summary for `{{.Component}}` in `{{.Stack}}`

Status: **{{.Status}}**

{{- if gt .ObjectsTotal 0 }}

Objects: **{{.ObjectsTotal}}**
{{- end }}

{{- if gt (len .Counts) 0 }}

| Action | Count |
| --- | ---: |
{{range .Counts }}
| {{.Action}} | {{.Count}} |
{{end }}
{{- end }}

{{- if gt (len .Actions) 0 }}

<details><summary>Object results</summary>

| Action | Resource | Namespace | Name |
| --- | --- | --- | --- |
{{range .Actions }}
| {{.Action}} | {{.Resource}} | {{if .Namespace}}{{.Namespace}}{{else}}-{{end}} | {{.Name}} |
{{end }}

</details>
{{- end }}

{{- if .Error }}

<details><summary>Error</summary>

```text
{{.Error}}
```

</details>
{{- end }}
