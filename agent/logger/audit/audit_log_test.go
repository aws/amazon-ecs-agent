// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package audit

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_infologger "github.com/aws/amazon-ecs-agent/agent/logger/audit/mocks"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit/request"
	"github.com/golang/mock/gomock"
)

const (
	dummyContainerInstanceArn = "containerInstanceArn"
	dummyCluster              = "cluster"
	dummyEventType            = "someEvent"
	dummyRemoteAddress        = "rAddr"
	dummyUrl                  = "http://foo.com" + dummyUrlPath
	dummyUrlPath              = "/urlPath"
	dummyUserAgent            = "userAgent"
	dummyResponseCode         = 400
	taskARN                   = "task-arn-1"

	commonAuditLogEntryFieldCount = 6
	getCredentialsEntryFieldCount = 4
)

func TestWritingToAuditLog(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoLogger := mock_infologger.NewMockInfoLogger(ctrl)

	req, _ := http.NewRequest("GET", "foo", nil)
	req.RemoteAddr = dummyRemoteAddress
	parsedUrl, err := url.Parse(dummyUrl)
	if err != nil {
		t.Fatal("error parsing dummyUrl")
	}
	req.URL = parsedUrl
	req.Header.Set("User-Agent", dummyUserAgent)

	cfg := &config.Config{
		Cluster:                 dummyCluster,
		CredentialsAuditLogFile: "foo.txt",
	}

	auditLogger := NewAuditLog(dummyContainerInstanceArn, cfg, mockInfoLogger)

	if auditLogger.GetCluster() != dummyCluster {
		t.Fatal("Cluster is not initialized properly")
	}

	if auditLogger.GetContainerInstanceArn() != dummyContainerInstanceArn {
		t.Fatal("ContainerInstanceArn is not initialized properly")
	}

	mockInfoLogger.EXPECT().Info(gomock.Any()).Do(func(logLine string) {
		tokens := strings.Split(logLine, " ")
		if len(tokens) != (commonAuditLogEntryFieldCount + getCredentialsEntryFieldCount) {

		}
		verifyAuditLogEntryResult(logLine, taskARN, t)
	})

	auditLogger.Log(request.LogRequest{Request: req, ARN: taskARN}, dummyResponseCode, GetCredentialsEventType())
}

func TestWritingErrorsToAuditLog(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoLogger := mock_infologger.NewMockInfoLogger(ctrl)

	req, _ := http.NewRequest("GET", "foo", nil)
	req.RemoteAddr = dummyRemoteAddress
	parsedUrl, err := url.Parse(dummyUrl)
	if err != nil {
		t.Fatal("error parsing dummyUrl")
	}
	req.URL = parsedUrl
	req.Header.Set("User-Agent", dummyUserAgent)

	cfg := &config.Config{
		Cluster:                 dummyCluster,
		CredentialsAuditLogFile: "foo.txt",
	}

	auditLogger := NewAuditLog(dummyContainerInstanceArn, cfg, mockInfoLogger)

	if auditLogger.GetCluster() != dummyCluster {
		t.Fatal("Cluster is not initialized properly")
	}

	if auditLogger.GetContainerInstanceArn() != dummyContainerInstanceArn {
		t.Fatal("ContainerInstanceArn is not initialized properly")
	}

	mockInfoLogger.EXPECT().Info(gomock.Any()).Do(func(logLine string) {
		tokens := strings.Split(logLine, " ")
		if len(tokens) != (commonAuditLogEntryFieldCount + getCredentialsEntryFieldCount) {

		}
		verifyAuditLogEntryResult(logLine, "-", t)
	})

	auditLogger.Log(request.LogRequest{Request: req, ARN: ""}, dummyResponseCode, GetCredentialsEventType())
}

func TestWritingToAuditLogWhenDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoLogger := mock_infologger.NewMockInfoLogger(ctrl)

	req, _ := http.NewRequest("GET", "foo", nil)

	cfg := &config.Config{
		Cluster:                     dummyCluster,
		CredentialsAuditLogFile:     "foo.txt",
		CredentialsAuditLogDisabled: true,
	}

	auditLogger := NewAuditLog(dummyContainerInstanceArn, cfg, mockInfoLogger)

	if auditLogger.GetCluster() != dummyCluster {
		t.Fatal("Cluster is not initialized properly")
	}

	if auditLogger.GetContainerInstanceArn() != dummyContainerInstanceArn {
		t.Fatal("ContainerInstanceArn is not initialized properly")
	}

	mockInfoLogger.EXPECT().Info(gomock.Any()).Times(0)

	auditLogger.Log(request.LogRequest{Request: req, ARN: taskARN}, dummyResponseCode, GetCredentialsEventType())
}

