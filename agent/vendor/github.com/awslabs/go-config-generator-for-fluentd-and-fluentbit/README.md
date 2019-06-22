## Go Config Generator For Fluentd And Fluentbit

A Go Library for programmatically generating Fluentd and Fluent Bit Configuration.

For example usage, see the [unit test file](generator_test.go).

The library exports the following interface:

```
type FluentConfig interface {
	// AddInput adds an input/source directive
	AddInput(name string, tag string, options map[string]string) FluentConfig

	// AddIncludeFilter adds a regex filter that only allows logs which match the regex
	AddIncludeFilter(regex string, key string, tag string) FluentConfig

	// AddExcludeFilter adds a regex filter that only allows logs which do not match the regex
	AddExcludeFilter(regex string, key string, tag string) FluentConfig

	// AddFieldToRecord adds a key value pair to log records
	AddFieldToRecord(key string, value string, tag string) FluentConfig

	// AddExternalConfig adds an @INCLUDE directive to reference an external config file
	AddExternalConfig(filePath string, position IncludePosition) FluentConfig

	// AddOutput adds an output/log destination
	AddOutput(name string, tag string, options map[string]string) FluentConfig

	// WriteFluentdConfig outputs the config in Fluentd syntax
	WriteFluentdConfig(wr io.Writer) error

	// WriteFluentBitConfig outputs the config in Fluent Bit syntax
	WriteFluentBitConfig(wr io.Writer) error
}
```

The library uses Fluent Bit terminology in the interface; `AddInput` adds a 'source' in Fluentd syntax, and the `name` argument is used as the plugin `@type`.

## License Summary

This sample code is made available under the MIT-0 license. See the LICENSE file.

#### Security disclosures

If you think you’ve found a potential security issue, please do not post it in the Issues.  Instead, please follow the instructions [here](https://aws.amazon.com/security/vulnerability-reporting/) or email AWS security directly at [aws-security@amazon.com](mailto:aws-security@amazon.com).
