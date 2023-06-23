package dotenv

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var noopPresets = make(map[string]string)

func parseAndCompare(t *testing.T, rawEnvLine string, expectedKey string, expectedValue string) {
	t.Helper()
	env, err := Parse(strings.NewReader(rawEnvLine))
	if !assert.NoError(t, err) {
		return
	}
	actualValue, ok := env[expectedKey]
	if !ok {
		t.Errorf("Key %q was not found in env: %v", expectedKey, env)
	} else if actualValue != expectedValue {
		t.Errorf("Expected '%v' to parse as '%v' => '%v', got '%v' => '%v' instead", rawEnvLine, expectedKey, expectedValue, expectedKey, actualValue)
	}
}

func loadEnvAndCompareValues(t *testing.T, loader func(files ...string) error, envFileName string, expectedValues map[string]string, presets map[string]string) {
	// first up, clear the env
	os.Clearenv()

	for k, v := range presets {
		os.Setenv(k, v)
	}

	err := loader(envFileName)
	if err != nil {
		t.Fatalf("Error loading %v", envFileName)
	}

	for k, expected := range expectedValues {
		actual := strings.ReplaceAll(os.Getenv(k), "\r\n", "\n")
		if actual != expected {
			t.Errorf("Mismatch for key '%v': expected '%v' got '%v'", k, expected, actual)
		}
	}
}

func TestLoadWithNoArgsLoadsDotEnv(t *testing.T) {
	err := Load()
	pathError := err.(*os.PathError)
	if pathError == nil || pathError.Op != "open" || pathError.Path != ".env" {
		t.Errorf("Didn't try and open .env by default")
	}
}

func TestLoadFileNotFound(t *testing.T) {
	err := Load("somefilethatwillneverexistever.env")
	if err == nil {
		t.Error("File wasn't found but Load didn't return an error")
	}
}

func TestReadPlainEnv(t *testing.T) {
	envFileName := "fixtures/plain.env"
	expectedValues := map[string]string{
		"OPTION_A": "1",
		"OPTION_B": "2",
		"OPTION_C": "3",
		"OPTION_D": "4",
		"OPTION_E": "5",
		"OPTION_F": "",
		"OPTION_G": "",
		"OPTION_H": "my string",
		"OPTION_I": "data\"kill",
	}

	envMap, err := Read(envFileName)
	if err != nil {
		t.Error("Error reading file")
	}

	if len(envMap) != len(expectedValues) {
		t.Error("Didn't get the right size map back")
	}

	for key, value := range expectedValues {
		if envMap[key] != value {
			t.Errorf("Read got one of the keys wrong. Expected: %q got %q", value, envMap[key])
		}
	}
}

func TestParse(t *testing.T) {
	envMap, err := Parse(bytes.NewReader([]byte("ONE=1\nTWO='2'\nTHREE = \"3\"")))
	expectedValues := map[string]string{
		"ONE":   "1",
		"TWO":   "2",
		"THREE": "3",
	}
	if err != nil {
		t.Fatalf("error parsing env: %v", err)
	}
	for key, value := range expectedValues {
		if envMap[key] != value {
			t.Errorf("expected %s to be %s, got %s", key, value, envMap[key])
		}
	}
}

func TestLoadDoesNotOverride(t *testing.T) {
	envFileName := "fixtures/plain.env"

	// ensure NO overload
	presets := map[string]string{
		"OPTION_A": "do_not_override",
		"OPTION_B": "",
	}

	expectedValues := map[string]string{
		"OPTION_A": "do_not_override",
		"OPTION_B": "",
	}
	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, presets)
}

func TestLoadPlainEnv(t *testing.T) {
	envFileName := "fixtures/plain.env"
	expectedValues := map[string]string{
		"OPTION_A": "1",
		"OPTION_B": "2",
		"OPTION_C": "3",
		"OPTION_D": "4",
		"OPTION_E": "5",
	}

	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, noopPresets)
}

func TestLoadExportedEnv(t *testing.T) {
	envFileName := "fixtures/exported.env"
	expectedValues := map[string]string{
		"OPTION_A": "2",
		"OPTION_B": "\\n",
	}

	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, noopPresets)
}

func TestLoadEqualsEnv(t *testing.T) {
	envFileName := "fixtures/equals.env"
	expectedValues := map[string]string{
		"OPTION_A": "postgres://localhost:5432/database?sslmode=disable",
	}

	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, noopPresets)
}

func TestLoadQuotedEnv(t *testing.T) {
	envFileName := "fixtures/quoted.env"
	expectedValues := map[string]string{
		"OPTION_A": "1",
		"OPTION_B": "2",
		"OPTION_C": "",
		"OPTION_D": "\\n",
		"OPTION_E": "1",
		"OPTION_F": "2",
		"OPTION_G": "",
		"OPTION_H": "\n",
		"OPTION_I": "echo 'asd'",
		"OPTION_J": `first line
second line
third line
and so on`,
		"OPTION_Z": "last value",
	}

	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, noopPresets)
}

