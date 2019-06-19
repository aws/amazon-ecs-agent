package generator

import (
	"io"
	"text/template"
)

type IncludePosition int

const (
	HeadOfFile IncludePosition = iota
	AfterInputs
	AfterFilters
	EndOfFile
)

type FluentConfig interface {
	AddInput(name string, tag string, options map[string]string) FluentConfig
	AddIncludeFilter(regex string, key string, tag string) FluentConfig
	AddExcludeFilter(regex string, key string, tag string) FluentConfig
	AddFieldToRecord(key string, value string, tag string) FluentConfig
	AddExternalConfig(filePath string, position IncludePosition) FluentConfig
	AddOutput(name string, tag string, options map[string]string) FluentConfig
	WriteFluentdConfig(wr io.Writer) error
	WriteFluentBitConfig(wr io.Writer) error
}

func New() FluentConfig {
	return &FluentConfigGenerator{
		ModifyRecords: make(map[string]RecordModifier),
	}
}

type FluentConfigGenerator struct {
	Inputs                    []LogPipe
	ModifyRecords             map[string]RecordModifier
	IncludeFilters            []RegexFilter
	ExcludeFilters            []RegexFilter
	IncludeConfigHeadOfFile   []string
	IncludeConfigAfterInputs  []string
	IncludeConfigAfterFilters []string
	IncludeConfigEndOfFile    []string
	Outputs                   []LogPipe
}

// LogPipe can represent an input or an output plugin
type LogPipe struct {
	Name    string
	Tag     string
	Options map[string]string
}

type RecordModifier struct {
	NewFields map[string]string
}

type RegexFilter struct {
	Regex string
	Key   string
	Tag   string
}

func (config *FluentConfigGenerator) AddInput(name string, tag string, options map[string]string) FluentConfig {
	config.Inputs = append(config.Inputs, LogPipe{
		Name:    name,
		Tag:     tag,
		Options: options,
	})
	return config
}

func (config *FluentConfigGenerator) AddIncludeFilter(regex string, key string, tag string) FluentConfig {
	config.IncludeFilters = append(config.IncludeFilters, RegexFilter{
		Regex: regex,
		Key:   key,
		Tag:   tag,
	})
	return config
}

func (config *FluentConfigGenerator) AddExcludeFilter(regex string, key string, tag string) FluentConfig {
	config.ExcludeFilters = append(config.ExcludeFilters, RegexFilter{
		Regex: regex,
		Key:   key,
		Tag:   tag,
	})
	return config
}

func (config *FluentConfigGenerator) AddFieldToRecord(key string, value string, tag string) FluentConfig {
	_, ok := config.ModifyRecords[tag]
	if !ok {
		config.ModifyRecords[tag] = RecordModifier{
			NewFields: make(map[string]string),
		}
	}
	config.ModifyRecords[tag].NewFields[key] = value

	return config
}

func (config *FluentConfigGenerator) AddExternalConfig(filePath string, position IncludePosition) FluentConfig {
	switch position {
	case HeadOfFile:
		config.IncludeConfigHeadOfFile = append(config.IncludeConfigHeadOfFile, filePath)
	case AfterInputs:
		config.IncludeConfigAfterInputs = append(config.IncludeConfigAfterInputs, filePath)
	case AfterFilters:
		config.IncludeConfigAfterFilters = append(config.IncludeConfigAfterFilters, filePath)
	case EndOfFile:
		config.IncludeConfigEndOfFile = append(config.IncludeConfigEndOfFile, filePath)
	}

	return config
}

func (config *FluentConfigGenerator) AddOutput(name string, tag string, options map[string]string) FluentConfig {
	config.Outputs = append(config.Outputs, LogPipe{
		Name:    name,
		Tag:     tag,
		Options: options,
	})
	return config
}

func (config *FluentConfigGenerator) WriteFluentdConfig(wr io.Writer) error {
	tmpl, err := template.New("fluent.conf").Parse(fluentDConfigTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(wr, config)
}
func (config *FluentConfigGenerator) WriteFluentBitConfig(wr io.Writer) error {
	tmpl, err := template.New("fluent-bit.conf").Parse(fluentBitConfigTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(wr, config)
}
