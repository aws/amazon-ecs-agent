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

// Package imds provides an IMDS credentials scanner that retrieves task
// credentials from IMDS.
package imds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"

	"golang.org/x/time/rate"
)

const (
	// NamespacePrefix is the IMDS path prefix for ECS IAM namespaces.
	NamespacePrefix = "iam-ecs-"

	// taskIDDelimiter separates the task ID from the rest of the IMDS key.
	taskIDDelimiter = "-"

	// infoPathFormat is the path to the info file within a namespace.
	infoPathFormat = "%s/info"

	// credentialPathFormat is the path to a credential file within a namespace.
	credentialPathFormat = "%s/security-credentials/%s"

	// imdsQueriesPerSec is the rate limit for IMDS requests.
	//
	// IMDS shares a 1024 packets-per-second (PPS) limit with other
	// link local services (Route 53 DNS, NTP).
	// Ref: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html
	// The rate limiter keeps credentials scanning within ~10% of the total PPS budget
	// to leave headroom for other link-local requests on the instance.
	//
	// TODO: this value will be finalized based on load testing.
	imdsQueriesPerSec = 10

	// imdsQueryBurstSize is the token bucket size for the rate limiter.
	imdsQueryBurstSize = 1

	// Field names attached to scanner failure metrics.
	//
	// metricFieldNamespace identifies the iam-ecs-* namespace this metric entry
	// pertains to.
	metricFieldNamespace = "Namespace"
	// metricFieldTaskID identifies the task whose credential entry caused this
	// metric emission.
	metricFieldTaskID = "TaskID"
	// metricFieldRoleType identifies the role type (e.g., TaskApplication,
	// TaskExecution) of the credential that failed.
	metricFieldRoleType = "RoleType"
)

// Scanner fetches task credentials from IMDS iam-ecs-* namespaces.
type Scanner interface {
	// Scan discovers all ECS IAM namespaces, reads their info files, and
	// fetches credentials from namespaces that have changed since the last
	// scan.
	Scan(ctx context.Context) (ScanResult, error)
}

// scanner implements the Scanner interface.
type scanner struct {
	ec2MetadataClient ec2.EC2MetadataClient
	// rateLimiter controls the rate of IMDS requests.
	rateLimiter *rate.Limiter
	// lastUpdated tracks the LastUpdated timestamp from each namespace's info file.
	lastUpdated map[string]time.Time
	// metricsFactory emits metrics for scan operations.
	metricsFactory metrics.EntryFactory
}

// NewScanner creates a new IMDS credentials scanner.
func NewScanner(ec2MetadataClient ec2.EC2MetadataClient,
	metricsFactory metrics.EntryFactory) Scanner {
	return &scanner{
		ec2MetadataClient: ec2MetadataClient,
		rateLimiter:       rate.NewLimiter(rate.Limit(imdsQueriesPerSec), imdsQueryBurstSize),
		lastUpdated:       make(map[string]time.Time),
		metricsFactory:    metricsFactory,
	}
}

// Scan discovers all ECS IAM namespaces, reads their info files, and
// fetches credentials from namespaces that have changed since the last scan.
func (s *scanner) Scan(ctx context.Context) (ScanResult, error) {
	namespaces, err := s.discoverNamespaces(ctx)
	if err != nil {
		return ScanResult{}, fmt.Errorf("imds scan: discover namespaces: %w", err)
	}

	// No namespaces is expected when IMDS does not have ECS task credentials yet.
	if len(namespaces) == 0 {
		logger.Debug("IMDS credentials scan: no iam-ecs namespace found")
		return ScanResult{}, nil
	}

	var result ScanResult
	var scanErrors []error
	for _, ns := range namespaces {
		nsResult, err := s.scanNamespace(ctx, ns)
		if err != nil {
			logger.Error("IMDS credentials scan: failed to scan namespace", logger.Fields{
				"namespace": ns,
				field.Error: err,
			})
			scanErrors = append(scanErrors, err)
			// Scanning for a namespace failed; try scanning remaining
			// namespaces before returning.
			continue
		}
		result.Credentials = append(result.Credentials, nsResult.Credentials...)
		result.AssumeRoleUnauthorizedAccessRoles = append(result.AssumeRoleUnauthorizedAccessRoles, nsResult.AssumeRoleUnauthorizedAccessRoles...)
	}

	// Surface an error only when neither credentials nor unauthorized roles were
	// read, so the caller doesn't mistake a total failure for "no credentials yet".
	if len(result.Credentials) == 0 && len(result.AssumeRoleUnauthorizedAccessRoles) == 0 && len(scanErrors) > 0 {
		return ScanResult{}, fmt.Errorf("imds scan: all %d namespace(s) failed: %w",
			len(scanErrors), errors.Join(scanErrors...))
	}

	return result, nil
}

