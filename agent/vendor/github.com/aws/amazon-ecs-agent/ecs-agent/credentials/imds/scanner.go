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

// Package imds provides an IMDS credential scanner that retrieves task
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

	"golang.org/x/time/rate"
)

const (
	// namespacePrefix is the IMDS path prefix for ECS IAM namespaces.
	namespacePrefix = "iam-ecs-"

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
	// The rate limiter keeps credential scanning within ~10% of the total PPS budget
	// to leave headroom for other link-local requests on the instance.
	//
	// TODO: this value will be finalized based on load testing.
	imdsQueriesPerSec = 10

	// imdsQueryBurstSize is the token bucket size for the rate limiter.
	imdsQueryBurstSize = 1
)

// Scanner fetches task credentials from IMDS iam-ecs-* namespaces.
type Scanner interface {
	// Scan discovers all ECS IAM namespaces, reads their info files, and
	// fetches credentials from namespaces that have changed since the last scan.
	Scan(ctx context.Context) ([]TaskCredential, error)
}

// scanner implements the Scanner interface.
// TODO: add metricsFactory for operational metrics on scan failures.
type scanner struct {
	ec2MetadataClient ec2.EC2MetadataClient
	// rateLimiter controls the rate of IMDS requests.
	rateLimiter *rate.Limiter
	// lastUpdated tracks the LastUpdated timestamp from each namespace's info file.
	lastUpdated map[string]time.Time
}

// NewScanner creates a new IMDS credential scanner.
func NewScanner(ec2MetadataClient ec2.EC2MetadataClient) Scanner {
	return &scanner{
		ec2MetadataClient: ec2MetadataClient,
		rateLimiter:       rate.NewLimiter(rate.Limit(imdsQueriesPerSec), imdsQueryBurstSize),
		lastUpdated:       make(map[string]time.Time),
	}
}

// Scan discovers all ECS IAM namespaces, reads their info files, and
// fetches credentials from namespaces that have changed since the last scan.
func (s *scanner) Scan(ctx context.Context) ([]TaskCredential, error) {
	namespaces, err := s.discoverNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("imds scan: discover namespaces: %w", err)
	}

	// No namespaces is expected when IMDS does not have ECS task credentials yet.
	if len(namespaces) == 0 {
		logger.Debug("No iam-ecs namespaces found in IMDS")
		return nil, nil
	}

	var creds []TaskCredential
	var scanErrors []error
	for _, ns := range namespaces {
		nsCreds, err := s.scanNamespace(ctx, ns)
		if err != nil {
			logger.Error("Failed to scan IMDS namespace", logger.Fields{
				"namespace": ns,
				field.Error: err,
			})
			scanErrors = append(scanErrors, err)
			// Scanning for a namespace failed; try scanning remaining namespaces before returning.
			continue
		}
		creds = append(creds, nsCreds...)
	}

	// Surface an error when all namespaces failed so the caller doesn't
	// mistake it for "no credentials yet".
	if len(creds) == 0 && len(scanErrors) > 0 {
		return nil, fmt.Errorf("imds scan: all %d namespace(s) failed: %w",
			len(scanErrors), errors.Join(scanErrors...))
	}

	return creds, nil
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
		if strings.HasPrefix(entry, namespacePrefix) {
			namespaces = append(namespaces, entry)
		}
	}

	return namespaces, nil
}

// scanNamespace reads the info file for a namespace and fetches
// credentials for each entry. Only successfully fetched credentials
// are returned.
func (s *scanner) scanNamespace(ctx context.Context, namespace string) ([]TaskCredential, error) {
	infoPath := fmt.Sprintf(infoPathFormat, namespace)
	infoResp, err := s.getMetadata(ctx, infoPath)
	if err != nil {
		return nil, fmt.Errorf("fetch info for %s: %w", namespace, err)
	}

	var info NamespaceInfo
	if err := json.Unmarshal([]byte(infoResp), &info); err != nil {
		return nil, fmt.Errorf("parse info for %s: %w", namespace, err)
	}

	// Skip credential fetches if the namespace hasn't been updated since the last scan.
	lastUpdated, err := time.Parse(time.RFC3339, info.LastUpdated)
	if err != nil {
		return nil, fmt.Errorf("parse LastUpdated for %s: %w", namespace, err)
	}
	if cached, ok := s.lastUpdated[namespace]; ok && lastUpdated.Equal(cached) {
		logger.Debug("Skipping namespace with unchanged LastUpdated", logger.Fields{
			"namespace": namespace,
		})
		return nil, nil
	}

	var creds []TaskCredential
	var hasErrors bool
	for key, entry := range info.TaskCredentials {
		taskID, roleType, err := parseCredentialKey(key)
		if err != nil {
			logger.Error("Failed to parse credential key from IMDS", logger.Fields{
				"namespace": namespace,
				field.Error: err,
			})
			// Cannot determine task ID; attempt the next credential.
			hasErrors = true
			continue
		}

		credPath := fmt.Sprintf(credentialPathFormat, namespace, key)
		credResp, err := s.getMetadata(ctx, credPath)
		if err != nil {
			logger.Error("Failed to fetch from IMDS", logger.Fields{
				field.TaskID: taskID,
				"namespace":  namespace,
				field.Error:  err,
			})
			// Fetch failed; try remaining credentials in this namespace.
			hasErrors = true
			continue
		}

		var imdsCred imdsCredential
		if err := json.Unmarshal([]byte(credResp), &imdsCred); err != nil {
			logger.Error("Failed to parse IMDS response", logger.Fields{
				field.TaskID: taskID,
				"namespace":  namespace,
				field.Error:  err,
			})
			// Error parsing response; try remaining credentials in this namespace.
			hasErrors = true
			continue
		}

		creds = append(creds, TaskCredential{
			TaskID:          taskID,
			RoleType:        roleType,
			RoleArn:         entry.RoleArn,
			AccessKeyID:     imdsCred.AccessKeyId,
			SecretAccessKey: imdsCred.SecretAccessKey,
			SessionToken:    imdsCred.Token,
			Expiration:      imdsCred.Expiration,
		})
	}

	// Only cache LastUpdated if all credentials were fetched successfully,
	// so that failed fetches are retried on the next scan.
	if !hasErrors {
		s.lastUpdated[namespace] = lastUpdated
	}

	return creds, nil
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

// getMetadata is a rate-limited wrapper around the EC2 metadata client.
func (s *scanner) getMetadata(ctx context.Context, path string) (string, error) {
	if err := s.rateLimiter.Wait(ctx); err != nil {
		return "", err
	}

	return s.ec2MetadataClient.GetMetadata(path)
}
