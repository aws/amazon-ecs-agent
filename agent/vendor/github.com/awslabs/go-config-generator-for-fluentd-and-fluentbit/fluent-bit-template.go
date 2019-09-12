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

var fluentBitConfigTemplate = `{{- range .IncludeConfigHeadOfFile -}}
@INCLUDE {{ . }}
{{ end -}}
{{- range .Inputs }}
[INPUT]
    Name {{ .Name }}
    {{- if .Tag }}
    Tag {{ .Tag }}
    {{- end }}
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