// discoverNamespaces lists the IMDS metadata root and returns all
// iam-ecs-* namespace names.
func (s *scanner) discoverNamespaces(ctx context.Context) ([]string, error) {
	// An empty path queries the IMDS metadata root (/meta-data/),
	// which returns a list of all available metadata categories.
	resp, err := s.getMetadata(ctx, "")
	if err != nil {
		return nil, err
	}

	var namespaces []string
	// IMDS returns a newline-separated list of entries.
	for _, line := range strings.Split(resp, "\n") {
		// Directory entries may have a trailing slash (e.g. "iam/").
		entry := strings.TrimSuffix(line, "/")
		if strings.HasPrefix(entry, NamespacePrefix) {
			namespaces = append(namespaces, entry)
		}
	}

	return namespaces, nil
}

// scanNamespace reads the info file for a namespace and fetches credentials for each entry.
// It returns a list of successfully fetched credentials and a list of IAM roles for which
// the "Code" in the info file was AssumeRoleUnauthorizedAccess.
func (s *scanner) scanNamespace(ctx context.Context, namespace string) (ScanResult, error) {
	infoPath := fmt.Sprintf(infoPathFormat, namespace)
	infoResp, err := s.getMetadata(ctx, infoPath)
	if err != nil {
		s.metricsFactory.New(metrics.IMDSCredentialsScannerNamespaceInfoFailureMetricName).
			WithFields(map[string]any{
				metricFieldNamespace: namespace,
			}).Done(err)
		return ScanResult{}, fmt.Errorf("fetch info for %s: %w", namespace, err)
	}

	var info NamespaceInfo
	if err := json.Unmarshal([]byte(infoResp), &info); err != nil {
		s.metricsFactory.New(metrics.IMDSCredentialsScannerNamespaceInfoFailureMetricName).
			WithFields(map[string]any{
				metricFieldNamespace: namespace,
			}).Done(err)
		return ScanResult{}, fmt.Errorf("parse info for %s: %w", namespace, err)
	}

	// Skip credential fetches if the namespace hasn't been updated since the last scan.
	lastUpdated, err := time.Parse(time.RFC3339, info.LastUpdated)
	if err != nil {
		s.metricsFactory.New(metrics.IMDSCredentialsScannerNamespaceInfoFailureMetricName).
			WithFields(map[string]any{
				metricFieldNamespace: namespace,
			}).Done(err)
		return ScanResult{}, fmt.Errorf("parse LastUpdated for %s: %w", namespace, err)
	}
	if cached, ok := s.lastUpdated[namespace]; ok && lastUpdated.Equal(cached) {
		logger.Debug("IMDS credentials scan: skipping namespace with unchanged LastUpdated", logger.Fields{
			"namespace": namespace,
		})
		return ScanResult{}, nil
	}

	var result ScanResult
	var hasErrors bool
	for key, entry := range info.TaskCredentials {
		taskID, roleType, err := parseCredentialKey(key)
		if err != nil {
			logger.Error("IMDS credentials scan: failed to parse credential key", logger.Fields{
				"namespace": namespace,
				field.Error: err,
			})
			s.metricsFactory.New(metrics.IMDSCredentialsScannerCredentialFailureMetricName).
				WithFields(map[string]any{
					metricFieldNamespace: namespace,
				}).Done(err)
			// Cannot determine task ID and role type; attempt the next credential.
			hasErrors = true
			continue
		}

		// A Code of AssumeRoleUnauthorizedAccess means the provider was not
		// authorized to assume the role, so no credential file was written for
		// this entry.
		if isCredentialAssumeRoleUnauthorizedAccess(entry.Code) {
			logger.Debug("IMDS credentials scan: provider not authorized to assume the IAM role",
				logger.Fields{
					field.TaskID: taskID,
					"roleType":   roleType,
					"namespace":  namespace,
				})
			result.AssumeRoleUnauthorizedAccessRoles = append(result.AssumeRoleUnauthorizedAccessRoles, AssumeRoleUnauthorizedAccessIAMRole{
				TaskID:   taskID,
				RoleType: roleType,
				RoleArn:  entry.RoleArn,
			})
			continue
		}

		// Success and AssumeRoleUnauthorizedAccess are the only codes the agent
		// interprets. An unrecognized code is logged and then fetched anyway: a
		// credential file may still exist for it.
		if !isCredentialDelivered(entry.Code) {
			logger.Warn("IMDS credentials scan: unrecognized credential code",
				logger.Fields{
					field.TaskID: taskID,
					"roleType":   roleType,
					"namespace":  namespace,
					"code":       entry.Code,
				})
		}

		credPath := fmt.Sprintf(credentialPathFormat, namespace, key)
		credResp, err := s.getMetadata(ctx, credPath)
		if err != nil {
			logger.Error("IMDS credentials scan: failed to fetch credential", logger.Fields{
				field.TaskID: taskID,
				"roleType":   roleType,
				"namespace":  namespace,
				field.Error:  err,
			})
			s.metricsFactory.New(metrics.IMDSCredentialsScannerCredentialFailureMetricName).
				WithFields(map[string]any{
					metricFieldNamespace: namespace,
					metricFieldTaskID:    taskID,
					metricFieldRoleType:  roleType,
				}).Done(err)
			// Error fetching credential; try remaining credentials in this namespace.
			hasErrors = true
			continue
		}

		var imdsCred imdsCredential
		if err := json.Unmarshal([]byte(credResp), &imdsCred); err != nil {
			logger.Error("IMDS credentials scan: failed to parse credential", logger.Fields{
				field.TaskID: taskID,
				"roleType":   roleType,
				"namespace":  namespace,
				field.Error:  err,
			})
			s.metricsFactory.New(metrics.IMDSCredentialsScannerCredentialFailureMetricName).
				WithFields(map[string]any{
					metricFieldNamespace: namespace,
					metricFieldTaskID:    taskID,
					metricFieldRoleType:  roleType,
				}).Done(err)
			// Error parsing credential; try remaining credentials in this namespace.
			hasErrors = true
			continue
		}

		if err := validateCredential(imdsCred); err != nil {
			logger.Error("IMDS credentials scan: invalid credential", logger.Fields{
				field.TaskID: taskID,
				"roleType":   roleType,
				"namespace":  namespace,
				field.Error:  err,
			})
			s.metricsFactory.New(metrics.IMDSCredentialsScannerCredentialFailureMetricName).
				WithFields(map[string]any{
					metricFieldNamespace: namespace,
					metricFieldTaskID:    taskID,
					metricFieldRoleType:  roleType,
				}).Done(err)
			// Invalid credential; try remaining credentials in this namespace.
			hasErrors = true
			continue
		}

		logger.Debug("IMDS credentials scan: fetched credential", logger.Fields{
			field.TaskID: taskID,
			"roleType":   roleType,
			"namespace":  namespace,
			"expiration": imdsCred.Expiration,
		})
		result.Credentials = append(result.Credentials, TaskCredential{
			TaskID:          taskID,
			RoleType:        roleType,
			RoleArn:         entry.RoleArn,
			AccessKeyID:     imdsCred.AccessKeyId,
			SecretAccessKey: imdsCred.SecretAccessKey,
			SessionToken:    imdsCred.Token,
			Expiration:      imdsCred.Expiration,
		})
	}

	// Only cache LastUpdated if there are no failures recorded,
	// so that failed fetches are retried on the next scan.
	if !hasErrors {
		s.lastUpdated[namespace] = lastUpdated
	}

	// Surface an error when the namespace yielded nothing and also had
	// failures, so callers don't mistake it for "no credentials yet".
	if len(result.Credentials) == 0 && len(result.AssumeRoleUnauthorizedAccessRoles) == 0 && hasErrors {
		return ScanResult{}, fmt.Errorf("all credential processing failed for %s", namespace)
	}

	logger.Info("IMDS credentials scan: namespace scan complete", logger.Fields{
		"namespace":                             namespace,
		"retrievedCredentialCount":              len(result.Credentials),
		"assumeRoleUnauthorizedAccessRoleCount": len(result.AssumeRoleUnauthorizedAccessRoles),
		"lastUpdated":                           info.LastUpdated,
	})

	return result, nil
}

