package generator

var fluentDConfigTemplate = `{{- range .IncludeConfigHeadOfFile -}}
@include {{ . }}
{{ end -}}
{{- range .Inputs }}
<source>
    @type {{ .Name }}
    {{- if .Tag }}tag {{ .Tag }}{{- end }}
    {{- range $key, $value := .Options }}
    {{ $key }} {{ $value }}
    {{- end }}
</source>
{{ end -}}
{{- range .IncludeConfigAfterInputs }}
@include {{ . }}
{{ end -}}
{{- range .IncludeFilters }}
<filter {{ .Tag }}>
    @type  grep
    <regexp>
        key {{ .Key }}
        pattern {{ .Regex }}
    </regexp>
</filter>
{{ end -}}
{{- range .ExcludeFilters }}
<filter {{ .Tag }}>
    @type  grep
    <exclude>
        key {{ .Key }}
        pattern {{ .Regex }}
    </exclude>
</filter>
{{ end -}}
{{- range $tag, $modifier := .ModifyRecords }}
<filter {{ $tag }}>
    @type record_transformer
    <record>
        {{- range $key, $value := $modifier.NewFields }}
        {{ $key }} {{ $value }}
        {{- end }}
    </record>
</filter>
{{ end -}}
{{- range .IncludeConfigAfterFilters }}
@include {{ . }}
{{ end -}}
{{- range .Outputs }}
<match {{ .Tag }}>
    @type {{ .Name }}
    {{- range $key, $value := .Options }}
    {{ $key }} {{ $value }}
    {{- end }}
</match>
{{ end -}}
{{- range .IncludeConfigEndOfFile }}
@include {{ . }}
{{ end -}}`
