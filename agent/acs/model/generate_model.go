// +build generate
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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/awslabs/aws-sdk-go/internal/model/api"
	"github.com/awslabs/aws-sdk-go/internal/util"
)

//go:generate go run generate_model.go

func main() {
	os.Exit(_main())
}

var tplAPI = template.Must(template.New("api").Parse(`
{{ range $_, $s := .ShapeList }}
{{ if eq $s.Type "structure"}}{{ $s.GoCode }}{{ end }}

{{ end }}
`))

// return-based exit code so the 'defer' works
func _main() int {
	apiFiles, err := filepath.Glob("./api/*.json")
	if err != nil || len(apiFiles) != 1 {
		fmt.Println("Expected a single json file in ./api")
		return 1
	}
	file := apiFiles[0]
	api := &api.API{
		NoInflections:        true,
		NoRemoveUnusedShapes: true,
	}
	api.Attach(file)

	outFile := filepath.Join(api.PackageName(), "api.go")

	var buf bytes.Buffer
	err = tplAPI.Execute(&buf, api)
	if err != nil {
		panic(err)
	}
	code := strings.TrimSpace(buf.String())
	code = util.GoFmt(code)
	err = ioutil.WriteFile(outFile, []byte(fmt.Sprintf("package %s\n\n%s", api.PackageName(), code)), 0644)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	return 0
}
