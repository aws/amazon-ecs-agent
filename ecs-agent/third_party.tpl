{{ range . -}}
## {{.Name}} ([{{.LicenseName}}]({{.LicenseURL}}))

```
{{- .LicenseText -}}
```
{{ end }}
