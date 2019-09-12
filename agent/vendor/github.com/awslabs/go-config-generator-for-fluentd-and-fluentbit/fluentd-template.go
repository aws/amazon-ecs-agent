// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package generator

var fluentDConfigTemplate = `{{- range .IncludeConfigHeadOfFile -}}
@include {{ . }}
{{ end -}}
{{- range .Inputs }}
<source>
    @type {{ .Name }}
    {{- if .Tag }}
    tag {{ .Tag }}
    {{- end }}
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
