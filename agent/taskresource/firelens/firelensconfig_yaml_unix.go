//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package firelens

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"text/template"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"

	generator "github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// yamlFluentBitConfig holds all the information needed to render a YAML formatted Fluent Bit config file for a
// firelens container. It intentionally mirrors the fields of generator.FluentConfigGenerator (and reuses its
// exported types), since go-config-generator-for-fluentd-and-fluentbit only knows how to render the classic
// INI-style format and has no YAML writer.
type yamlFluentBitConfig struct {
	Inputs         []generator.LogPipe
	IncludeFilters []generator.RegexFilter
	ExcludeFilters []generator.RegexFilter
	ModifyRecords  map[string]generator.RecordModifier
	Outputs        []generator.LogPipe
	// Includes are external YAML config file paths referenced via the top level "includes:" section, which is
	// Fluent Bit YAML's equivalent of classic mode's "@INCLUDE" directive.
	Includes []string
}

// yamlFluentBitConfigTemplate renders a yamlFluentBitConfig as a Fluent Bit YAML config document. It's structured
// analogously to the classic mode template in fluent-bit-template.go, except that "includes" is a top level
// section (Fluent Bit's YAML format has no equivalent of classic mode's positional "@INCLUDE" directive) and
// plugin properties that can repeat (e.g. record_modifier's "Record") are represented as a YAML list rather than
// as repeated keys, since YAML mappings can't have duplicate keys.
var yamlFluentBitConfigTemplate = `{{- if .Includes }}
includes:
{{- range .Includes }}
  - {{ . | quoteYAML }}
{{- end }}
{{ end -}}
pipeline:
  inputs:
{{- range .Inputs }}
    - name: {{ .Name | quoteYAML }}
{{- if .Tag }}
      tag: {{ .Tag | quoteYAML }}
{{- end }}
{{- range $key, $value := .Options }}
      {{ $key }}: {{ $value | quoteYAML }}
{{- end }}
{{ end -}}
{{- if or .IncludeFilters .ExcludeFilters .ModifyRecords }}
  filters:
{{- range .IncludeFilters }}
    - name: grep
      match: {{ .Tag | quoteYAML }}
      regex: {{ printf "%s %s" .Key .Regex | quoteYAML }}
{{ end -}}
{{- range .ExcludeFilters }}
    - name: grep
      match: {{ .Tag | quoteYAML }}
      exclude: {{ printf "%s %s" .Key .Regex | quoteYAML }}
{{ end -}}
{{- range $tag, $modifier := .ModifyRecords }}
    - name: record_modifier
      match: {{ $tag | quoteYAML }}
      record:
{{- range $key, $value := $modifier.NewFields }}
        - {{ printf "%s %s" $key $value | quoteYAML }}
{{- end }}
{{ end -}}
{{- end }}
{{- if .Outputs }}
  outputs:
{{- range .Outputs }}
    - name: {{ .Name | quoteYAML }}
      match: {{ .Tag | quoteYAML }}
{{- range $key, $value := .Options }}
      {{ $key }}: {{ $value | quoteYAML }}
{{- end }}
{{ end -}}
{{- end }}
`

// quoteYAML renders s as a double-quoted YAML scalar. Values coming from task definitions (log options, ARNs,
// ECS metadata, etc.) are arbitrary strings that may contain YAML-significant characters (":", "#", leading
// "*"/"&"/"!", etc.), so every scalar is quoted defensively rather than relying on plain scalar rules.
func quoteYAML(s string) string {
	return strconv.Quote(s)
}

