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

package imds

const (
	// CredentialStatusDelivered is the info file status for a credential the
	// provider assumed the role for and wrote to IMDS.
	CredentialStatusDelivered = "0"

	// CredentialStatusAssumeRoleFailed is the info file status for a credential
	// the provider could not assume the role for, so no credential file was
	// written. It is remediable by the customer (for example by fixing the
	// role's trust policy or an account issue), after which the provider
	// delivers the credential.
	CredentialStatusAssumeRoleFailed = "1"
)

// NamespaceInfo represents the parsed info file from an iam-ecs-* namespace.
// JSON tags match the IMDS response format.
type NamespaceInfo struct {
	LastUpdated string `json:"LastUpdated"`
	// TaskCredentials maps a "<taskID>-<roleType>" key to its delivery status.
	TaskCredentials map[string]string `json:"TaskCredentials"`
}

// TaskCredential represents a task credential retrieved from IMDS.
type TaskCredential struct {
	TaskID          string
	RoleType        string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      string
}

// AssumeRoleFailedIAMRole identifies a task's IAM role that the provider could
// not assume, as reported by a namespace info file entry with status
// CredentialStatusAssumeRoleFailed. It carries no credential material because
// the provider wrote none.
type AssumeRoleFailedIAMRole struct {
	TaskID   string
	RoleType string
}

// ScanResult holds the outcome of a single IMDS credentials scan.
type ScanResult struct {
	// Credentials are the task credentials retrieved from IMDS.
	Credentials []TaskCredential
	// AssumeRoleFailedRoles are the roles the provider could not assume. A
	// consumer uses them to distinguish a stale credential the customer must
	// remediate from a broken delivery path.
	AssumeRoleFailedRoles []AssumeRoleFailedIAMRole
}

// imdsCredential is used internally by the scanner to deserialize IMDS
// credential files, which use different field names than TaskCredential
// (e.g. "Token" vs SessionToken). JSON tags match the IMDS response format.
type imdsCredential struct {
	AccessKeyId     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
	Expiration      string `json:"Expiration"`
}