func TestLoadUnquotedEnv(t *testing.T) {
	envFileName := "fixtures/unquoted.env"
	expectedValues := map[string]string{
		"OPTION_A": "some quoted phrase",
		"OPTION_B": "first one with an unquoted phrase",
		"OPTION_C": "then another one with an unquoted phrase",
		"OPTION_D": "then another one with an unquoted phrase special è char",
		"OPTION_E": "then another one quoted phrase",
	}

	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, noopPresets)
}

func TestVariableStringValueSeparator(t *testing.T) {
	input := "TEST_URLS=\"stratum+tcp://stratum.antpool.com:3333\nstratum+tcp://stratum.antpool.com:443\""
	want := map[string]string{
		"TEST_URLS": "stratum+tcp://stratum.antpool.com:3333\nstratum+tcp://stratum.antpool.com:443",
	}
	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Error(err)
	}

	if len(got) != len(want) {
		t.Fatalf(
			"unexpected value:\nwant:\n\t%#v\n\ngot:\n\t%#v", want, got)
	}

	for k, wantVal := range want {
		gotVal, ok := got[k]
		if !ok {
			t.Fatalf("key %q doesn't present in result", k)
		}
		if wantVal != gotVal {
			t.Fatalf(
				"mismatch in %q value:\nwant:\n\t%s\n\ngot:\n\t%s", k,
				wantVal, gotVal)
		}
	}
}

func TestActualEnvVarsAreLeftAlone(t *testing.T) {
	os.Clearenv()
	os.Setenv("OPTION_A", "actualenv")
	_ = Load("fixtures/plain.env")

	if os.Getenv("OPTION_A") != "actualenv" {
		t.Error("An ENV var set earlier was overwritten")
	}
}