// generateYAMLConfig builds a yamlFluentBitConfig that contains all necessary information to construct a YAML
// formatted Fluent Bit config file for a firelens container. It's the YAML counterpart to generateConfig() and
// must be kept in sync with it; it's only used for fluentbit firelens containers with a YAML formatted external
// config (Create() fails fast for any other combination).
func (firelens *FirelensResource) generateYAMLConfig() (*yamlFluentBitConfig, error) {
	cfg := &yamlFluentBitConfig{
		ModifyRecords: make(map[string]generator.RecordModifier),
	}

	cfg.Inputs = append(cfg.Inputs, generator.LogPipe{
		Name: inputNameForward,
		Options: map[string]string{
			socketInputPathOptionFluentbit: socketPath,
			memBufLimitOptionFluentBit:     firelens.resolveMemBufLimit(),
		},
	})

	matchAnyWildcard := matchAnyWildcardFluentbit

	if firelens.networkMode == bridgeNetworkMode || firelens.networkMode == awsvpcNetworkMode {
		var inputBindValue string
		if firelens.networkMode == bridgeNetworkMode {
			inputBindValue = inputBridgeBindValue
		} else {
			inputBindValue = inputAWSVPCBindValue
		}
		cfg.Inputs = append(cfg.Inputs, generator.LogPipe{
			Name: inputNameForward,
			Options: map[string]string{
				inputPortOptionFluentbit:   inputPortValue,
				inputListenOptionFluentbit: inputBindValue,
			},
		})

		// Healthcheck input/output sections, mirroring addHealthcheckSections() for classic mode.
		cfg.Inputs = append(cfg.Inputs, generator.LogPipe{
			Name: healthcheckInputNameFluentbit,
			Tag:  healthcheckTag,
			Options: map[string]string{
				inputPortOptionFluentbit:   healthcheckInputPortValue,
				inputListenOptionFluentbit: healthcheckInputBindValue,
			},
		})
		cfg.Outputs = append(cfg.Outputs, generator.LogPipe{Name: healthcheckOutputName, Tag: healthcheckTag})
	}

	if firelens.ecsMetadataEnabled {
		modifier := generator.RecordModifier{NewFields: map[string]string{
			"ecs_cluster":         firelens.cluster,
			"ecs_task_arn":        firelens.taskARN,
			"ecs_task_definition": firelens.taskDefinition,
		}}
		if firelens.ec2InstanceID != "" {
			modifier.NewFields["ec2_instance_id"] = firelens.ec2InstanceID
		}
		cfg.ModifyRecords[matchAnyWildcard] = modifier
	}

	for containerName, logOptions := range firelens.containerToLogOptions {
		tag := fmt.Sprintf(fluentTagOutputFormat, containerName, matchAnyWildcard)
		if err := addOutputSectionYAML(tag, logOptions, cfg); err != nil {
			return nil, fmt.Errorf("unable to apply log options of container %s to firelens config: %v", containerName, err)
		}
	}

	switch firelens.externalConfigType {
	case ExternalConfigTypeFile:
		cfg.Includes = append(cfg.Includes, firelens.externalConfigValue)
	case ExternalConfigTypeS3:
		cfg.Includes = append(cfg.Includes, S3ConfigPathFluentbitYAML)
	}
	seelog.Infof("Included external firelens config file at: %s", firelens.externalConfigValue)

	return cfg, nil
}

// addOutputSectionYAML is the YAML counterpart to addOutputSection (see firelensconfig_unix.go for details on the
// expected shape of logOptions). It must be kept in sync with addOutputSection.
func addOutputSectionYAML(tag string, logOptions map[string]string, cfg *yamlFluentBitConfig) error {
	outputOptions := make(map[string]string)
	for key, value := range logOptions {
		switch key {
		case outputTypeLogOptionKeyFluentbit:
			continue
		case includePatternKey:
			cfg.IncludeFilters = append(cfg.IncludeFilters, generator.RegexFilter{Regex: value, Key: "log", Tag: tag})
		case excludePatternKey:
			cfg.ExcludeFilters = append(cfg.ExcludeFilters, generator.RegexFilter{Regex: value, Key: "log", Tag: tag})
		default:
			outputOptions[key] = value
		}
	}

	output, ok := logOptions[outputTypeLogOptionKeyFluentbit]
	if len(outputOptions) > 0 && !ok {
		return errors.Errorf("missing output key %s which is required for firelens configuration of type %s",
			outputTypeLogOptionKeyFluentbit, FirelensConfigTypeFluentbit)
	} else if !ok {
		return nil
	}

	cfg.Outputs = append(cfg.Outputs, generator.LogPipe{Name: output, Tag: tag, Options: outputOptions})
	return nil
}

// writeFluentBitYAMLConfig renders cfg as a YAML formatted Fluent Bit config and writes it to wr.
func writeFluentBitYAMLConfig(wr io.Writer, cfg *yamlFluentBitConfig) error {
	tmpl, err := template.New("fluent-bit.yaml").Funcs(template.FuncMap{"quoteYAML": quoteYAML}).Parse(yamlFluentBitConfigTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(wr, cfg)
}

// generateYAMLConfigFile generates a YAML formatted firelens config file at
// $(RESOURCE_DIR)/config/fluent-bit.yaml. It's the YAML counterpart to generateConfigFile(), used only when the
// external firelens config is YAML formatted (see usesYAMLFluentBitConfig()).
func (firelens *FirelensResource) generateYAMLConfigFile() error {
	cfg, err := firelens.generateYAMLConfig()
	if err != nil {
		return errors.Wrap(err, "unable to generate firelens yaml config")
	}

	confFilePath := filepath.Join(firelens.resourceDir, "config", "fluent-bit.yaml")
	err = firelens.writeConfigFile(func(file oswrapper.File) error {
		return writeFluentBitYAMLConfig(file, cfg)
	}, confFilePath)
	if err != nil {
		return errors.Wrapf(err, "unable to generate firelens yaml config file")
	}

	seelog.Infof("Generated firelens yaml config file at: %s", confFilePath)
	return nil
}
