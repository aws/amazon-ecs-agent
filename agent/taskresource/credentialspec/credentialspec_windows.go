//go:build windows
// +build windows

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

package credentialspec

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/s3"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	internalarn "github.com/aws/amazon-ecs-agent/ecs-agent/utils/arn"
	"github.com/aws/aws-sdk-go/aws/arn"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/registry"
)

const (
	tempFileName = "temp_file"
	// filePerm is the permission for the credentialspec file.
	filePerm = 0644

	s3DownloadTimeout = 30 * time.Second

	// Environment variables to setup resource location
	envProgramData              = "ProgramData"
	dockerCredentialSpecDataDir = "docker/credentialspecs"
	ecsCcgPluginRegistryKeyRoot = `System\CurrentControlSet\Services\AmazonECSCCGPlugin`
	regKeyPathFormat            = `HKEY_LOCAL_MACHINE\` + ecsCcgPluginRegistryKeyRoot + `\%s`

	credentialSpecParseErrorMsgTemplate = "Unable to parse %s from credential spec"
	untypedMarshallErrorMsgTemplate     = "Unable to marshal untyped object %s to type %s"
)

var (
	// For ease of unit testing
	osWriteFileImpl                           = os.WriteFile
	osReadFileImpl                            = os.ReadFile
	osRemoveImpl                              = os.Remove
	readCredentialSpecImpl                    = readCredentialSpec
	writeCredentialSpecImpl                   = writeCredentialSpec
	readWriteDomainlessCredentialSpecImpl     = readWriteDomainlessCredentialSpec
	setTaskExecutionCredentialsRegKeysImpl    = SetTaskExecutionCredentialsRegKeys
	handleNonFileDomainlessGMSACredSpecImpl   = handleNonFileDomainlessGMSACredSpec
	deleteTaskExecutionCredentialsRegKeysImpl = deleteTaskExecutionCredentialsRegKeys
)

type pluginInput struct {
	CredentialArn string `json:"credentialArn,omitempty"`
	RegKeyPath    string `json:"regKeyPath,omitempty"`
}

// CredentialSpecResource is the abstraction for credentialspec resources
type CredentialSpecResource struct {
	*CredentialSpecResourceCommon
	ioutil ioutilwrapper.IOUtil
	// credentialSpecResourceLocation is the location for all the tasks' credentialspec artifacts
	credentialSpecResourceLocation string
	isDomainlessGMSATask           bool
}

// NewCredentialSpecResource creates a new CredentialSpecResource object
func NewCredentialSpecResource(taskARN, region string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	s3ClientCreator s3factory.S3ClientCreator,
	asmClientCreator asmfactory.ClientCreator,
	credentialSpecContainerMap map[string]string) (*CredentialSpecResource, error) {

	s := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			taskARN:                    taskARN,
			region:                     region,
			credentialsManager:         credentialsManager,
			executionCredentialsID:     executionCredentialsID,
			ssmClientCreator:           ssmClientCreator,
			s3ClientCreator:            s3ClientCreator,
			CredSpecMap:                make(map[string]string),
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
		ioutil:               ioutilwrapper.NewIOUtil(),
		isDomainlessGMSATask: false,
	}

	err := s.setCredentialSpecResourceLocation()
	if err != nil {
		return nil, err
	}

	s.initStatusToTransition()
	return s, nil
}

// Create is used to create all the credentialspec resources for a given task
func (cs *CredentialSpecResource) Create() error {
	var err error
	var iamCredentials credentials.IAMRoleCredentials

	executionCredentials, ok := cs.credentialsManager.GetTaskCredentials(cs.getExecutionCredentialsID())
	if ok {
		iamCredentials = executionCredentials.GetIAMRoleCredentials()
	}

	for credSpecStr := range cs.credentialSpecContainerMap {
		credSpecSplit := strings.SplitAfterN(credSpecStr, ":", 2)
		if len(credSpecSplit) != 2 {
			seelog.Errorf("Invalid credentialspec: %s", credSpecStr)
			continue
		}
		credSpecPrefix := credSpecSplit[0]
		if credSpecPrefix == "credentialspecdomainless:" {
			cs.isDomainlessGMSATask = true
		}
		credSpecValue := credSpecSplit[1]

		if strings.HasPrefix(credSpecValue, "file://") {
			err = cs.handleCredentialspecFile(credSpecStr)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
			continue
		}

		parsedARN, err := arn.Parse(credSpecValue)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}

		parsedARNService := parsedARN.Service
		if parsedARNService == "s3" {
			err = cs.handleS3CredentialspecFile(credSpecStr, credSpecValue, iamCredentials)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file from s3: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
		} else if parsedARNService == "ssm" {
			err = cs.handleSSMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file from SSM: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
		} else {
			err = errors.New("unsupported credentialspec ARN, only s3/ssm ARNs are valid")
			cs.setTerminalReason(err.Error())
			return err
		}
	}

	if cs.isDomainlessGMSATask {
		// The domainless gMSA Windows Plugin needs the execution role credentials to pull customer secrets
		err = setTaskExecutionCredentialsRegKeysImpl(iamCredentials, cs.CredentialSpecResourceCommon.taskARN)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
	}

	return nil
}

func (cs *CredentialSpecResource) handleCredentialspecFile(credentialspec string) error {
	credSpecSplit := strings.SplitAfterN(credentialspec, ":", 2)
	if len(credSpecSplit) != 2 {
		seelog.Errorf("Invalid credentialspec: %s", credentialspec)
		return errors.New("invalid credentialspec file specification")
	}
	credSpecPrefix := credSpecSplit[0]
	credSpecFile := credSpecSplit[1]

	if !strings.HasPrefix(credSpecFile, "file://") {
		return errors.New("invalid credentialspec file specification")
	}

	if credSpecPrefix == "credentialspecdomainless:" {
		relativeFilePath := strings.TrimPrefix(credSpecFile, "file://")
		dir, originalFileName := filepath.Split(relativeFilePath)

		// Generate unique filename using taskId, containerName, credspecfile original name
		taskId, err := internalarn.TaskIdFromArn(cs.taskARN)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
		containerName, ok := cs.credentialSpecContainerMap[credentialspec]
		if !ok {
			return errors.New(fmt.Sprintf("Unable to retrieve containerName from credentialSpecContainerMap. No such key %s", credentialspec))
		}

		// We need a different outfile in order to avoid modifying the customers original credentialspec
		outFile := fmt.Sprintf("%s_%s_%s", taskId, containerName, originalFileName)
		credSpecFile = "file://" + filepath.Join(dir, outFile)

		// Fill in appropriate domainless gMSA fields
		err = readWriteDomainlessCredentialSpecImpl(filepath.Join(cs.credentialSpecResourceLocation, dir, originalFileName), filepath.Join(cs.credentialSpecResourceLocation, dir, outFile), cs.taskARN)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}
	}

	dockerHostconfigSecOptCredSpec := "credentialspec=" + credSpecFile
	cs.updateCredSpecMapping(credentialspec, dockerHostconfigSecOptCredSpec)

	return nil
}

func (cs *CredentialSpecResource) handleS3CredentialspecFile(originalCredentialspec, credentialspecS3ARN string, iamCredentials credentials.IAMRoleCredentials) error {
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		return err
	}

	parsedARN, err := arn.Parse(credentialspecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	bucket, key, err := s3.ParseS3ARN(credentialspecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	s3Client, err := cs.s3ClientCreator.NewS3ManagerClient(bucket, cs.region, iamCredentials)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	resourceBase := filepath.Base(parsedARN.Resource)
	taskArnSplit := strings.Split(cs.taskARN, "/")
	length := len(taskArnSplit)
	if length < 2 {
		return errors.New("Failed to retrieve taskId from taskArn.")
	}

	localCredSpecFilePath := fmt.Sprintf("%s\\s3_%v_%s", cs.credentialSpecResourceLocation, taskArnSplit[length-1], resourceBase)
	err = cs.writeS3File(func(file oswrapper.File) error {
		return s3.DownloadFile(bucket, key, s3DownloadTimeout, file, s3Client)
	}, localCredSpecFilePath)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	err = handleNonFileDomainlessGMSACredSpecImpl(originalCredentialspec, localCredSpecFilePath, cs.taskARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	dockerHostconfigSecOptCredSpec := fmt.Sprintf("credentialspec=file://%s", filepath.Base(localCredSpecFilePath))
	cs.updateCredSpecMapping(originalCredentialspec, dockerHostconfigSecOptCredSpec)

	return nil
}

func (cs *CredentialSpecResource) handleSSMCredentialspecFile(originalCredentialspec, credentialspecSSMARN string, iamCredentials credentials.IAMRoleCredentials) error {
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		return err
	}

	parsedARN, err := arn.Parse(credentialspecSSMARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	ssmClient := cs.ssmClientCreator.NewSSMClient(cs.region, iamCredentials)

	// An SSM ARN is in the form of arn:aws:ssm:us-west-2:123456789012:parameter/a/b. The parsed ARN value
	// would be parameter/a/b. The following code gets the SSM parameter by passing "/a/b" value to the
	// GetParametersFromSSM method to retrieve the value in the parameter.
	ssmParam := strings.SplitAfterN(parsedARN.Resource, "parameter", 2)
	if len(ssmParam) != 2 {
		err := fmt.Errorf("the provided SSM parameter:%s is in an invalid format", parsedARN.Resource)
		cs.setTerminalReason(err.Error())
		return err
	}
	ssmParams := []string{ssmParam[1]}
	ssmParamMap, err := ssm.GetParametersFromSSM(ssmParams, ssmClient)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	ssmParamData := ssmParamMap[ssmParam[1]]
	taskArnSplit := strings.Split(cs.taskARN, "/")
	length := len(taskArnSplit)
	if length < 2 {
		return errors.New("Failed to retrieve taskId from taskArn.")
	}

	taskId := taskArnSplit[length-1]
	containerName := cs.credentialSpecContainerMap[originalCredentialspec]

	// We compose a string that is a concatenation of the task_id, container name and the ARN of the credential spec
	// SSM parameter. This concatenated string is hashed using the SHA-256 hashing scheme to generate a fixed length
	// checksum string of 64 characters. This helps with resolving collisions within a host or a task using the same SSM
	// parameter.
	credSpecFileNameHashFormat := fmt.Sprintf("%s%s%s", taskId, containerName, credentialspecSSMARN)
	hashFunction := sha256.New()
	hashFunction.Write([]byte(credSpecFileNameHashFormat))
	customCredSpecFileName := fmt.Sprintf("%x", hashFunction.Sum(nil))

	localCredSpecFilePath := filepath.Join(cs.credentialSpecResourceLocation, customCredSpecFileName)
	err = cs.writeSSMFile(ssmParamData, localCredSpecFilePath)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	err = handleNonFileDomainlessGMSACredSpecImpl(originalCredentialspec, localCredSpecFilePath, cs.taskARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	dockerHostconfigSecOptCredSpec := fmt.Sprintf("credentialspec=file://%s", customCredSpecFileName)
	cs.updateCredSpecMapping(originalCredentialspec, dockerHostconfigSecOptCredSpec)

	return nil
}

var rename = os.Rename

func (cs *CredentialSpecResource) writeS3File(writeFunc func(file oswrapper.File) error, filePath string) error {
	temp, err := cs.ioutil.TempFile(cs.credentialSpecResourceLocation, tempFileName)
	if err != nil {
		return err
	}

	err = writeFunc(temp)
	if err != nil {
		return err
	}

	err = temp.Close()
	if err != nil {
		seelog.Errorf("Error while closing the handle to file %s: %v", temp.Name(), err)
		return err
	}

	err = rename(temp.Name(), filePath)
	if err != nil {
		seelog.Errorf("Error while renaming the temporary file %s: %v", temp.Name(), err)
		return err
	}
	return nil
}

func (cs *CredentialSpecResource) writeSSMFile(ssmParamData, filePath string) error {
	return cs.ioutil.WriteFile(filePath, []byte(ssmParamData), filePerm)
}

func (cs *CredentialSpecResource) updateCredSpecMapping(credSpecInput, targetCredSpec string) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	seelog.Debugf("Updating credentialspec mapping for %s with %s", credSpecInput, targetCredSpec)
	cs.CredSpecMap[credSpecInput] = targetCredSpec
}

// Cleanup removes the credentialspec created for the task
func (cs *CredentialSpecResource) Cleanup() error {
	cs.clearCredentialSpec()
	if cs.isDomainlessGMSATask {
		err := cs.deleteTaskExecutionCredentialsRegKeys()
		if err != nil {
			return err
		}
	}
	return nil
}

// clearCredentialSpec cycles through the collection of credentialspec data and
// removes them from the task
func (cs *CredentialSpecResource) clearCredentialSpec() {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	for key, value := range cs.CredSpecMap {
		if strings.HasPrefix(key, "credentialspec:file://") {
			seelog.Debugf("Skipping cleanup of local credentialspec file option: %s", key)
			continue
		}
		// Split credentialspec to obtain local file-name
		credSpecSplit := strings.SplitAfterN(value, "file://", 2)
		if len(credSpecSplit) != 2 {
			seelog.Warnf("Unable to parse target credentialspec: %s", value)
			continue
		}
		localCredentialSpecFile := credSpecSplit[1]
		localCredentialSpecFilePath := filepath.Join(cs.credentialSpecResourceLocation, localCredentialSpecFile)
		err := osRemoveImpl(localCredentialSpecFilePath)
		if err != nil {
			seelog.Warnf("Unable to clear local credential spec file %s for task %s", localCredentialSpecFile, cs.taskARN)
		}

		delete(cs.CredSpecMap, key)
	}
}

func (cs *CredentialSpecResource) deleteTaskExecutionCredentialsRegKeys() error {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	return deleteTaskExecutionCredentialsRegKeysImpl(cs.taskARN)
}

// deleteTaskExecutionCredentialsRegKeys deletes the taskExecutionRole IAM credentials in the task registry key
// after the task has been terminated.
func deleteTaskExecutionCredentialsRegKeys(taskARN string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, ecsCcgPluginRegistryKeyRoot, registry.ALL_ACCESS)
	if err != nil {
		// Early exit with success case, if the registry key doesn't exist then there are no task execution role creds to cleanup
		seelog.Errorf("Error opening %s key: %s", ecsCcgPluginRegistryKeyRoot, err)
		return nil
	}
	defer k.Close()

	err = registry.DeleteKey(k, taskARN)
	if err != nil {
		seelog.Errorf("Error deleting %s key: %s", ecsCcgPluginRegistryKeyRoot+"\\"+taskARN, err)
		return err
	}
	seelog.Infof("Deleted Task Execution Credential Registry key for task: %s", taskARN)
	return nil
}

func (cs *CredentialSpecResource) setCredentialSpecResourceLocation() error {
	// TODO: Use registry to setup credentialspec resource location
	// This should always be available on Windows instances
	programDataDir := os.Getenv(envProgramData)
	if programDataDir != "" {
		// Sample resource location: C:\ProgramData\docker\credentialspecs
		cs.credentialSpecResourceLocation = filepath.Join(programDataDir, dockerCredentialSpecDataDir)
	}

	if cs.credentialSpecResourceLocation == "" {
		return errors.New("credentialspec resource location not available")
	}

	return nil
}

// CredentialSpecResourceJSON is the json representation of the credentialspec resource
type CredentialSpecResourceJSON struct {
	*CredentialSpecResourceJSONCommon
}

func (cs *CredentialSpecResource) MarshallPlatformSpecificFields(credentialSpecResourceJSON *CredentialSpecResourceJSON) {
	return
}

func (cs *CredentialSpecResource) UnmarshallPlatformSpecificFields(credentialSpecResourceJSON CredentialSpecResourceJSON) {
	return
}

// setTaskExecutionCredentialsRegKeys stores the taskExecutionRole IAM credentials to the task registry key
// so that the domainless gMSA plugin may use these credentials to access the customer Active Directory authentication
// information.
func SetTaskExecutionCredentialsRegKeys(taskCredentials credentials.IAMRoleCredentials, taskArn string) error {
	if taskCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("Unable to find execution role credentials while setting registry key for task " + taskArn)
		return err
	}

	taskRegistryKey, _, err := registry.CreateKey(registry.LOCAL_MACHINE, ecsCcgPluginRegistryKeyRoot+"\\"+taskArn, registry.WRITE)
	if err != nil {
		errMsg := fmt.Sprintf("Error creating registry key root %s for task %s: %s", ecsCcgPluginRegistryKeyRoot, taskArn, err)
		seelog.Errorf(errMsg)
		return errors.Wrapf(err, errMsg)
	}
	defer taskRegistryKey.Close()

	err = taskRegistryKey.SetStringValue("AKID", taskCredentials.AccessKeyID)
	if err != nil {
		errMsg := fmt.Sprintf("Error creating AKID child value for task %s:%s", taskArn, err)
		seelog.Errorf(errMsg)
		return errors.Wrapf(err, errMsg)
	}
	err = taskRegistryKey.SetStringValue("SKID", taskCredentials.SecretAccessKey)
	if err != nil {
		errMsg := fmt.Sprintf("Error creating AKID child value for task %s:%s", taskArn, err)
		seelog.Errorf(errMsg)
		return errors.Wrapf(err, errMsg)
	}
	err = taskRegistryKey.SetStringValue("SESSIONTOKEN", taskCredentials.SessionToken)
	if err != nil {
		errMsg := fmt.Sprintf("Error creating SESSIONTOKEN child value for task %s:%s", taskArn, err)
		seelog.Errorf(errMsg)
		return errors.Wrapf(err, errMsg)
	}

	seelog.Infof("Successfully SetTaskExecutionCredentialsRegKeys for task %s", taskArn)
	return nil
}

// handleNonFileDomainlessGMSACredSpec reads and then injects the taskExecutionRoleRegistryKey location for
// the s3/ssm gMSA credential spec cases.
func handleNonFileDomainlessGMSACredSpec(originalCredSpec, localCredSpecFilePath, taskARN string) error {
	// Exit early for non domainless gMSA cred specs
	if !strings.HasPrefix(originalCredSpec, "credentialspecdomainless:") {
		return nil
	}

	err := readWriteDomainlessCredentialSpecImpl(localCredSpecFilePath, localCredSpecFilePath, taskARN)
	if err != nil {
		return err
	}
	return nil
}

// readWriteDomainlessCredentialSpec is used to open the credential spec file on local disk, inject the
// taskExecutionRoleInformation in memory, and then write the file to a specific path. The reason we do not
// modify the same fail is to avoid modifying the customer resource when the customer provides a local file
// credential spec
func readWriteDomainlessCredentialSpec(filePath, outFilePath, taskARN string) error {
	credSpec, err := readCredentialSpecImpl(filePath)
	if err != nil {
		return err
	}
	err = writeCredentialSpecImpl(credSpec, outFilePath, taskARN)
	if err != nil {
		return err
	}
	return nil
}

// readCredentialSpec is used to open the credential spec file on local disk and read it into a generic
// bytes map object map[string]interface{}
func readCredentialSpec(filePath string) (map[string]interface{}, error) {
	byteResult, err := osReadFileImpl(filePath)
	if err != nil {
		return nil, err
	}
	var credSpec map[string]interface{}
	err = json.Unmarshal(byteResult, &credSpec)
	if err != nil {
		return nil, err
	}
	return credSpec, nil
}

// writeCredentialSpec is used to selectively decode portions of the Microsoft gMSA generated credential spec file and then
// inject the taskExecutionRoleRegistryKey location so that the gMSA plugin is able to access these IAM credentials.
// The reason that the JSON unmarshalling is manual is to protect against future key/value pairs that appear in the JSON,
// while only modifying the portions that pertain to domainless gMSA. This is in case Microsoft adds additional keys to the
// JSON credential spec, so that our writer does not ignore this data.
func writeCredentialSpec(credSpec map[string]interface{}, outFilePath string, taskARN string) error {
	activeDirectoryConfigUntyped, ok := credSpec["ActiveDirectoryConfig"]
	if !ok {
		return errors.New(fmt.Sprintf(credentialSpecParseErrorMsgTemplate, "ActiveDirectoryConfig"))
	}
	activeDirectoryConfig, ok := activeDirectoryConfigUntyped.(map[string]interface{})
	if !ok {
		return errors.New(fmt.Sprintf(untypedMarshallErrorMsgTemplate, "activeDirectoryConfigUntyped", "map[string]interface{}"))
	}

	hostAccountConfigUntyped, ok := activeDirectoryConfig["HostAccountConfig"]
	if !ok {
		return errors.New(fmt.Sprintf(credentialSpecParseErrorMsgTemplate, "HostAccountConfig"))
	}
	hostAccountConfig, ok := hostAccountConfigUntyped.(map[string]interface{})
	if !ok {
		return errors.New(fmt.Sprintf(untypedMarshallErrorMsgTemplate, "hostAccountConfigUntyped", "map[string]interface{}"))
	}

	pluginInputStringUntyped, ok := hostAccountConfig["PluginInput"]
	if !ok {
		return errors.New(fmt.Sprintf(credentialSpecParseErrorMsgTemplate, "PluginInput"))
	}
	var pluginInputParsed pluginInput
	pluginInputString, ok := pluginInputStringUntyped.(string)
	if !ok {
		return errors.New(fmt.Sprintf(untypedMarshallErrorMsgTemplate, "pluginInputStringUntyped", "string"))
	}
	err := json.Unmarshal([]byte(pluginInputString), &pluginInputParsed)
	if err != nil {
		return err
	}

	pluginInputParsed.RegKeyPath = fmt.Sprintf(regKeyPathFormat, taskARN)

	pluginInputBytes, err := json.Marshal(pluginInputParsed)
	if err != nil {
		return err
	}

	hostAccountConfig["PluginInput"] = string(pluginInputBytes)

	jsonBytes, err := json.Marshal(credSpec)
	if err != nil {
		return err
	}

	err = osWriteFileImpl(outFilePath, jsonBytes, filePerm)
	if err != nil {
		return err
	}

	return nil
}