func TestConstructCommonAuditLogEntryFields(t *testing.T) {
	req, _ := http.NewRequest("GET", "foo", nil)
	req.RemoteAddr = dummyRemoteAddress
	parsedUrl, err := url.Parse(dummyUrl)
	if err != nil {
		t.Fatal("error parsing dummyUrl")
	}
	req.URL = parsedUrl
	req.Header.Set("User-Agent", dummyUserAgent)

	result := constructCommonAuditLogEntryFields(request.LogRequest{Request: req, ARN: taskARN}, dummyResponseCode)

	verifyCommonAuditLogEntryFieldResult(result, taskARN, t)
}

func TestConstructAuditLogEntryByTypeGetCredentials(t *testing.T) {
	result := constructAuditLogEntryByType(GetCredentialsEventType(), dummyCluster,
		dummyContainerInstanceArn)
	verifyConstructAuditLogEntryGetCredentialsResult(result, t)
}

func verifyAuditLogEntryResult(logLine string, expectedTaskArn string, t *testing.T) {
	tokens := strings.Split(logLine, " ")
	if len(tokens) != (commonAuditLogEntryFieldCount + getCredentialsEntryFieldCount) {
		t.Fatalf("Incorrect number of tokens in audit log entry. Expected %d.",
			commonAuditLogEntryFieldCount+getCredentialsEntryFieldCount)
	}
	verifyCommonAuditLogEntryFieldResult(strings.Join(tokens[:commonAuditLogEntryFieldCount], " "), expectedTaskArn, t)
	verifyConstructAuditLogEntryGetCredentialsResult(strings.Join(tokens[commonAuditLogEntryFieldCount:], " "), t)
}

func verifyCommonAuditLogEntryFieldResult(result string, expectedTaskArn string, t *testing.T) {
	tokens := strings.Split(result, " ")

	if len(tokens) != commonAuditLogEntryFieldCount {
		t.Fatalf("Incorrect number of tokens in common audit log entry. Expected %d.",
			commonAuditLogEntryFieldCount)
	}

	respCode, _ := strconv.Atoi(tokens[1])
	if respCode != dummyResponseCode {
		t.Fatal("response code does not match")
	}

	if tokens[2] != dummyRemoteAddress {
		t.Fatal("remote address does not match")
	}

	if tokens[3] != fmt.Sprintf(`"%s"`, dummyUrlPath) {
		t.Fatalf("url path does not match %v, %v", tokens[3], fmt.Sprintf(`"%s"`, dummyUrlPath))
	}

	if tokens[4] != fmt.Sprintf(`"%s"`, dummyUserAgent) {
		t.Fatal("user agent does not match")
	}

	if tokens[5] != expectedTaskArn {
		t.Fatal("arn for credentials does not match")
	}
}

func verifyConstructAuditLogEntryGetCredentialsResult(result string, t *testing.T) {
	tokens := strings.Split(result, " ")

	if len(tokens) != getCredentialsEntryFieldCount {
		t.Fatalf("Incorrect number of tokens in getCredentials audit log entry. Expected %d.",
			getCredentialsEntryFieldCount)
	}

	if tokens[0] != GetCredentialsEventType() {
		t.Fatal("event type does not match")
	}

	auditLogVersion, _ := strconv.Atoi(tokens[1])
	if auditLogVersion != getCredentialsAuditLogVersion {
		t.Fatal("version does not match")
	}

	if tokens[2] != dummyCluster {
		t.Fatal("cluster does not match")
	}

	if tokens[3] != dummyContainerInstanceArn {
		t.Fatal("containerInstanceArn does not match")
	}
}

func TestConstructAuditLogEntryByTypeUnknownType(t *testing.T) {
	result := constructAuditLogEntryByType("unknownEvent", dummyCluster, dummyContainerInstanceArn)

	if result != "" {
		t.Fatal("unknown event type should not return an entry")
	}
}