func TestParsing(t *testing.T) {
	// unquoted values
	parseAndCompare(t, "FOO=bar", "FOO", "bar")

	// parses values with spaces around equal sign
	parseAndCompare(t, "FOO =bar", "FOO", "bar")
	parseAndCompare(t, "FOO= bar", "FOO", "bar")

	// parses double quoted values
	parseAndCompare(t, `FOO="bar"`, "FOO", "bar")

	// parses single quoted values
	parseAndCompare(t, "FOO='bar'", "FOO", "bar")

	// parses escaped double quotes
	parseAndCompare(t, `FOO="escaped\"bar"`, "FOO", `escaped"bar`)
	parseAndCompare(t, `FOO="\"quoted\""`, "FOO", `"quoted"`)

	// parses single quotes inside double quotes
	parseAndCompare(t, `FOO="'d'"`, "FOO", `'d'`)

	// parses yaml style options
	parseAndCompare(t, "OPTION_A: 1", "OPTION_A", "1")

	// parses yaml values with equal signs
	parseAndCompare(t, "OPTION_A: Foo=bar", "OPTION_A", "Foo=bar")

	// parses non-yaml options with colons
	parseAndCompare(t, "OPTION_A=1:B", "OPTION_A", "1:B")

	// parses export keyword
	parseAndCompare(t, "export OPTION_A=2", "OPTION_A", "2")
	parseAndCompare(t, `export OPTION_B='\n'`, "OPTION_B", "\\n")
	parseAndCompare(t, "export exportFoo=2", "exportFoo", "2")
	parseAndCompare(t, "exportFOO=2", "exportFOO", "2")
	parseAndCompare(t, "export_FOO =2", "export_FOO", "2")
	parseAndCompare(t, "export.FOO= 2", "export.FOO", "2")
	parseAndCompare(t, "export\tOPTION_A=2", "OPTION_A", "2")
	parseAndCompare(t, "  export OPTION_A=2", "OPTION_A", "2")
	parseAndCompare(t, "\texport OPTION_A=2", "OPTION_A", "2")
	parseAndCompare(t, `export OPTION_A="export A"`, "OPTION_A", "export A")

	// it 'expands newlines in quoted strings' do
	// expect(env('FOO="bar\nbaz"')).to eql('FOO' => "bar\nbaz")
	parseAndCompare(t, `FOO="bar\nbaz"`, "FOO", "bar\nbaz")
	parseAndCompare(t, `FOO=a\tb`, "FOO", `a\tb`)
	parseAndCompare(t, `FOO="a\tb"`, "FOO", "a\tb")

	// various shell escape sequences
	// see https://pubs.opengroup.org/onlinepubs/9699919799/utilities/echo.html
	parseAndCompare(t, `KEY="Z\aZ\bZ\fZ\nZ\rZ\tZ\vZ\\Z\0123Z"`, "KEY", "Z\aZ\bZ\fZ\nZ\rZ\tZ\vZ\\ZSZ")

	// it 'parses variables with "." in the name' do
	// expect(env('FOO.BAR=foobar')).to eql('FOO.BAR' => 'foobar')
	parseAndCompare(t, "FOO.BAR=foobar", "FOO.BAR", "foobar")

	// it 'parses variables with several "=" in the value' do
	// expect(env('FOO=foobar=')).to eql('FOO' => 'foobar=')
	parseAndCompare(t, "FOO=foobar=", "FOO", "foobar=")

	// it 'strips unquoted values' do
	// expect(env('foo=bar ')).to eql('foo' => 'bar') # not 'bar '
	parseAndCompare(t, "FOO=bar ", "FOO", "bar")

	// it 'ignores inline comments' do
	// expect(env("foo=bar # this is foo")).to eql('foo' => 'bar')
	parseAndCompare(t, "FOO=bar # this is foo", "FOO", "bar")
	parseAndCompare(t, "FOO=bar #this is foo", "FOO", "bar")
	parseAndCompare(t, "FOO=bar #", "FOO", "bar")
	parseAndCompare(t, "FOO=123#not-an-inline-comment", "FOO", "123#not-an-inline-comment")

	// it 'allows # in quoted value' do
	// expect(env('foo="bar#baz" # comment')).to eql('foo' => 'bar#baz')
	parseAndCompare(t, `FOO="bar#baz"`, "FOO", "bar#baz")
	parseAndCompare(t, `FOO="bar#baz"#`, "FOO", "bar#baz")
	parseAndCompare(t, `FOO="bar#baz" # comment`, "FOO", "bar#baz")
	parseAndCompare(t, "FOO='bar#baz' # comment", "FOO", "bar#baz")
	parseAndCompare(t, `FOO="bar#baz#bang" # comment`, "FOO", "bar#baz#bang")

	// it 'parses # in quoted values' do
	// expect(env('foo="ba#r"')).to eql('foo' => 'ba#r')
	// expect(env("foo='ba#r'")).to eql('foo' => 'ba#r')
	parseAndCompare(t, `FOO="ba#r"`, "FOO", "ba#r")
	parseAndCompare(t, "FOO='ba#r'", "FOO", "ba#r")

	// newlines and backslashes should be escaped
	parseAndCompare(t, `FOO="bar\n\ b\az"`, "FOO", "bar\n\\ b\az")
	parseAndCompare(t, `FOO="bar\\\n\ b\az"`, "FOO", "bar\\\n\\ b\az")
	parseAndCompare(t, `FOO="bar\\r\ b\az"`, "FOO", "bar\\r\\ b\az")

	parseAndCompare(t, `="value"`, "", "value")

	// leading whitespace should be ignored
	parseAndCompare(t, " KEY =value", "KEY", "value")
	parseAndCompare(t, "   KEY=value", "KEY", "value")
	parseAndCompare(t, "\tKEY=value", "KEY", "value")

	// XSI-echo style octal escapes are expanded
	parseAndCompare(t, `KEY="\0123"`, "KEY", "S")

	// non-XSI/POSIX escapes are ignored
	parseAndCompare(t, `KEY="\x07"`, "KEY", `\x07`)
	parseAndCompare(t, `KEY="\u12e4"`, "KEY", `\u12e4`)
	parseAndCompare(t, `KEY="\U00101234"`, "KEY", `\U00101234`)

	// it 'throws an error if line format is incorrect' do
	// expect{env('lol$wut')}.to raise_error(Dotenv::FormatError)
	badlyFormattedLine := "lol$wut"
	_, err := Parse(strings.NewReader(badlyFormattedLine))
	if err == nil {
		t.Errorf("Expected \"%v\" to return error, but it didn't", badlyFormattedLine)
	}
}

func TestUnterminatedQuotes(t *testing.T) {
	cases := []string{
		`KEY="`,
		`KEY="value`,
		`KEY="value\"`,
		`KEY="value'`,
		`KEY='`,
		`KEY='value`,
		`KEY='value\'`,
		`KEY='value"`,
	}
	for _, tc := range cases {
		_, err := Parse(strings.NewReader(tc))
		assert.Contains(t, err.Error(), "unterminated quoted value")
		//assert.EqualErrorf(t, err, "unterminated quoted value", "Env data: %v", tc)
	}
}

