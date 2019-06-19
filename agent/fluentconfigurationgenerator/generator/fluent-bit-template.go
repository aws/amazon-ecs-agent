package generator

var fluentBitConfigTemplate = `{{- range .IncludeConfigHeadOfFile -}}
@INCLUDE {{ . }}
{{ end -}}
{{- range .Inputs }}
[INPUT]
    Name {{ .Name }}
    {{- if .Tag }}Tag {{ .Tag }}{{- end }}
    {{- range $key, $value := .Options }}
    {{ $key }} {{ $value }}
    {{- end }}
{{ end -}}
{{- range .IncludeConfigAfterInputs }}
@INCLUDE {{ . }}
{{ end -}}
{{- range .IncludeFilters }}
[FILTER]
    Name   grep
    Match {{ .Tag }}
    Regex  {{ .Key }} {{ .Regex }}
{{ end -}}
{{- range .ExcludeFilters }}
[FILTER]
    Name   grep
    Match {{ .Tag }}
    Exclude {{ .Key }} {{ .Regex }}
{{ end -}}
{{- range $tag, $modifier := .ModifyRecords }}
[FILTER]
    Name record_modifier
    Match {{ $tag }}
    {{- range $key, $value := $modifier.NewFields }}
    Record {{ $key }} {{ $value }}
    {{- end }}
{{ end -}}
{{- range .IncludeConfigAfterFilters }}
@INCLUDE {{ . }}
{{ end -}}
{{- range .Outputs }}
[OUTPUT]
    Name {{ .Name }}
    Match {{ .Tag }}
    {{- range $key, $value := .Options }}
    {{ $key }} {{ $value }}
    {{- end }}
{{ end -}}
{{- range .IncludeConfigEndOfFile }}
@INCLUDE {{ . }}
{{ end -}}`
