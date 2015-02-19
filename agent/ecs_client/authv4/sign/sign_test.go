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

package sign

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/signable"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

var (
	// from Amazon authv4 test suite
	region      = "us-east-1"
	service     = "host"
	accessKey   = "AKIDEXAMPLE"
	secretKey   = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	refdate     = "Mon, 09 Sep 2011 23:36:00 GMT"
	testdate, _ = time.Parse(time.RFC1123, refdate)
)

var creds = credentials.NewCredentialProvider(accessKey, secretKey)

// reads req file and creates an http request to match
func getTestReq(filename string) (*http.Request, []string, error) {

	reqtext, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}

	reqarray := strings.Split(string(reqtext), "\n")

	// first line should be method and path
	first_line := strings.Split(reqarray[0], " ")
	method := strings.TrimSpace(first_line[0])
	path := strings.TrimSpace(first_line[1])

	// headers follow, we need to find the host to make the url
	host := ""
	for _, line := range reqarray {
		if strings.HasPrefix(line, "h") || strings.HasPrefix(line, "H") {
			parsed_line := strings.Split(line, ":")
			host = strings.TrimSpace(parsed_line[1])
			break
		}
	}
	if host == "" {
		return nil, nil, errors.New("No host header found in " + filename)
	}
	requrl := "http://" + host + path

	// now find the body
	body := ""
	for i, line := range reqarray {
		if strings.TrimSpace(line) == "" {
			for j := i + 1; j < len(reqarray); j++ {
				body += reqarray[j]
			}
			break
		}
	}

	// make an http request
	r, err := http.NewRequest(method, requrl, strings.NewReader(body))
	if err != nil {
		return r, nil, err
	}

	// add headers
	var headers []string
	for i, line := range reqarray {

		if i == 0 || len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "\n\n") {
			break
		}

		parsed_line := strings.SplitN(line, ":", 2)
		if len(parsed_line) == 2 {
			h := strings.TrimSpace(parsed_line[0])
			v := strings.TrimSpace(parsed_line[1])
			r.Header.Add(h, v)
			headers = append(headers, h)
		}
	}

	return r, headers, err
}

func getTestAuthZ(filename string) (string, error) {
	authz, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(authz), err
}

func doTest(testname string, t *testing.T) {

	r, headers, err := getTestReq(testname + ".req")
	if err != nil {
		msg := err.Error()

		// the test 'post-vanilla-query-nonunreserved' has URL that Go refuses to parse, skip
		if strings.Contains(msg, "invalid URL escape") {
			return
		} else {
			t.Error("Fail ", testname, msg)
			return
		}
	}

	signer := NewSigner(region, service, creds, headers)
	_, authz, _ := signer.SignDetails(testdate, signable.HttpRequest{r})

	// check
	reference, err := getTestAuthZ(testname + ".authz")
	if err != nil {
		t.Error("Fail ", testname, err)
		return
	}

	if authz != reference {
		t.Error("Fail ", testname, "Does not match expected authz")
		return
	}

}

func TestAWSv4Suite(t *testing.T) {

	dir := "testdata/aws4_testsuite/"

	if false {

		// cherry pick ones to debug
		doTest(dir+"get-vanilla-ut8-query", t)

	} else {

		// get the test file prefixes (read all files, strip suffix)
		testnames := make(map[string]bool, 1)
		fileinfos, err := ioutil.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, fileinfo := range fileinfos {
			testnames[strings.Split(fileinfo.Name(), ".")[0]] = true
		}

		// for each test, run
		for testname, _ := range testnames {
			if testname == "" {
				continue
			}
			t.Log(testname)
			doTest(dir+testname, t)
		}
	}
}