func TestLinesToIgnore(t *testing.T) {
	cases := map[string]struct {
		input string
		want  string
	}{
		"Line with nothing but line break": {
			input: "\n",
		},
		"Line with nothing but windows-style line break": {
			input: "\r\n",
		},
		"Line full of whitespace": {
			input: "\t\t ",
		},
		"Comment": {
			input: "# Comment",
		},
		"Indented comment": {
			input: "\t # comment",
		},
		"non-ignored value": {
			input: `export OPTION_B='\n'`,
			want:  `export OPTION_B='\n'`,
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			got := string(newParser().getStatementStart(c.input))
			if got != c.want {
				t.Errorf("Expected:\t %q\nGot:\t %q", c.want, got)
			}
		})
	}
}

func TestErrorReadDirectory(t *testing.T) {
	envFileName := "fixtures/"
	envMap, err := Read(envFileName)

	if err == nil {
		t.Errorf("Expected error, got %v", envMap)
	}
}

func TestErrorParsing(t *testing.T) {
	envFileName := "fixtures/invalid1.env"
	_, err := Read(envFileName)
	assert.EqualError(t, err, "line 7: key cannot contain a space")
}

func TestInheritedEnvVariableSameSize(t *testing.T) {
	const envKey = "VAR_TO_BE_LOADED_FROM_OS_ENV"
	const envVal = "SOME_RANDOM_VALUE"
	os.Setenv(envKey, envVal)

	envFileName := "fixtures/inherited-multi-var.env"
	expectedValues := map[string]string{
		envKey: envVal,
		"foo":  "bar",
		"bar":  "baz",
	}

	envMap, err := ReadWithLookup(os.LookupEnv, envFileName)
	if err != nil {
		t.Error("Error reading file")
	}
	if len(envMap) != len(expectedValues) {
		t.Error("Didn't get the right size map back")
	}
	for key, value := range expectedValues {
		if envMap[key] != value {
			t.Errorf("Read got one of the keys wrong, [%q]->%q", key, envMap[key])
		}
	}
}

func TestInheritedEnvVariableSingleVar(t *testing.T) {
	const envKey = "VAR_TO_BE_LOADED_FROM_OS_ENV"
	const envVal = "SOME_RANDOM_VALUE"
	os.Setenv(envKey, envVal)

	envFileName := "fixtures/inherited-single-var.env"
	expectedValues := map[string]string{
		envKey: envVal,
	}

	envMap, err := ReadWithLookup(os.LookupEnv, envFileName)
	if err != nil {
		t.Error("Error reading file")
	}
	if len(envMap) != len(expectedValues) {
		t.Error("Didn't get the right size map back")
	}
	for key, value := range expectedValues {
		if envMap[key] != value {
			t.Errorf("Read got one of the keys wrong, [%q]->%q", key, envMap[key])
		}
	}
}

func TestInheritedEnvVariableNotFound(t *testing.T) {
	envMap, err := Read("fixtures/inherited-not-found.env")
	if _, ok := envMap["VARIABLE_NOT_FOUND"]; ok || err != nil {
		t.Errorf("Expected 'VARIABLE_NOT_FOUND' to be undefined with no errors")
	}
}

func TestInheritedEnvVariableNotFoundWithLookup(t *testing.T) {
	notFoundMap := make(map[string]interface{})
	envMap, err := ReadWithLookup(func(v string) (string, bool) {
		envVar, ok := os.LookupEnv(v)
		if !ok {
			notFoundMap[v] = nil
		}
		return envVar, ok
	}, "fixtures/inherited-not-found.env")
	if _, ok := envMap["VARIABLE_NOT_FOUND"]; ok || err != nil {
		t.Errorf("Expected 'VARIABLE_NOT_FOUND' to be undefined with no errors")
	}
	_, ok := notFoundMap["VARIABLE_NOT_FOUND"]
	if !ok {
		t.Errorf("Expected 'VARIABLE_NOT_FOUND' to be in the set of not found variables")
	}
}

func TestUTF8BOM(t *testing.T) {
	envFileName := "fixtures/utf8-bom.env"

	// sanity check the fixture, since the UTF-8 BOM is invisible, it'd be
	// easy for it to get removed by accident, which would invalidate this
	// test
	envFileData, err := os.ReadFile(envFileName)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(envFileData, []byte("\uFEFF")),
		"Test fixture file is missing UTF-8 BOM")

	expectedValues := map[string]string{
		"OPTION_A": "1",
		"OPTION_B": "2",
		"OPTION_C": "3",
		"OPTION_D": "4",
		"OPTION_E": "5",
	}

	loadEnvAndCompareValues(t, Load, envFileName, expectedValues, noopPresets)
}

func TestDash(t *testing.T) {
	loadEnvAndCompareValues(t, Load, "fixtures/special.env", map[string]string{
		"VAR-WITH-DASHES":      "dashes",
		"VAR.WITH.DOTS":        "dots",
		"VAR_WITH_UNDERSCORES": "underscores",
	}, noopPresets)
}
