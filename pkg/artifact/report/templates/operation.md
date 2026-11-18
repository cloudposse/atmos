### {{.Icon}} {{.Number}}. {{.Title}}

{{.Summary}}

- **Status:** {{.Status}}
{{- if .Step}}
- **Step:** {{.Step}}
{{- end}}
{{- if .Repository}}
- **Repository:** {{.Repository}}
{{- end}}
{{- if .Ref}}
- **Reference:** {{.Ref}}{{if .Requested}} (requested {{.Requested}}){{end}}
{{- end}}
{{- if .Destination}}
- **Destination:** {{.Destination}}
{{- end}}
{{- if .Aliases}}
- **Also resolves from:** {{range $index, $alias := .Aliases}}{{if $index}}, {{end}}{{$alias}}{{end}}
{{- end}}
{{- if .ExactSize}}
- **Size:** {{.ExactSize}}
{{- end}}
{{- if .Duration}}
- **Duration:** {{.Duration}}
{{- end}}
{{- if gt .Attempts 1}}
- **Attempts:** {{.Attempts}} (the report shows the final attempt)
{{- end}}
{{- if .Lock}}
- **Lock:** {{.Lock.Scope}} scope. {{.Lock.Guarantee}}
{{- end}}
{{- if .Signature}}
- **Signature:** {{.Signature.Status}}. {{.Signature.Statement}}
{{- if .Signature.SubjectDigest}}
  - Subject: {{.Signature.SubjectKind}} {{.Signature.SubjectDigest}}
{{- end}}
{{- if .Signature.Policy}}
  - Policy fingerprint: {{.Signature.Policy}}
{{- end}}
{{- range .Signature.Claims}}
  - Verified signer:{{if .Identity}} identity {{.Identity}}{{end}}{{if .Issuer}} issuer {{.Issuer}}{{end}}{{if .KeyID}} key {{.KeyID}}{{end}}
{{- end}}
{{- range .Signature.Evidence}}
  - Evidence:{{if .Format}} {{.Format}}{{end}} {{.Digest}}
{{- end}}
{{- end}}
{{- if .Error}}

**Error**

{{.Error}}
{{- end}}
{{- if .Digests}}

<details>
<summary>Digests ({{len .Digests}})</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
{{range .Digests -}}
| {{.Label}} | {{.Value}} | {{.Meaning}} |
{{end}}
{{.Integrity}}

</details>
{{- end}}
{{- if .Diagnostics}}

<details>
<summary>Diagnostics ({{len .Diagnostics}}{{if .DiagnosticsMore}}, {{.DiagnosticsMore}} more not shown{{end}})</summary>

{{range .Diagnostics -}}
- **{{.Severity}}**{{if .Code}} {{.Code}}{{end}}: {{.Message}}
{{end}}
</details>
{{- end}}
{{- if .Inventory}}

<details>
<summary>Inventory ({{.Inventory.Count}} {{if eq .Inventory.Count 1}}entry{{else}}entries{{end}}{{if .Inventory.Bytes}}, {{.Inventory.Bytes}}{{end}})</summary>

{{range .Inventory.Files -}}
- {{.Path}}{{if .Size}} ({{.Size}}){{end}}
{{end -}}
{{if .Inventory.More}}- … and {{.Inventory.More}} more entries not shown
{{end}}
</details>
{{- end}}
{{- if or .Description .Labels}}

<details>
<summary>Metadata</summary>

{{if .Description -}}
{{.Description}}

{{end -}}
{{range .Labels -}}
- {{.Key}}: {{.Value}}
{{end}}
</details>
{{- end}}
{{- if .Collection}}

<details>
<summary>Repositories ({{len .Collection.Repositories}})</summary>

{{range .Collection.Repositories -}}
- {{.Repository}}: {{.Count}} from {{.Source}}
{{end}}
{{- if .Collection.Artifacts}}
Artifacts:

{{range .Collection.Artifacts -}}
- {{.}}
{{end -}}
{{if .Collection.More}}- … and {{.Collection.More}} more not shown
{{end -}}
{{end}}
</details>
{{- end}}