// isCredentialDelivered reports whether an info file entry's Code says a
// credential file was successfully written for it.
func isCredentialDelivered(code string) bool {
	return strings.EqualFold(code, CredentialCodeSuccess)
}

// isCredentialAssumeRoleUnauthorizedAccess reports whether an info file entry's
// Code says the provider was not authorized to assume the IAM role.
func isCredentialAssumeRoleUnauthorizedAccess(code string) bool {
	return strings.EqualFold(code, CredentialCodeAssumeRoleUnauthorizedAccess)
}

// parseCredentialKey extracts the task ID and role type from an IMDS key.
// The expected key format is <taskID>-<roleType>.
func parseCredentialKey(key string) (string, string, error) {
	taskID, roleType, ok := strings.Cut(key, taskIDDelimiter)
	if !ok || taskID == "" || roleType == "" {
		return "", "", fmt.Errorf("unexpected credential key format: %s", key)
	}
	return taskID, roleType, nil
}

// validateCredential reports an error when a required credential field is absent.
// json.Unmarshal leaves fields missing from the document zero-valued, so an
// empty value means the field was not published.
func validateCredential(c imdsCredential) error {
	var missing []string
	if c.AccessKeyId == "" {
		missing = append(missing, "AccessKeyId")
	}
	if c.SecretAccessKey == "" {
		missing = append(missing, "SecretAccessKey")
	}
	if c.Token == "" {
		missing = append(missing, "Token")
	}
	if c.Expiration == "" {
		missing = append(missing, "Expiration")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// getMetadata is a rate-limited wrapper around the EC2 metadata client.
func (s *scanner) getMetadata(ctx context.Context, path string) (string, error) {
	if err := s.rateLimiter.Wait(ctx); err != nil {
		return "", err
	}

	return s.ec2MetadataClient.GetMetadata(path)
}
