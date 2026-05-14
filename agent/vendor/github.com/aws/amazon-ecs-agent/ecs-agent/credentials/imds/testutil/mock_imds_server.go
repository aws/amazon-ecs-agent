//go:build integration
// +build integration

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

// Package testutil provides a mock IMDS HTTP server for integration testing
// the IMDS credential scanner with agent's refresher.
package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockIMDSServer is an HTTP server that mimics the EC2 IMDS
// endpoints.
type MockIMDSServer struct {
	mu         sync.Mutex
	namespaces map[string]*mockNamespace
	server     *httptest.Server
}

type mockNamespace struct {
	lastUpdated time.Time
	credentials map[string]*mockCredential
}

type mockCredential struct {
	RoleArn         string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      string
}

// NewMockIMDSServer creates and starts a mock IMDS HTTP server.
func NewMockIMDSServer() *MockIMDSServer {
	s := &MockIMDSServer{
		namespaces: make(map[string]*mockNamespace),
	}
	s.server = httptest.NewServer(s)
	return s
}

// URL returns the base URL of the mock server.
func (s *MockIMDSServer) URL() string {
	return s.server.URL
}

// Close shuts down the mock server.
func (s *MockIMDSServer) Close() {
	s.server.Close()
}

// AddCredential registers a credential in the mock IMDS server.
// The namespace is auto-created if it doesn't exist.
func (s *MockIMDSServer) AddCredential(
	namespace, taskID, roleType, roleArn, accessKeyID string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.getOrCreateNamespace(namespace)
	key := taskID + "-" + roleType
	ns.credentials[key] = &mockCredential{
		RoleArn:         roleArn,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: "secret-" + accessKeyID,
		SessionToken:    "token-" + accessKeyID,
		Expiration:      time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
	}
	ns.lastUpdated = time.Now().UTC()
}

// RotateCredential updates a credential and bumps the namespace's
// LastUpdated timestamp. Fails the test if the namespace or credential
// key is not found.
func (s *MockIMDSServer) RotateCredential(
	t *testing.T,
	namespace, taskID, roleType, newAccessKeyID string,
) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()

	ns, ok := s.namespaces[namespace]
	if !ok {
		t.Fatalf("mock IMDS: namespace %q not registered", namespace)
	}
	key := taskID + "-" + roleType
	cred, ok := ns.credentials[key]
	if !ok {
		t.Fatalf("mock IMDS: credential %q not found in namespace %q",
			key, namespace)
	}
	cred.AccessKeyID = newAccessKeyID
	cred.SecretAccessKey = "secret-" + newAccessKeyID
	cred.SessionToken = "token-" + newAccessKeyID
	ns.lastUpdated = time.Now().UTC()
}

// getOrCreateNamespace returns a existing namespace or creates a new
// one if it doesn't exist yet.
func (s *MockIMDSServer) getOrCreateNamespace(name string) *mockNamespace {
	ns, ok := s.namespaces[name]
	if !ok {
		ns = &mockNamespace{
			lastUpdated: time.Now().UTC(),
			credentials: make(map[string]*mockCredential),
		}
		s.namespaces[name] = ns
	}
	return ns
}

// ServeHTTP handles IMDS requests.
// Expected paths:
//   - /latest/meta-data/ → list of entries including iam-ecs-* namespaces
//   - /latest/meta-data/<ns>/info → NamespaceInfo JSON
//   - /latest/meta-data/<ns>/security-credentials/<key> → credential JSON
func (s *MockIMDSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle IMDSv2 token request.
	if r.Method == http.MethodPut && r.URL.Path == "/latest/api/token" {
		w.Header().Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
		fmt.Fprint(w, "mock-token")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := strings.TrimPrefix(r.URL.Path, "/latest/meta-data")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	// Root listing: return namespace names.
	if path == "" {
		var lines []string
		for name := range s.namespaces {
			lines = append(lines, name+"/")
		}
		fmt.Fprint(w, strings.Join(lines, "\n"))
		return
	}

	// Instance identity metadata used by IsIMDSAvailable.
	if path == "instance-id" {
		fmt.Fprint(w, "i-mock12345")
		return
	}

	// Check if path matches <namespace>/info
	if ns, suffix, ok := strings.Cut(path, "/"); ok {
		nsData, exists := s.namespaces[ns]
		if !exists {
			http.NotFound(w, r)
			return
		}

		if suffix == "info" {
			s.serveInfo(w, nsData)
			return
		}

		// Check if path matches <namespace>/security-credentials/<key>
		if credKey, ok := strings.CutPrefix(suffix, "security-credentials/"); ok {
			s.serveCredential(w, r, nsData, credKey)
			return
		}
	}

	http.NotFound(w, r)
}

// serveInfo writes the namespace info JSON response containing
// LastUpdated and the TaskCredentials map.
func (s *MockIMDSServer) serveInfo(w http.ResponseWriter, ns *mockNamespace) {
	type taskCredInfo struct {
		RoleARN string `json:"RoleARN"`
	}
	info := struct {
		LastUpdated     string                  `json:"LastUpdated"`
		TaskCredentials map[string]taskCredInfo `json:"TaskCredentials"`
	}{
		LastUpdated:     ns.lastUpdated.Format(time.RFC3339Nano),
		TaskCredentials: make(map[string]taskCredInfo),
	}
	for key, cred := range ns.credentials {
		info.TaskCredentials[key] = taskCredInfo{RoleARN: cred.RoleArn}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// serveCredential writes the credential JSON response for a single
// credential key within a namespace.
func (s *MockIMDSServer) serveCredential(
	w http.ResponseWriter, r *http.Request,
	ns *mockNamespace, key string,
) {
	cred, ok := ns.credentials[key]
	if !ok {
		http.NotFound(w, r)
		return
	}
	resp := struct {
		AccessKeyId     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
		Expiration      string `json:"Expiration"`
	}{
		AccessKeyId:     cred.AccessKeyID,
		SecretAccessKey: cred.SecretAccessKey,
		Token:           cred.SessionToken,
		Expiration:      cred.Expiration,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
