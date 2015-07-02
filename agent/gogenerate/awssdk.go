// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/aws/aws-sdk-go/internal/model/api"
	"github.com/aws/aws-sdk-go/internal/util"
	"golang.org/x/tools/imports"
)

func main() {
	os.Exit(_main())
}

var typesOnlyTplAPI = template.Must(template.New("api").Parse(`
{{ range $_, $s := .ShapeList }}
{{ if eq $s.Type "structure"}}{{ $s.GoCode }}{{ end }}

{{ end }}
`))

const copyrightHeader = `// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
`

// return-based exit code so the 'defer' works
func _main() int {

	var typesOnly bool
	flag.BoolVar(&typesOnly, "typesOnly", false, "only generate types")
	flag.Parse()

	apiFiles, err := filepath.Glob("./api/api-2.json")
	if err != nil {
		fmt.Println("Error listing api json files: ", err)
		return 1
	}
	for _, file := range apiFiles {
		var err error
		if typesOnly {
			err = genTypesOnlyAPI(file)
		} else {
			err = genFull(file)
		}
		if err != nil {
			fmt.Println(err)
			return 1
		}
	}
	return 0
}

func genTypesOnlyAPI(file string) error {
	apiGen := &api.API{
		NoInflections:        true,
		NoRemoveUnusedShapes: true,
	}
	apiGen.Attach(file)
	apiGen.Setup()
	// to reset imports so that timestamp has an entry in the map.
	apiGen.APIGoCode()

	var buf bytes.Buffer
	err := typesOnlyTplAPI.Execute(&buf, apiGen)
	if err != nil {
		panic(err)
	}
	code := strings.TrimSpace(buf.String())
	code = util.GoFmt(code)

	// Ignore dir error, filepath will catch it for an invalid path.
	os.Mkdir(apiGen.PackageName(), 0755)
	// Fix imports.
	codeWithImports, err := imports.Process("", []byte(fmt.Sprintf("package %s\n\n%s", apiGen.PackageName(), code)), nil)
	if err != nil {
		fmt.Println(err)
		return err
	}
	outFile := filepath.Join(apiGen.PackageName(), "api.go")
	err = ioutil.WriteFile(outFile, []byte(fmt.Sprintf("%s\n%s", copyrightHeader, codeWithImports)), 0644)
	if err != nil {
		return err
	}
	return nil
}

func genFull(file string) error {
	// Heavily inspired by https://github.com/aws/aws-sdk-go/blob/45ba2f817fbf625fe48cf9b51fae13e5dfba6171/internal/model/cli/gen-api/main.go#L36
	// That code is copyright Amazon
	api := &api.API{}
	api.Attach(file)
	paginatorsFile := strings.Replace(file, "api-2.json", "paginators-1.json", -1)
	if _, err := os.Stat(paginatorsFile); err == nil {
		api.AttachPaginators(paginatorsFile)
	}

	docsFile := strings.Replace(file, "api-2.json", "docs-2.json", -1)
	if _, err := os.Stat(docsFile); err == nil {
		api.AttachDocs(docsFile)
	}
	api.Setup()

	// Ignore dir error, filepath will catch it for an invalid path.
	os.Mkdir(api.PackageName(), 0755)
	outFile := filepath.Join(api.PackageName(), "api.go")
	err := ioutil.WriteFile(outFile, []byte(fmt.Sprintf("%s\npackage %s\n\n%s", copyrightHeader, api.PackageName(), api.APIGoCode())), 0644)
	if err != nil {
		return err
	}

	outFile = filepath.Join(api.PackageName(), "service.go")
	err = ioutil.WriteFile(outFile, []byte(fmt.Sprintf("%s\npackage %s\n\n%s", copyrightHeader, api.PackageName(), api.ServiceGoCode())), 0644)
	if err != nil {
		return err
	}
	return nil
}
