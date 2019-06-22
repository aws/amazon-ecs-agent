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

import (
	"io"
	"text/template"
)

// IncludePosition is a type to represent the position of an @INCLUDE directive
// to include external config
type IncludePosition int

// This library assumes that the config is structured as follows:
// 1. Sources/inputs
// 2. Filters
// 3. Outputs
const (
	// HeadOfFile: The external config is included as the first directive in the config file
	HeadOfFile IncludePosition = iota
	// AfterInputs: The external config is included after the input/source directives
	AfterInputs
	// AfterFilters: The external config is included after the filter directives
	AfterFilters
	// EndOfFile: The external config is included at the end of the file
	EndOfFile
)

// FluentConfig is the interface for generating fluentd and fluentbit config
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

// New Creates a new Fluent Config generater
func New() FluentConfig {
	return &FluentConfigGenerator{
		ModifyRecords: make(map[string]RecordModifier),
	}
}

// FluentConfigGenerator implements FluentConfig
// It and all its fields must be public, so that the template library can
// use them when
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

// RecordModifier tracks new fields added to log records
type RecordModifier struct {
	NewFields map[string]string
}

// RegexFilter represents a filter for logs
type RegexFilter struct {
	Regex string
	Key   string
	Tag   string
}

// AddInput add an input/source directive
func (config *FluentConfigGenerator) AddInput(name string, tag string, options map[string]string) FluentConfig {
	config.Inputs = append(config.Inputs, LogPipe{
		Name:    name,
		Tag:     tag,
		Options: options,
	})
	return config
}

// AddIncludeFilter adds a regex filter that only allows logs which match the regex
func (config *FluentConfigGenerator) AddIncludeFilter(regex string, key string, tag string) FluentConfig {
	config.IncludeFilters = append(config.IncludeFilters, RegexFilter{
		Regex: regex,
		Key:   key,
		Tag:   tag,
	})
	return config
}

// AddExcludeFilter adds a regex filter that only allows logs which do not match the regex
func (config *FluentConfigGenerator) AddExcludeFilter(regex string, key string, tag string) FluentConfig {
	config.ExcludeFilters = append(config.ExcludeFilters, RegexFilter{
		Regex: regex,
		Key:   key,
		Tag:   tag,
	})
	return config
}

// AddFieldToRecord adds a key value pair to log records
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

// AddExternalConfig adds an @INCLUDE directive to reference an external config file
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

// AddOutput adds an output/log destination
func (config *FluentConfigGenerator) AddOutput(name string, tag string, options map[string]string) FluentConfig {
	config.Outputs = append(config.Outputs, LogPipe{
		Name:    name,
		Tag:     tag,
		Options: options,
	})
	return config
}

// WriteFluentdConfig outputs the config in Fluentd syntax
func (config *FluentConfigGenerator) WriteFluentdConfig(wr io.Writer) error {
	tmpl, err := template.New("fluent.conf").Parse(fluentDConfigTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(wr, config)
}

// WriteFluentBitConfig outputs the config in Fluent Bit syntax
func (config *FluentConfigGenerator) WriteFluentBitConfig(wr io.Writer) error {
	tmpl, err := template.New("fluent-bit.conf").Parse(fluentBitConfigTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(wr, config)
}
