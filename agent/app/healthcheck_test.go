//go:build unit
// +build unit

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

package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHealthcheck_Sunny(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	rc := runHealthcheck(ts.URL, time.Second*2)
	require.Equal(t, 0, rc)
}

func TestHealthcheck_InvalidURL2(t *testing.T) {
	// leading space in url is invalid
	rc := runHealthcheck("  http://foobar", time.Second*2)
	require.Equal(t, 1, rc)
}

func TestHealthcheck_EnvvarConfig(t *testing.T) {
	testEnvVar := "TEST_AGENT_HEALTHCHECK_ENV_VAR"
	defer os.Unsetenv(testEnvVar)
	os.Setenv(testEnvVar, "127.0.0.1")

	// testing healthcheck url setup from app/run.go
	localhost := "localhost"
	if localhostOverride := os.Getenv(testEnvVar); localhostOverride != "" {
		localhost = localhostOverride
	}
	healthcheckUrl := fmt.Sprintf("http://%s:51678/v1/metadata", localhost)
	expectedUrl := "http://127.0.0.1:51678/v1/metadata"

	require.Equal(t, healthcheckUrl, expectedUrl)
}

func TestHealthcheck_Timeout(t *testing.T) {
	sema := make(chan int)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-sema
	}))
	defer ts.Close()

	rc := runHealthcheck(ts.URL, time.Second*2)
	require.Equal(t, 1, rc)
	close(sema)
}

// we actually pass the healthcheck in the event of a non-200 http status code.
func TestHealthcheck_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("https://http.cat/404"))
	}))
	defer ts.Close()

	rc := runHealthcheck(ts.URL+"/foobar", time.Second*2)
	require.Equal(t, 0, rc)
}

var brc int

func BenchmarkHealthcheck(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello")
	}))
	defer ts.Close()

	for n := 0; n < b.N; n++ {
		brc = runHealthcheck(ts.URL, time.Second*1)
	}
}
